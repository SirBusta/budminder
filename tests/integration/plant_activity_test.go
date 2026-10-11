package integration

import (
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// activityFixture wires up the FK chain (breeder → strain → zone →
// plant) plus the API key, ready for activity/measurement endpoints.
type activityHTTPFixture struct {
	APIKey   string
	PlantID  int64
	WaterID  int
	FeedID   int
	HeightID int
}

func seedActivityHTTP(t *testing.T, db *sql.DB) activityHTTPFixture {
	t.Helper()
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := int64(testutil.SeedPlant(t, db, "Plant 1", strainID, zoneID))

	var waterID, feedID, heightID int
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name='Water'`).Scan(&waterID))
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name='Feed'`).Scan(&feedID))
	require.NoError(t, db.QueryRow(`SELECT id FROM metric WHERE name='Height'`).Scan(&heightID))

	return activityHTTPFixture{
		APIKey:  testutil.SeedAPIKey(t, db, "test-activity-key"),
		PlantID: plantID,
		WaterID: waterID, FeedID: feedID, HeightID: heightID,
	}
}

// ---------------------------------------------------------------------------
// POST /plantActivity
// ---------------------------------------------------------------------------

func TestActivity_CreateHappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plantActivity", fix.APIKey, map[string]interface{}{
		"plant_id":    fix.PlantID,
		"activity_id": fix.WaterID,
		"note":        "first water",
		"date":        "2026-04-25",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_activity WHERE plant_id = $1`, fix.PlantID).Scan(&n))
	assert.Equal(t, 1, n, "activity row should be persisted")
}

func TestActivity_CreateRejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	c := server.NewClient(t)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/plantActivity", strings.NewReader("{not json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", fix.APIKey)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// POST /plantActivity/edit
// ---------------------------------------------------------------------------

func TestActivity_EditUpdatesRow(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	res, err := db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'orig note', '2026-04-01')`,
		fix.PlantID, fix.WaterID,
	)
	require.NoError(t, err)
	actID, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plantActivity/edit", fix.APIKey, map[string]interface{}{
		"id":          actID,
		"date":        "2026-04-15",
		"activity_id": fix.FeedID, // change activity from Water → Feed
		"note":        "updated note",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var note string
	var actName int
	require.NoError(t, db.QueryRow(
		`SELECT note, activity_id FROM plant_activity WHERE id = $1`, actID,
	).Scan(&note, &actName))
	assert.Equal(t, "updated note", note)
	assert.Equal(t, fix.FeedID, actName)
}

// TestActivity_EditAcceptsMinutePrecisionDate reproduces the "changing
// the activity type doesn't save" report: a browser's datetime-local input
// drops the seconds whenever they are zero ("2026-10-03T09:00"), which the
// edit handler used to reject — and the page reloaded without showing the
// error, so the type change appeared to silently revert.
func TestActivity_EditAcceptsMinutePrecisionDate(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)
	testutil.SeedAdmin(t, db, "activity-edit-pw")

	res, err := db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'a note', '2026-10-03T09:00:00')`,
		fix.PlantID, fix.FeedID,
	)
	require.NoError(t, err)
	actID, _ := res.LastInsertId()

	c, token := server.LoginAndFetchCSRF(t, "activity-edit-pw", "/plants")
	resp := c.SessionPostJSON(t, "/plantActivity/edit", token, map[string]interface{}{
		"id":          actID,
		"date":        "2026-10-03T09:00",
		"activity_id": fix.WaterID,
		"note":        "a note",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var activityID int
	var date string
	require.NoError(t, db.QueryRow(
		`SELECT activity_id, date FROM plant_activity WHERE id = $1`, actID,
	).Scan(&activityID, &date))
	assert.Equal(t, fix.WaterID, activityID)
	assert.Contains(t, date, "09:00:00", "seconds must be filled in so the stored date parses back")
}

// ---------------------------------------------------------------------------
// DELETE /plantActivity/delete/:id
// ---------------------------------------------------------------------------

func TestActivity_DeleteRemovesRow(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	res, err := db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'doomed', '2026-04-25')`,
		fix.PlantID, fix.WaterID,
	)
	require.NoError(t, err)
	actID, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/plantActivity/delete/"+strconv.FormatInt(actID, 10), fix.APIKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_activity WHERE id = $1`, actID).Scan(&n))
	assert.Zero(t, n)
}

// ---------------------------------------------------------------------------
// POST /record-multi-activity
// ---------------------------------------------------------------------------

func TestActivity_MultiPlantHappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	// Add a second plant so multi-plant insert covers >1 row.
	res, err := db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ('Plant 2', 1, 1, '', 0, '2026-01-01', '[]')`,
	)
	require.NoError(t, err)
	plant2, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/record-multi-activity", fix.APIKey, map[string]interface{}{
		"plant_ids":   []int64{fix.PlantID, plant2},
		"activity_id": fix.WaterID,
		"note":        "scheduled water",
		"date":        "2026-04-25",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_activity WHERE note = 'scheduled water'`).Scan(&n))
	assert.Equal(t, 2, n, "one row per plant in plant_ids")
}

func TestActivity_MultiPlantRejectsEmptyList(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/record-multi-activity", fix.APIKey, map[string]interface{}{
		"plant_ids":   []int{},
		"activity_id": fix.WaterID,
		"note":        "no plants",
		"date":        "2026-04-25",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GET /activities  (the old Activity Log page → All plants notebook)
// ---------------------------------------------------------------------------

func TestActivity_LogPageRedirectsToAllPlants(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "redirect-pw")
	c := server.LoginAsAdmin(t, "redirect-pw")
	noFollow := *c.Client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	location := func() string {
		resp, err := noFollow.Get(c.BaseURL + "/activities")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode)
		return resp.Header.Get("Location")
	}

	assert.Equal(t, "/plants", location(), "no plants yet")

	fix := seedActivityHTTP(t, db)
	other := testutil.SeedPlant(t, db, "Plant 2", 1, 1)
	var veg, dead int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = 'Veg'`).Scan(&veg))
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = 'Dead'`).Scan(&dead))
	testutil.MustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-02')`, fix.PlantID, veg)
	testutil.MustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-02')`, other, veg)
	testutil.SeedActivityAt(t, db, int(fix.PlantID), fix.WaterID, "2026-01-05T08:00:00")
	testutil.SeedActivityAt(t, db, other, fix.WaterID, "2026-01-03T08:00:00")
	assert.Equal(t, "/plant/"+strconv.Itoa(other)+"#all", location(), "most recently logged growing plant")

	testutil.MustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-06')`, other, dead)
	assert.Equal(t, "/plant/"+strconv.FormatInt(fix.PlantID, 10)+"#all", location(), "dead plants are skipped")
}

// ---------------------------------------------------------------------------
// GET /activities/export/csv  (protected — session-gated)
// ---------------------------------------------------------------------------

func TestActivity_ExportCSV(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	_, err := db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'csv test row', '2026-04-25')`,
		fix.PlantID, fix.WaterID,
	)
	require.NoError(t, err)

	testutil.SeedAdmin(t, db, "csv-pw")
	c := server.LoginAsAdmin(t, "csv-pw")

	resp := c.Get("/activities/export/csv")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/csv")
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	csv := string(body)
	assert.Contains(t, csv, "Date,Plant,Strain,Zone,Activities,Measurements,Note", "header row")
	assert.Contains(t, csv, "csv test row", "seeded note must appear")
}
