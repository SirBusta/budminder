package handlers_test

// Built-in (locked) activity types and metrics: Water, Feed, Note and
// Height keep their names (and Water/Feed their watering/feeding flags)
// and can't be deleted; their look, measurements and unit can change.
// Other types can't be renamed to a built-in name.

import (
	"database/sql"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

const lockedTestPassword = "locked-test-pw"

type lockedActivity struct {
	id                    int
	name                  string
	isWatering, isFeeding bool
}

func builtinActivity(t *testing.T, db *sql.DB, name string) lockedActivity {
	t.Helper()
	a := lockedActivity{name: name}
	require.NoError(t, db.QueryRow(
		`SELECT id, is_watering, is_feeding FROM activity WHERE name = $1 AND lock = TRUE`, name).
		Scan(&a.id, &a.isWatering, &a.isFeeding))
	return a
}

func lockedEnv(t *testing.T) (*sql.DB, *testutil.Client, string) {
	t.Helper()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, lockedTestPassword)
	server := testutil.NewTestServer(t, db)
	c, token := server.LoginAndFetchCSRF(t, lockedTestPassword, "/settings")
	return db, c, token
}

func TestLockedActivity_LookAndMeasurementsCanChange(t *testing.T) {
	t.Parallel()
	db, c, token := lockedEnv(t)
	feed := builtinActivity(t, db, "Feed")

	resp := c.SessionPutJSON(t, "/activities/"+strconv.Itoa(feed.id), token, map[string]any{
		"activity_name": feed.name, "is_watering": feed.isWatering, "is_feeding": feed.isFeeding,
		"icon": "fa-flask", "color": "#123456",
	})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var color string
	require.NoError(t, db.QueryRow(`SELECT color FROM activity WHERE id = $1`, feed.id).Scan(&color))
	assert.Equal(t, "#123456", color)
}

func TestLockedActivity_NameAndFlagsCantChange(t *testing.T) {
	t.Parallel()
	db, c, token := lockedEnv(t)
	water := builtinActivity(t, db, "Water")

	cases := map[string]map[string]any{
		"rename":      {"activity_name": "Irrigate", "is_watering": water.isWatering, "is_feeding": water.isFeeding},
		"watering":    {"activity_name": water.name, "is_watering": !water.isWatering, "is_feeding": water.isFeeding},
		"feeding":     {"activity_name": water.name, "is_watering": water.isWatering, "is_feeding": !water.isFeeding},
		"letter case": {"activity_name": "water", "is_watering": water.isWatering, "is_feeding": water.isFeeding},
	}
	for name, body := range cases {
		resp := c.SessionPutJSON(t, "/activities/"+strconv.Itoa(water.id), token, body)
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, name)
	}

	got := builtinActivity(t, db, "Water")
	assert.Equal(t, water, got, "the built-in type is unchanged")
}

func TestLockedActivity_CantBeDeleted(t *testing.T) {
	t.Parallel()
	db, c, token := lockedEnv(t)
	note := builtinActivity(t, db, "Note")

	resp := c.SessionDeleteJSON(t, "/activities/"+strconv.Itoa(note.id), token, nil)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity WHERE id = $1`, note.id).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestActivity_CantBeRenamedToReservedName(t *testing.T) {
	t.Parallel()
	db, c, token := lockedEnv(t)
	var id int
	require.NoError(t, db.QueryRow(`INSERT INTO activity (name, lock) VALUES ('Mine', FALSE) RETURNING id`).Scan(&id))

	for _, name := range []string{"Water", "Feed", "Note", "Height", " height "} {
		resp := c.SessionPutJSON(t, "/activities/"+strconv.Itoa(id), token, map[string]any{"activity_name": name})
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%q is reserved", name)
	}

	var got string
	require.NoError(t, db.QueryRow(`SELECT name FROM activity WHERE id = $1`, id).Scan(&got))
	assert.Equal(t, "Mine", got)
}

func TestActivity_UnknownIDIsNotFound(t *testing.T) {
	t.Parallel()
	_, c, token := lockedEnv(t)

	resp := c.SessionPutJSON(t, "/activities/99999", token, map[string]any{"activity_name": "Ghost"})
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	resp = c.SessionDeleteJSON(t, "/activities/99999", token, nil)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestLockedMetric_UnitChangesNameDoesNot(t *testing.T) {
	t.Parallel()
	db, c, token := lockedEnv(t)
	var id int
	require.NoError(t, db.QueryRow(`SELECT id FROM metric WHERE name = 'Height' AND lock = TRUE`).Scan(&id))
	path := "/metrics/" + strconv.Itoa(id)

	resp := c.SessionPutJSON(t, path, token, map[string]any{"metric_name": "Height", "metric_unit": "cm"})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the unit of a built-in metric can change")

	resp = c.SessionPutJSON(t, path, token, map[string]any{"metric_name": "Stretch", "metric_unit": "cm"})
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "its name can't")

	var name, unit string
	require.NoError(t, db.QueryRow(`SELECT name, unit FROM metric WHERE id = $1`, id).Scan(&name, &unit))
	assert.Equal(t, "Height", name)
	assert.Equal(t, "cm", unit)

	resp = c.SessionPutJSON(t, "/metrics/99999", token, map[string]any{"metric_name": "Ghost"})
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetActivitiesAndMetrics_CarryLock(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)

	// The Settings edit window reads the flag from these rows.
	for _, a := range handlers.GetActivities(db) {
		switch a.Name {
		case "Water", "Feed", "Note", "Height":
			assert.True(t, a.Lock, a.Name)
		case "Training", "Defoliate":
			assert.False(t, a.Lock, a.Name)
		}
	}
	for _, m := range handlers.GetMetrics(db) {
		assert.Equal(t, m.Name == "Height", m.Lock, m.Name)
	}
}
