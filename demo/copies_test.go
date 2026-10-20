package demo_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/demo"
	"isley/tests/testutil"
)

const layout = "2006-01-02 15:04:05"

func TestNew_BringsReadingsUpToNowAndTrims(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	sensor := testutil.SeedSensor(t, db, "acinfinity", "D", "ACI.1.0")
	now := time.Now().UTC().Truncate(time.Minute)
	// Readings every 10 minutes from 3 days ago until 3 hours ago; the value
	// is the minute of the day, so a repeat is easy to check.
	for at := now.Add(-72 * time.Hour); !at.After(now.Add(-3 * time.Hour)); at = at.Add(10 * time.Minute) {
		_, err := db.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`,
			sensor, at.Hour()*60+at.Minute(), at.Format(layout))
		require.NoError(t, err)
	}

	_, err := demo.New(db, "", config.NewStore(), t.TempDir(), 0)
	require.NoError(t, err)

	var newest, oldest string
	require.NoError(t, db.QueryRow(`SELECT MAX(create_dt), MIN(create_dt) FROM sensor_data`).Scan(&newest, &oldest))
	n, _ := time.Parse(layout, newest)
	o, _ := time.Parse(layout, oldest)
	assert.WithinDuration(t, now, n, 10*time.Minute, "readings reach the present")
	assert.WithinDuration(t, now.Add(-48*time.Hour), o, 11*time.Minute, "only two days of per-minute readings are kept")

	var v float64
	require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE create_dt = $1`, newest).Scan(&v))
	assert.Equal(t, float64(n.Hour()*60+n.Minute()), v, "a repeat of the reading from a day earlier")
}

func newCopies(t *testing.T, max int) (*demo.Copies, *sql.DB) {
	t.Helper()
	db := testutil.NewTestDB(t)
	m, err := demo.New(db, "", config.NewStore(), t.TempDir(), max)
	require.NoError(t, err)
	t.Cleanup(m.Close)
	return m, db
}

func TestForRequest_CopiesOnlyForChanges(t *testing.T) {
	t.Parallel()
	m, base := newCopies(t, 0)
	id, db, _, err := m.ForRequest("", "1.1.1.1", false)
	require.NoError(t, err)
	assert.Empty(t, id)
	assert.Same(t, base, db, "a reader gets the shared demo")

	id, db, store, err := m.ForRequest("", "1.1.1.1", true)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	assert.NotSame(t, base, db)
	assert.NotNil(t, store)
	again, db2, _, err := m.ForRequest(id, "1.1.1.1", false)
	require.NoError(t, err)
	assert.Equal(t, id, again)
	assert.Same(t, db, db2, "the same visitor keeps their copy")

	_, err = db.Exec(`INSERT INTO zones (name) VALUES ('Only mine')`)
	require.NoError(t, err)
	var n int
	require.NoError(t, base.QueryRow(`SELECT COUNT(*) FROM zones WHERE name = 'Only mine'`).Scan(&n))
	assert.Zero(t, n, "a copy's change never reaches the shared demo")
}

func TestTick_DropsIdleCopies(t *testing.T) {
	t.Parallel()
	m, _ := newCopies(t, 0)
	id, _, _, err := m.ForRequest("", "1.1.1.1", true)
	require.NoError(t, err)
	m.Tick(time.Now().Add(demo.Idle-time.Minute), false)
	assert.Equal(t, 1, m.Count(), "still in use")
	m.Tick(time.Now().Add(demo.Idle+time.Minute), false)
	assert.Zero(t, m.Count(), "idle for over an hour")
	newID, _, _, err := m.ForRequest(id, "1.1.1.1", false)
	require.NoError(t, err)
	assert.Empty(t, newID, "the visitor is back on the shared demo")
}

func TestForRequest_CapDropsTheOldest(t *testing.T) {
	t.Parallel()
	m, _ := newCopies(t, 2)
	first, _, _, err := m.ForRequest("", "1.1.1.1", true)
	require.NoError(t, err)
	for _, ip := range []string{"2.2.2.2", "3.3.3.3"} {
		_, _, _, err := m.ForRequest("", ip, true)
		require.NoError(t, err)
	}
	assert.Equal(t, 2, m.Count())
	id, _, _, _ := m.ForRequest(first, "1.1.1.1", false)
	assert.Empty(t, id, "the oldest copy went first")
}
