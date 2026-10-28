package demo_test

import (
	"database/sql"
	"fmt"
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

func TestRollDates_KeepsTheGrowAtTheSamePoint(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S")
	plant := testutil.SeedPlant(t, db, "P", strain, testutil.SeedZone(t, db, "Z"))
	exec := func(q string, args ...any) {
		t.Helper()
		_, err := db.Exec(q, args...)
		require.NoError(t, err)
	}
	exec(`UPDATE plant SET start_dt = '2026-01-01' WHERE id = $1`, plant)
	exec(`UPDATE strain SET seeds_added_on = '2025-12-01' WHERE id = $1`, strain)
	exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, 1, '2026-01-01T09:00:00')`, plant)
	exec(`INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES ($1, '2026-01-02', 'n')`, plant)
	// Ten days later: moving rows one at a time, the first would land on this one.
	exec(`INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES ($1, '2026-01-12', 'm')`, plant)
	exec(`INSERT INTO smoke_report (strain_name, smoked_on, harvested_on) VALUES ('S', '2026-01-02', '')`)
	exec(`INSERT INTO settings (name, value) VALUES ('demo_dates_on', '2026-01-03')`)

	today := time.Date(2026, 1, 13, 15, 0, 0, 0, time.Local)
	days, err := demo.RollDates(db, today)
	require.NoError(t, err)
	assert.Equal(t, 10, days)

	one := func(q string, args ...any) string {
		t.Helper()
		var v string
		require.NoError(t, db.QueryRow(q, args...).Scan(&v))
		return v
	}
	assert.Equal(t, "2026-01-11", one(`SELECT CAST(start_dt AS TEXT) FROM plant WHERE id = $1`, plant))
	assert.Equal(t, "2025-12-11", one(`SELECT CAST(seeds_added_on AS TEXT) FROM strain WHERE id = $1`, strain))
	assert.Equal(t, "2026-01-11T09:00:00", one(`SELECT CAST(date AS TEXT) FROM plant_status_log WHERE plant_id = $1`, plant), "same format")
	assert.Equal(t, "2026-01-12", one(`SELECT CAST(entry_date AS TEXT) FROM plant_day_entry WHERE note = 'n'`))
	assert.Equal(t, "2026-01-22", one(`SELECT CAST(entry_date AS TEXT) FROM plant_day_entry WHERE note = 'm'`))
	assert.Equal(t, "2026-01-12", one(`SELECT CAST(smoked_on AS TEXT) FROM smoke_report`))
	assert.Equal(t, "", one(`SELECT CAST(harvested_on AS TEXT) FROM smoke_report`), "an empty date stays empty")
	assert.Equal(t, "2026-01-13", one(`SELECT value FROM settings WHERE name = 'demo_dates_on'`))

	days, err = demo.RollDates(db, today.Add(5*time.Hour))
	require.NoError(t, err)
	assert.Zero(t, days, "once a day")
	assert.Equal(t, "2026-01-11", one(`SELECT CAST(start_dt AS TEXT) FROM plant WHERE id = $1`, plant))
}

func TestRollDates_SnapshotWithoutItsDay(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	days, err := demo.RollDates(db, time.Now())
	require.NoError(t, err)
	assert.Zero(t, days, "nothing to go by: counting starts today")
	var v string
	require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE name = 'demo_dates_on'`).Scan(&v))
	assert.Equal(t, time.Now().Format("2006-01-02"), v)
}

// TestForRequest_IPv6CountsPerSlash64: addresses in one IPv6 /64 share
// the per-address limit, so rotating them can't push other visitors'
// copies out.
func TestForRequest_IPv6CountsPerSlash64(t *testing.T) {
	t.Parallel()
	m, _ := newCopies(t, 50)
	for i := 0; i < demo.CopiesPerIPPerHour; i++ {
		_, _, _, err := m.ForRequest("", fmt.Sprintf("2001:db8:1:2::%x", i+1), true)
		require.NoError(t, err)
	}
	_, _, _, err := m.ForRequest("", "2001:db8:1:2::ffff", true)
	assert.ErrorIs(t, err, demo.ErrBusy)
	_, _, _, err = m.ForRequest("", "2001:db8:1:3::1", true)
	assert.NoError(t, err, "another /64 is another visitor")
}
