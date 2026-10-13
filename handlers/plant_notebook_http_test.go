package handlers_test

// HTTP tests for the plant notebook: saving a day (PUT /plant/:id/day/:date),
// deleting a day, logging several plants at once (POST /plants/day), the
// read endpoints, and the per-day exports. Mutations go through the real
// session + CSRF round-trip, like the page does.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/model/types"
	"isley/tests/testutil"
)

const notebookPassword = "notebook-pw"

func notebookServer(t *testing.T) (*sql.DB, notebookFixture, *testutil.Client, string) {
	t.Helper()
	db := testutil.NewTestDB(t)
	f := seedNotebookFixture(t, db)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, notebookPassword)
	c, token := server.LoginAndFetchCSRF(t, notebookPassword, "/plants")
	return db, f, c, token
}

func f64(v float64) *float64 { return &v }

func getNotebook(t *testing.T, c *testutil.Client, plantID int) []types.NotebookDay {
	t.Helper()
	resp := c.Get(fmt.Sprintf("/plant/%d/notebook", plantID))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		Days []types.NotebookDay `json:"days"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.Days
}

func countRows(t *testing.T, db *sql.DB, query string, args ...interface{}) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(query, args...).Scan(&n))
	return n
}

func TestNotebookHTTP_AuthGating(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	c := server.NewClient(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPut, "/plant/1/day/2026-10-10"},
		{http.MethodDelete, "/plant/1/day/2026-10-10"},
		{http.MethodPost, "/plants/day"},
	} {
		req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, strings.NewReader(`{}`))
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		testutil.DrainAndClose(resp)
		assert.Containsf(t, []int{http.StatusUnauthorized, http.StatusForbidden}, resp.StatusCode, "%s %s", tc.method, tc.path)
	}
}

func TestNotebookHTTP_SaveEditAndClearDay(t *testing.T) {
	t.Parallel()
	db, f, c, token := notebookServer(t)
	path := fmt.Sprintf("/plant/%d/day/2026-10-07", f.Plant1)

	// Water + Height 16 + a note.
	resp := c.SessionPutJSON(t, path, token, map[string]interface{}{
		"activity_ids": []int{f.WaterID, f.HeightID},
		"note":         "  Stretch slowing down.  ",
		"measurements": []map[string]interface{}{{"activity_id": f.HeightID, "metric_id": f.HeightMetric, "value": 16}},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var saved struct {
		Day *types.NotebookDay `json:"day"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&saved))
	resp.Body.Close()
	require.NotNil(t, saved.Day)
	assert.Equal(t, "Stretch slowing down.", saved.Day.Note, "note is trimmed")
	assert.True(t, saved.Day.HasEntry)
	assert.ElementsMatch(t, []int{f.WaterID, f.HeightID}, typeIDs(saved.Day))
	require.Len(t, saved.Day.Measurements, 1)
	assert.Equal(t, 16.0, saved.Day.Measurements[0].Value)
	measurementID := saved.Day.Measurements[0].ID
	for _, r := range saved.Day.Rows {
		assert.Regexp(t, `^\d\d:\d\d:\d\d$`, r.Time, "time recorded to the second")
	}

	// Untick Water, change the height and the note.
	resp = c.SessionPutJSON(t, path, token, map[string]interface{}{
		"activity_ids": []int{f.HeightID},
		"note":         "Stretch done.",
		"measurements": []map[string]interface{}{{"activity_id": f.HeightID, "metric_id": f.HeightMetric, "value": 17.5}},
	})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	d := findDay(getNotebook(t, c, f.Plant1), f.Plant1, "2026-10-07")
	require.NotNil(t, d)
	assert.Equal(t, []int{f.HeightID}, typeIDs(d))
	assert.Equal(t, "Stretch done.", d.Note)
	require.Len(t, d.Measurements, 1)
	assert.Equal(t, 17.5, d.Measurements[0].Value)
	assert.Equal(t, measurementID, d.Measurements[0].ID, "the number is updated in place")
	assert.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM plant_activity WHERE plant_id = $1`, f.Plant1))

	// Nothing ticked, no note: the day disappears.
	resp = c.SessionPutJSON(t, path, token, map[string]interface{}{"activity_ids": []int{}, "note": ""})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, findDay(getNotebook(t, c, f.Plant1), f.Plant1, "2026-10-07"))
	assert.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM plant_measurements`))
	assert.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM plant_day_entry`))
}

func TestNotebookHTTP_SaveValidation(t *testing.T) {
	t.Parallel()
	_, f, c, token := notebookServer(t)

	cases := []struct {
		name, path string
		body       map[string]interface{}
		want       int
	}{
		{"height needs its number", fmt.Sprintf("/plant/%d/day/2026-10-07", f.Plant1),
			map[string]interface{}{"activity_ids": []int{f.HeightID}}, http.StatusBadRequest},
		{"bad date", fmt.Sprintf("/plant/%d/day/2026-13-40", f.Plant1),
			map[string]interface{}{"activity_ids": []int{f.WaterID}}, http.StatusBadRequest},
		{"unknown activity type", fmt.Sprintf("/plant/%d/day/2026-10-07", f.Plant1),
			map[string]interface{}{"activity_ids": []int{99999}}, http.StatusBadRequest},
		{"number for a metric the type doesn't record", fmt.Sprintf("/plant/%d/day/2026-10-07", f.Plant1),
			map[string]interface{}{"activity_ids": []int{f.WaterID},
				"measurements": []map[string]interface{}{{"activity_id": f.WaterID, "metric_id": f.HeightMetric, "value": 3}}}, http.StatusBadRequest},
		{"unknown plant", "/plant/99999/day/2026-10-07",
			map[string]interface{}{"activity_ids": []int{f.WaterID}}, http.StatusNotFound},
	}
	for _, tc := range cases {
		resp := c.SessionPutJSON(t, tc.path, token, tc.body)
		testutil.DrainAndClose(resp)
		assert.Equal(t, tc.want, resp.StatusCode, tc.name)
	}
}

func TestNotebookHTTP_EditingAnOldDayMovesItsText(t *testing.T) {
	t.Parallel()
	db, f, c, token := notebookServer(t)
	testutil.MustExec(t, db, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'Gave a gallon.', '2026-09-30T08:10:00')`, f.Plant1, f.WaterID)
	testutil.MustExec(t, db, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'Flipped to 12/12.', '2026-09-30 09:00:00')`, f.Plant1, f.NoteID)

	resp := c.SessionPutJSON(t, fmt.Sprintf("/plant/%d/day/2026-09-30", f.Plant1), token, map[string]interface{}{
		"activity_ids": []int{f.WaterID, f.NoteID},
		"note":         "Gave a gallon.\nFlipped to 12/12, lights on at 10 PM.",
	})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	d := findDay(getNotebook(t, c, f.Plant1), f.Plant1, "2026-09-30")
	require.NotNil(t, d)
	assert.True(t, d.HasEntry)
	assert.Equal(t, "Gave a gallon.\nFlipped to 12/12, lights on at 10 PM.", d.Note)
	assert.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM plant_activity WHERE note <> ''`), "old rows' notes are cleared")
	assert.Equal(t, 2, countRows(t, db, `SELECT COUNT(*) FROM plant_activity`), "kept ticks keep their rows")
}

func TestNotebookHTTP_StandaloneMeasurement(t *testing.T) {
	t.Parallel()
	db, f, c, token := notebookServer(t)
	testutil.MustExec(t, db, `INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, 11, '2026-09-30 12:00:00')`, f.Plant1, f.HeightMetric)
	path := fmt.Sprintf("/plant/%d/day/2026-09-30", f.Plant1)

	// Ticking Height on that day adopts the old number instead of adding one.
	resp := c.SessionPutJSON(t, path, token, map[string]interface{}{
		"activity_ids": []int{f.HeightID},
		"measurements": []map[string]interface{}{{"activity_id": f.HeightID, "metric_id": f.HeightMetric, "value": 12}},
	})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM plant_measurements`))
	assert.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM plant_measurements WHERE value = 12 AND plant_activity_id IS NOT NULL`))

	// Saving the day with Height unticked removes it.
	resp = c.SessionPutJSON(t, path, token, map[string]interface{}{"activity_ids": []int{f.WaterID}})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM plant_measurements`))
}

func TestNotebookHTTP_LogSeveralPlants(t *testing.T) {
	t.Parallel()
	db, f, c, token := notebookServer(t)
	// Plant 1 already watered and wrote a note that day.
	testutil.SeedActivityAt(t, db, f.Plant1, f.WaterID, "2026-10-05T07:00:00")
	testutil.MustExec(t, db, `INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES ($1, '2026-10-05', 'Morning check.')`, f.Plant1)

	resp := c.SessionPostJSON(t, "/plants/day", token, map[string]interface{}{
		"plant_ids":    []int{f.Plant1, f.Plant2, f.Plant2},
		"date":         "2026-10-05",
		"activity_ids": []int{f.WaterID},
		"note":         "Gave both a gallon.",
	})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	days := getNotebook(t, c, f.Plant1)
	d1 := findDay(days, f.Plant1, "2026-10-05")
	require.NotNil(t, d1)
	assert.Equal(t, "Morning check.\nGave both a gallon.", d1.Note, "text appended on a new line")
	assert.Equal(t, 2, d1.Types[0].Count, "a second watering is recorded twice")
	d2 := findDay(getNotebook(t, c, f.Plant2), f.Plant2, "2026-10-05")
	require.NotNil(t, d2)
	assert.Equal(t, "Gave both a gallon.", d2.Note)
	assert.Equal(t, 1, d2.Types[0].Count, "a plant listed twice is logged once")

	for _, body := range []map[string]interface{}{
		{"plant_ids": []int{}, "date": "2026-10-05", "activity_ids": []int{f.WaterID}},
		{"plant_ids": []int{f.Plant1}, "date": "2026-10-05"},
		{"plant_ids": []int{f.Plant1}, "date": "05/10/2026", "activity_ids": []int{f.WaterID}},
	} {
		resp := c.SessionPostJSON(t, "/plants/day", token, body)
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
	}
	resp = c.SessionPostJSON(t, "/plants/day", token, map[string]interface{}{"plant_ids": []int{99999}, "date": "2026-10-05", "activity_ids": []int{f.WaterID}})
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestNotebookHTTP_DeleteDayKeepsPhotos(t *testing.T) {
	t.Parallel()
	db, f, c, token := notebookServer(t)
	row := testutil.SeedActivityAt(t, db, f.Plant1, f.HeightID, "2026-09-25T08:00:00")
	testutil.MustExec(t, db, `INSERT INTO plant_measurements (plant_id, metric_id, value, date, plant_activity_id) VALUES ($1, $2, 9, '2026-09-25T08:00:00', $3)`, f.Plant1, f.HeightMetric, row)
	testutil.MustExec(t, db, `INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES ($1, '2026-09-25', 'Defoliated.')`, f.Plant1)
	testutil.MustExec(t, db, `INSERT INTO plant_images (plant_id, image_path, image_date) VALUES ($1, 'uploads/plants/x.jpg', '2026-09-25')`, f.Plant1)
	// Another day is untouched.
	testutil.SeedActivityAt(t, db, f.Plant1, f.WaterID, "2026-09-26T08:00:00")

	resp := c.SessionDeleteJSON(t, fmt.Sprintf("/plant/%d/day/2026-09-25", f.Plant1), token, nil)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	d := findDay(getNotebook(t, c, f.Plant1), f.Plant1, "2026-09-25")
	require.NotNil(t, d, "the photo keeps the day in the notebook")
	assert.Empty(t, d.Types)
	assert.Empty(t, d.Note)
	assert.Len(t, d.Photos, 1)
	assert.Equal(t, 0, countRows(t, db, `SELECT COUNT(*) FROM plant_measurements`))
	assert.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM plant_activity`))

	resp = c.SessionDeleteJSON(t, fmt.Sprintf("/plant/%d/day/not-a-date", f.Plant1), token, nil)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestNotebookHTTP_AllPlants(t *testing.T) {
	t.Parallel()
	db, f, c, _ := notebookServer(t)
	var veg, success int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = 'Veg'`).Scan(&veg))
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = 'Success'`).Scan(&success))
	testutil.MustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-08-19')`, f.Plant1, veg)
	testutil.MustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-08-19')`, f.Plant2, veg)
	testutil.MustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-09-30')`, f.Plant2, success)
	testutil.SeedActivityAt(t, db, f.Plant1, f.WaterID, "2026-10-01T08:00:00")
	testutil.SeedActivityAt(t, db, f.Plant2, f.WaterID, "2026-09-01T08:00:00")
	testutil.MustExec(t, db, `INSERT INTO plant_images (plant_id, image_path, image_date) VALUES ($1, 'uploads/a.jpg', '2026-09-01'), ($1, 'uploads/b.jpg', '2026-09-20')`, f.Plant1)

	type allBody struct {
		Plants []struct {
			ID      int    `json:"id"`
			Active  bool   `json:"active"`
			Status  string `json:"status"`
			Photo   string `json:"photo"`
			Start   string `json:"start"`
			History []struct {
				Status string `json:"status"`
				Date   string `json:"date"`
			} `json:"history"`
		} `json:"plants"`
		Days []types.NotebookDay `json:"days"`
	}
	get := func(path string) allBody {
		resp := c.Get(path)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var b allBody
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&b))
		return b
	}

	b := get("/notebook")
	require.Len(t, b.Plants, 2)
	assert.True(t, b.Plants[0].Active)
	assert.Equal(t, "/uploads/b.jpg", b.Plants[0].Photo, "latest photo")
	assert.Equal(t, "2026-01-01", b.Plants[0].Start)
	assert.False(t, b.Plants[1].Active, "harvested")
	assert.Equal(t, "Success", b.Plants[1].Status)
	require.Len(t, b.Plants[1].History, 2)
	assert.Equal(t, "Veg", b.Plants[1].History[0].Status, "history oldest first")
	for _, d := range b.Days {
		assert.Equal(t, f.Plant1, d.PlantID, "default: growing plants only")
	}

	b = get(fmt.Sprintf("/notebook?plant_ids=%d,%d", f.Plant1, f.Plant2))
	assert.NotNil(t, findDay(b.Days, f.Plant2, "2026-09-01"))
}

func TestNotebookHTTP_ExportOneRowPerDay(t *testing.T) {
	t.Parallel()
	db, f, c, token := notebookServer(t)
	resp := c.SessionPutJSON(t, fmt.Sprintf("/plant/%d/day/2026-10-07", f.Plant1), token, map[string]interface{}{
		"activity_ids": []int{f.WaterID, f.HeightID},
		"note":         "Stretch slowing down.",
		"measurements": []map[string]interface{}{{"activity_id": f.HeightID, "metric_id": f.HeightMetric, "value": 16}},
	})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.SeedActivityAt(t, db, f.Plant2, f.FeedID, "2026-10-01T08:00:00")
	// Photo-only days aren't exported.
	testutil.MustExec(t, db, `INSERT INTO plant_images (plant_id, image_path, image_date) VALUES ($1, 'uploads/a.jpg', '2026-09-01')`, f.Plant1)

	read := func(path string) (string, string) {
		resp := c.Get(path)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(body), resp.Header.Get("Content-Disposition")
	}

	csv, _ := read(fmt.Sprintf("/activities/export/csv?plant_id=%d", f.Plant1))
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	require.Len(t, lines, 2, csv)
	assert.Equal(t, "Date,Plant,Strain,Zone,Activities,Measurements,Note", strings.TrimSpace(lines[0]))
	assert.Contains(t, lines[1], "2026-10-07,Plant 1,S,Tent,")
	assert.Contains(t, lines[1], "Height 16 in")
	assert.Contains(t, lines[1], "Stretch slowing down.")

	csv, disp := read(fmt.Sprintf("/activities/export/csv?plant_ids=%d,%d&from=2026-10-01&to=2026-10-31", f.Plant1, f.Plant2))
	assert.Len(t, strings.Split(strings.TrimSpace(csv), "\n"), 3)
	assert.Contains(t, disp, "budminder-notebook-all-")
	csv, _ = read(fmt.Sprintf("/activities/export/csv?activity_id=%d", f.FeedID))
	assert.Len(t, strings.Split(strings.TrimSpace(csv), "\n"), 2, "only days with a feed")

	_, disp = read(fmt.Sprintf("/activities/export/xlsx?plant_id=%d", f.Plant1))
	assert.Contains(t, disp, "budminder-notebook-plant-1-")
}

func TestNotebookHTTP_PlantPageRenders(t *testing.T) {
	t.Parallel()
	db, f, c, _ := notebookServer(t)
	testutil.SeedActivityAt(t, db, f.Plant1, f.WaterID, "2026-10-01T08:00:00")

	resp := c.Get(fmt.Sprintf("/plant/%d", f.Plant1))
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	page := string(body)
	assert.Contains(t, page, `id="plantPage"`)
	assert.Contains(t, page, "pp_notebook", "the page's translated strings are embedded")
	assert.Contains(t, page, "2026-10-01", "the notebook days are embedded")
	assert.Contains(t, page, "/static/js/plant-page.js")
	assert.NotContains(t, page, `href="/activities"`, "the Activity Log menu item is gone")

	noFollow := *c.Client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err = noFollow.Get(c.BaseURL + "/plant/99999")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/plants", resp.Header.Get("Location"))
}
