package handlers_test

// Demo mode (handlers/demo.go), tested the way an attacker would use it:
// requests sent straight to the server, skipping the pages.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/demo"
	"isley/handlers"
	"isley/tests/testutil"
)

type demoEnv struct {
	server *testutil.TestServer
	client *testutil.Client
	token  string
	plant  int
	strain int
	zone   int
	water  int
}

func newDemo(t *testing.T) *demoEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, "demo-pass-unused")
	breeder := testutil.SeedBreeder(t, db, "Seed Junky")
	strain := testutil.SeedStrain(t, db, breeder, "Permanent Marker")
	zone := testutil.SeedZone(t, db, "3x3 Tent")
	plant := testutil.SeedPlant(t, db, "Permanent Marker #1", strain, zone)
	server := testutil.NewTestServer(t, db, testutil.WithDemo())
	client := server.NewClient(t)
	return &demoEnv{server: server, client: client, token: client.FetchMetaCSRFToken("/plants"),
		plant: plant, strain: strain, zone: zone, water: activityIDByName(t, db, "Water")}
}

// send makes a request with any body, like a hand-made attack would.
func (e *demoEnv) send(t *testing.T, method, path, contentType string, body []byte) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, e.client.BaseURL+path, bytes.NewReader(body))
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-CSRF-Token", e.token)
	resp, err := e.client.Do(req)
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (e *demoEnv) json(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	return e.send(t, method, path, "application/json", b)
}

func (e *demoEnv) getJSON(t *testing.T, path string, out any) {
	t.Helper()
	resp := e.client.Get(path)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, path)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
}

func assertRefused(t *testing.T, code int, out map[string]any, what string) {
	t.Helper()
	assert.Equal(t, http.StatusForbidden, code, what)
	assert.Equal(t, true, out["demo"], what+": refused by the demo, not something else")
}

func TestDemo_SignsVisitorsInAndHidesLogin(t *testing.T) {
	t.Parallel()
	e := newDemo(t)

	resp := e.client.Get("/")
	body, _ := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "no password needed")
	assert.Contains(t, string(body), "bm-demo-banner")
	assert.Contains(t, string(body), `content="noindex, nofollow"`)
	assert.Equal(t, "noindex, nofollow", resp.Header.Get("X-Robots-Tag"))

	for _, p := range []string{"/login", "/change-password"} {
		resp := e.client.Get(p)
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusFound, resp.StatusCode, p)
		assert.Equal(t, "/", resp.Header.Get("Location"), p)
	}
}

func TestDemo_RefusesEverythingNotOnTheList(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	for _, r := range []struct{ method, path string }{
		{"POST", "/login"}, {"POST", "/change-password"}, {"POST", "/settings"},
		{"POST", "/settings/backup/create"}, {"POST", "/settings/backup/restore"}, {"POST", "/settings/backup/sqlite/upload"},
		{"POST", "/settings/upload-logo"}, {"POST", "/settings/homeassistant/test"},
		{"POST", "/sensors/scanACI"}, {"POST", "/sensors/scanEC"}, {"POST", "/sensors/scanHA"},
		{"POST", "/streams"}, {"POST", "/aci/login"}, {"POST", "/zones"}, {"POST", "/activities"},
		{"POST", "/breeders"}, {"POST", "/breeders/reference/refresh"}, {"DELETE", "/strains/1"},
		{"DELETE", "/plant/delete/1"}, {"POST", "/plant/1/images/upload"}, {"POST", "/decorateImage"},
		{"POST", "/smoke/1/photos/url"}, {"POST", "/smoke/1/photos"}, {"DELETE", "/smoke/1"},
		{"POST", "/strains/import/save"}, {"POST", "/api/sensors/ingest"}, {"POST", "/plant"},
	} {
		code, out := e.json(t, r.method, r.path, map[string]any{})
		assertRefused(t, code, out, r.method+" "+r.path)
	}
}

func TestDemo_APIKeyDoesNotSkipTheChecks(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	req, err := http.NewRequest("POST", e.client.BaseURL+"/api/sensors/ingest", strings.NewReader(`{}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "anything")
	resp, err := e.client.Do(req)
	require.NoError(t, err)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "no CSRF token: the key no longer skips the check")
}

func TestDemo_NoExportsLogsOrBackups(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	for _, p := range []string{"/activities/export/csv?plant_id=1", "/activities/export/xlsx?plant_id=1",
		"/settings/logs", "/settings/logs/download", "/settings/backup/list", "/settings/backup/sqlite/download",
		"/sensors/dumpACI", "/strains/cannadb/search?q=grape"} {
		resp := e.client.Get(p)
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, p)
	}
}

func TestDemo_NotesCanBeTypedButAreShort(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	day := func(date string, note any) (int, map[string]any) {
		return e.json(t, "POST", "/plants/day", map[string]any{
			"plant_ids": []int{e.plant}, "date": date, "activity_ids": []int{e.water}, "note": note,
		})
	}
	for _, bad := range []any{42, strings.Repeat("x", handlers.DemoMaxNote+1)} {
		code, out := day("2026-10-01", bad)
		assertRefused(t, code, out, "note")
	}
	// Typed notes are stored as plain text, even ones that look like attacks.
	notes := map[string]string{
		"2026-10-02": "Watered until runoff.",
		"2026-10-03": "'; DROP TABLE plant; --",
		"2026-10-04": "<script>alert(1)</script>",
		"2026-10-05": strings.Repeat("é", handlers.DemoMaxNote),
	}
	for date, note := range notes {
		code, out := day(date, note)
		require.Equal(t, http.StatusOK, code, out)
	}
	var nb struct {
		Days []struct {
			Date string `json:"date"`
			Note string `json:"note"`
		} `json:"days"`
	}
	e.getJSON(t, "/plant/"+itoa(e.plant)+"/notebook", &nb)
	got := map[string]string{}
	for _, d := range nb.Days {
		got[d.Date] = d.Note
	}
	for date, note := range notes {
		assert.Equal(t, note, got[date], "stored exactly, as text")
	}
}

func TestDemo_RefusesOddRequests(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	path := "/strains/" + itoa(e.strain) + "/wanted"
	cases := []struct {
		name, ctype string
		body        []byte
	}{
		{"unknown field", "application/json", []byte(`{"wanted": true, "admin": true}`)},
		{"wrong type", "application/json", []byte(`{"wanted": "yes"}`)},
		{"not JSON", "application/x-www-form-urlencoded", []byte(`wanted=true`)},
		{"two objects", "application/json", []byte(`{"wanted": true}{"wanted": false}`)},
		{"broken JSON", "application/json", []byte(`{"wanted": tru`)},
		{"huge body", "application/json", append([]byte(`{"wanted": true, "x": "`), bytes.Repeat([]byte("a"), 100<<10)...)},
	}
	for _, c := range cases {
		code, _ := e.send(t, "POST", path, c.ctype, c.body)
		assert.Contains(t, []int{http.StatusForbidden, http.StatusRequestEntityTooLarge}, code, c.name)
	}
	code, _ := e.send(t, "POST", path, "application/json", []byte(`{"wanted": true}`))
	assert.Equal(t, http.StatusOK, code)
}

func TestDemo_NumbersHaveLimits(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	path := "/strains/" + itoa(e.strain) + "/seeds"
	for _, n := range []any{0, -5, 101, 1e9, 2.5} {
		code, out := e.json(t, "POST", path, map[string]any{"count": n})
		assertRefused(t, code, out, "seed count")
	}
	code, out := e.json(t, "POST", path, map[string]any{"count": 3, "seed_location": "Under the floorboards"})
	assertRefused(t, code, out, "a new seed location is typed text")
	code, _ = e.json(t, "POST", path, map[string]any{"count": 3})
	assert.Equal(t, http.StatusOK, code)
}

func TestDemo_AddPlantIsNamedByTheServer(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	var veg int
	require.NoError(t, e.server.DB.QueryRow(`SELECT id FROM plant_status WHERE status = 'Veg'`).Scan(&veg))
	base := map[string]any{"name": "<b>mine</b>", "strain_id": e.strain, "zone_id": e.zone, "status_id": veg, "date": "2026-10-01", "sensors": "[]"}
	code, out := e.json(t, "POST", "/plants", base)
	require.Equal(t, http.StatusOK, code, out)
	var living []struct {
		Name string `json:"name"`
	}
	e.getJSON(t, "/plants/living", &living)
	names := []string{}
	for _, p := range living {
		names = append(names, p.Name)
	}
	assert.Contains(t, names, "Permanent Marker #2", "named by the server, in the visitor's copy")

	for k, v := range map[string]any{"new_zone": "Evil tent", "new_strain": map[string]any{"name": "x"}} {
		bad := map[string]any{}
		for bk, bv := range base {
			bad[bk] = bv
		}
		bad[k] = v
		code, out := e.json(t, "POST", "/plants", bad)
		assertRefused(t, code, out, k)
	}
}

func TestDemo_StrainEditKeepsTheText(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	code, _ := e.json(t, "PUT", "/strains/"+itoa(e.strain), map[string]any{
		"name": "Hacked", "description": "<img src=x onerror=alert(1)>", "url": "http://evil.example",
		"indica": 70, "sativa": 30, "autoflower": false, "seed_count": 4, "cycle_time": 63,
		"growing": map[string]any{"height_indoor": "tall"},
	})
	require.Equal(t, http.StatusOK, code)
	var got struct {
		Name   string `json:"name"`
		Indica int    `json:"indica"`
	}
	e.getJSON(t, "/strains/"+itoa(e.strain), &got)
	assert.Equal(t, "Permanent Marker", got.Name, "the name can't change")
	assert.Equal(t, 70, got.Indica, "numbers can")

	code, out := e.json(t, "PUT", "/strains/"+itoa(e.strain), map[string]any{"indica": 170})
	assertRefused(t, code, out, "indica over 100")
}

func TestDemo_StrainLookupUsesTheRecording(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	resp := e.client.Get("/strains/straincompass/search?q=Grape%20OG")
	var found struct {
		Results []struct {
			Slug, Name string
		} `json:"results"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&found))
	testutil.DrainAndClose(resp)
	require.NotEmpty(t, found.Results, "answered from the recording, offline")

	code, out := e.json(t, "POST", "/strains/straincompass/import", map[string]any{"slug": "not-recorded", "name": "x"})
	assertRefused(t, code, out, "a slug outside the recording")

	code, out = e.json(t, "POST", "/strains/straincompass/import", map[string]any{"slug": "grape-og", "name": "Anything"})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, "Grape OG", out["name"], "the name comes from the recording")
}

func TestDemo_SmokeReportsUseBuiltInChoices(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	report := func(extra map[string]any) map[string]any {
		r := map[string]any{"source": "grown", "plant_id": e.plant, "strain_id": e.strain, "strain_name": "Permanent Marker",
			"smoked_on": "2026-10-10", "score_bud": 8, "score_taste": 9, "notes": "Smooth, sweet grape finish."}
		for k, v := range extra {
			r[k] = v
		}
		return r
	}
	code, out := e.json(t, "POST", "/smoke", report(nil))
	require.Equal(t, http.StatusOK, code, out)

	for name, extra := range map[string]map[string]any{
		"note too long":  {"notes": strings.Repeat("x", handlers.DemoMaxNote+1)},
		"custom tag":     {"tags": []any{map[string]any{"kind": "effects", "name": "Hacked", "custom": true}}},
		"score over 10":  {"score_bud": 11},
		"typed strain":   {"strain_name": "Made Up Kush"},
		"typed lights":   {"lights": "1000W of evil"},
		"bad percentage": {"thc": "22; DROP TABLE strain"},
	} {
		code, out := e.json(t, "POST", "/smoke", report(extra))
		assertRefused(t, code, out, name)
	}
}

func TestDemo_LimitsChangesPerVisitor(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	path := "/strains/" + itoa(e.strain) + "/wanted"
	last := 0
	for i := 0; i <= handlers.DemoWritesPerMinute; i++ {
		last, _ = e.json(t, "POST", path, map[string]any{"wanted": i%2 == 0})
	}
	assert.Equal(t, http.StatusTooManyRequests, last)
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestDemo_EachVisitorHasTheirOwnCopy(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	other := e.server.NewClient(t)
	other.FetchMetaCSRFToken("/plants")
	seeds := func(c *testutil.Client) int {
		var s struct {
			SeedCount int `json:"seed_count"`
		}
		resp := c.Get("/strains/" + itoa(e.strain))
		defer testutil.DrainAndClose(resp)
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&s))
		return s.SeedCount
	}
	before := seeds(other)
	assert.Zero(t, e.server.DemoCopies.Count(), "reading makes no copy")

	code, _ := e.json(t, "POST", "/strains/"+itoa(e.strain)+"/seeds", map[string]any{"count": 3})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, e.server.DemoCopies.Count(), "the first change makes this visitor's copy")
	assert.Equal(t, before+3, seeds(e.client), "the visitor sees their change")
	assert.Equal(t, before, seeds(other), "another visitor doesn't")

	var base int
	require.NoError(t, e.server.DB.QueryRow(`SELECT seed_count FROM strain WHERE id = $1`, e.strain).Scan(&base))
	assert.Equal(t, before, base, "the shared demo is never written")
}

func TestDemo_LimitsNewCopiesPerAddress(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	path := "/strains/" + itoa(e.strain) + "/wanted"
	last := 0
	for i := 0; i <= demo.CopiesPerIPPerHour; i++ {
		c := e.server.NewClient(t) // a new visitor each time, from the same address
		token := c.FetchMetaCSRFToken("/plants")
		resp := c.SessionPostJSON(t, path, token, map[string]any{"wanted": true})
		last = resp.StatusCode
		testutil.DrainAndClose(resp)
	}
	assert.Equal(t, http.StatusTooManyRequests, last)
	assert.Equal(t, demo.CopiesPerIPPerHour, e.server.DemoCopies.Count())
}
