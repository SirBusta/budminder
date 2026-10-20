package handlers_test

// The Home ("Today") page data (handlers/home.go), its refresh endpoint,
// guest rules, and the old /graph/:id pages forwarding to Environment.

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

const homePassword = "home-pass"

type homeData struct {
	Plants []struct {
		ID        int                `json:"id"`
		Name      string             `json:"name"`
		ZoneID    int                `json:"zone_id"`
		PhotoDate string             `json:"photo_date"`
		Last      map[string]float64 `json:"last"`
	} `json:"plants"`
	Tents []struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Readings []struct {
			ID int `json:"id"`
		} `json:"readings"`
		Ports []struct {
			ID int `json:"id"`
		} `json:"ports"`
		Quiet []struct {
			ID int `json:"id"`
		} `json:"quiet"`
		Cameras []struct {
			ID       int    `json:"id"`
			Snapshot string `json:"snapshot"`
			SnapAt   int64  `json:"snap_at"`
		} `json:"cameras"`
		Plants []int `json:"plants"`
	} `json:"tents"`
	Recent []struct {
		Date     string `json:"date"`
		PlantIDs []int  `json:"plant_ids"`
		Note     string `json:"note"`
	} `json:"recent"`
	OldSeeds *int `json:"old_seed_batches"`
	LoggedIn bool `json:"logged_in"`
}

func getHomeData(t *testing.T, c *testutil.Client) (int, homeData) {
	t.Helper()
	resp := c.Get("/home/data")
	defer testutil.DrainAndClose(resp)
	var d homeData
	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&d))
	}
	return resp.StatusCode, d
}

func TestHomeData(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	streamDir := t.TempDir()
	server := testutil.NewTestServer(t, db, testutil.WithStreamDir(streamDir))
	testutil.SeedAdmin(t, db, homePassword)
	c, token := server.LoginAndFetchCSRF(t, homePassword, "/settings")
	e := &sensorEnv{db: db, server: server, client: c, token: token}

	camTent := testutil.SeedZone(t, db, "Cam Tent")
	plainTent := testutil.SeedZone(t, db, "Plain Tent")
	emptyTent := testutil.SeedZone(t, db, "Empty Tent")
	a := livingPlant(t, e, "Alpha", camTent, "Veg")
	b := livingPlant(t, e, "Beta", camTent, "Flower")
	cPlant := livingPlant(t, e, "Gamma", plainTent, "Seedling")
	livingPlant(t, e, "Gone", emptyTent, "Dead")

	// Sensors in the camera tent: one on Home, one not on Home, one hidden, a port.
	sensor := func(kind, vis string) int {
		id := testutil.SeedSensor(t, db, "acinfinity", "D", kind)
		e.exec(t, `UPDATE sensors SET zone_id = $1, visibility = $2 WHERE id = $3`, camTent, vis, id)
		e.exec(t, `INSERT INTO sensor_data (sensor_id, value) VALUES ($1, 70)`, id)
		return id
	}
	homeTemp := sensor("ACI.1.0", "zone_plant")
	sensor("ACI.2.4", "plant")
	sensor("ACI.1.2", "hide")
	port := sensor("ACIP.1", "plant")

	// A visible camera with a snapshot, and a hidden one.
	e.exec(t, `INSERT INTO streams (name, url, zone_id, visible) VALUES ('Cam', 'http://cam/live.m3u8', $1, true)`, camTent)
	e.exec(t, `INSERT INTO streams (name, url, zone_id, visible) VALUES ('Hidden cam', 'http://cam/2.m3u8', $1, false)`, plainTent)
	var camID int
	require.NoError(t, db.QueryRow(`SELECT id FROM streams WHERE name = 'Cam'`).Scan(&camID))
	require.NoError(t, os.WriteFile(filepath.Join(streamDir, "stream_"+strconv.Itoa(camID)+"_latest.jpg"), []byte("jpg"), 0o644))

	// Measurements: the latest value per metric wins.
	var metric int
	require.NoError(t, db.QueryRow(`SELECT id FROM metric WHERE name = 'Water amount'`).Scan(&metric))
	e.exec(t, `INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, 1, '2026-10-01 08:00:00')`, a, metric)
	e.exec(t, `INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, 1.5, '2026-10-05T08:00:00')`, a, metric)
	e.exec(t, `INSERT INTO plant_images (plant_id, image_path, image_date) VALUES ($1, 'uploads/plants/x.jpg', '2026-10-03')`, a)

	// Recent: the same entry logged for two plants is shown once.
	water := activityIDByName(t, db, "Water")
	resp := c.SessionPostJSON(t, "/plants/day", token, map[string]any{
		"plant_ids": []int{a, b}, "date": time.Now().Format("2006-01-02"), "activity_ids": []int{water}, "note": "both",
	})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// One seed batch over two years old, one recent, one out of stock.
	breeder := testutil.SeedBreeder(t, db, "Seeds B")
	old := testutil.SeedStrain(t, db, breeder, "Old")
	e.exec(t, `UPDATE strain SET seed_count = 5, seeds_added_on = '2020-01-01' WHERE id = $1`, old)
	fresh := testutil.SeedStrain(t, db, breeder, "Fresh")
	e.exec(t, `UPDATE strain SET seed_count = 5, seeds_added_on = $1 WHERE id = $2`, time.Now().Format("2006-01-02"), fresh)
	empty := testutil.SeedStrain(t, db, breeder, "Empty")
	e.exec(t, `UPDATE strain SET seed_count = 0, seeds_added_on = '2019-01-01' WHERE id = $1`, empty)

	code, d := getHomeData(t, c)
	require.Equal(t, http.StatusOK, code)
	assert.True(t, d.LoggedIn)

	names := []string{}
	for _, p := range d.Plants {
		names = append(names, p.Name)
	}
	assert.ElementsMatch(t, []string{"Alpha", "Beta", "Gamma"}, names, "growing plants only")

	require.Len(t, d.Tents, 2, "only tents with growing plants")
	cam, plain := d.Tents[0], d.Tents[1]
	assert.Equal(t, "Cam Tent", cam.Name)
	assert.ElementsMatch(t, []int{a, b}, cam.Plants)
	require.Len(t, cam.Readings, 1, "only the Home-visible, not hidden reading")
	assert.Equal(t, homeTemp, cam.Readings[0].ID)
	require.Len(t, cam.Ports, 1)
	assert.Equal(t, port, cam.Ports[0].ID)
	require.Len(t, cam.Cameras, 1)
	assert.Equal(t, "/uploads/streams/stream_"+strconv.Itoa(camID)+"_latest.jpg", cam.Cameras[0].Snapshot)
	assert.NotZero(t, cam.Cameras[0].SnapAt)
	assert.Equal(t, "Plain Tent", plain.Name)
	assert.Empty(t, plain.Cameras, "a hidden camera gives no tile")
	assert.Equal(t, []int{cPlant}, plain.Plants)

	for _, p := range d.Plants {
		if p.ID == a {
			assert.Equal(t, 1.5, p.Last[strconv.Itoa(metric)], "latest water amount")
			assert.Equal(t, "2026-10-03", p.PhotoDate)
		}
	}

	require.NotEmpty(t, d.Recent)
	assert.Equal(t, "both", d.Recent[0].Note)
	assert.ElementsMatch(t, []int{a, b}, d.Recent[0].PlantIDs, "logged for both plants, shown once")

	require.NotNil(t, d.OldSeeds)
	assert.Equal(t, 1, *d.OldSeeds)
}

// A sensor left off the tent summary still reaches Needs attention when it
// stops reporting; a fresh one or a hidden one does not.
func TestHomeData_QuietSensorsOffTheSummary(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, homePassword)
	c, token := server.LoginAndFetchCSRF(t, homePassword, "/settings")
	e := &sensorEnv{db: db, server: server, client: c, token: token}

	tent := testutil.SeedZone(t, db, "Tent")
	livingPlant(t, e, "Alpha", tent, "Veg")
	sensor := func(kind, vis, at string) int {
		id := testutil.SeedSensor(t, db, "acinfinity", "D", kind)
		e.exec(t, `UPDATE sensors SET zone_id = $1, visibility = $2 WHERE id = $3`, tent, vis, id)
		e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 70, $2)`, id, at)
		return id
	}
	old := time.Now().UTC().Add(-6 * time.Hour).Format("2006-01-02 15:04:05")
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	quiet := sensor("ACI.2.4", "plant", old)
	sensor("ACI.1.0", "plant", now)
	sensor("ACI.1.2", "hide", old)

	code, d := getHomeData(t, c)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, d.Tents, 1)
	assert.Empty(t, d.Tents[0].Readings, "none of them is on the summary")
	require.Len(t, d.Tents[0].Quiet, 1, "only the stopped, not hidden sensor")
	assert.Equal(t, quiet, d.Tents[0].Quiet[0].ID)
}

func TestHomeData_GuestRules(t *testing.T) {
	t.Parallel()
	// Without guest mode: login needed.
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	code, _ := getHomeData(t, server.NewClient(t))
	assert.NotEqual(t, http.StatusOK, code)

	// Guest mode: readable, but no alerts.
	gdb := testutil.NewTestDB(t)
	guest := testutil.NewTestServer(t, gdb, testutil.WithGuestMode())
	code, d := getHomeData(t, guest.NewClient(t))
	require.Equal(t, http.StatusOK, code)
	assert.False(t, d.LoggedIn)
	assert.Nil(t, d.OldSeeds, "no alerts for guests")
}

func TestGraphPageForwardsToEnvironment(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	zone := testutil.SeedZone(t, e.db, "Graph Tent")
	sid := testutil.SeedSensor(t, e.db, "acinfinity", "D", "ACI.1.0")
	e.exec(t, `UPDATE sensors SET zone_id = $1 WHERE id = $2`, zone, sid)
	loose := testutil.SeedSensor(t, e.db, "acinfinity", "D", "ACI.1.2")

	check := func(id int, want string) {
		resp := e.client.Get("/graph/" + strconv.Itoa(id)) // the test client doesn't follow redirects
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, want, resp.Header.Get("Location"))
	}
	check(sid, "/sensors#zone="+strconv.Itoa(zone))
	check(loose, "/sensors")
}

// The page itself: logged in it carries the Log window and the upload window;
// for guests it renders read-only, without them.
func TestHomePage_Renders(t *testing.T) {
	t.Parallel()
	page := func(c *testutil.Client) string {
		resp := c.Get("/")
		defer testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(body)
	}

	e := newSensorEnv(t)
	html := page(e.client)
	assert.Contains(t, html, `id="homePage"`)
	assert.Contains(t, html, `id="hmLog"`, "logged in: Log activity window")
	assert.Contains(t, html, `id="uploadImagesModal"`, "logged in: Add photos window")
	assert.NotContains(t, html, "video.min.js", "no camera, no player")

	guest := testutil.NewTestServer(t, testutil.NewTestDB(t), testutil.WithGuestMode())
	html = page(guest.NewClient(t))
	assert.Contains(t, html, `id="homePage"`)
	assert.NotContains(t, html, `id="hmLog"`, "guests are read-only")
	assert.NotContains(t, html, `id="uploadImagesModal"`)
}

// The top menu marks the page you're on (it shows its name in a pill).
// Manage sensors sits in the Settings menu, so Settings is the one marked.
func TestTopMenu_SelectedItem(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	selected := regexp.MustCompile(`<a href="([^"]+)" class="nav-link active" aria-current="page"`)
	for path, want := range map[string]string{
		"/": "/", "/plants": "/plants", "/strains": "/strains",
		"/sensors": "/sensors", "/sensors/manage": "/settings", "/settings": "/settings",
	} {
		resp := e.client.Get(path)
		body, err := io.ReadAll(resp.Body)
		testutil.DrainAndClose(resp)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
		got := selected.FindAllStringSubmatch(string(body), -1)
		require.Len(t, got, 1, path)
		assert.Equal(t, want, got[0][1], path)
	}
}
