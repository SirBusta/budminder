package model

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 033 replaces the SQLite update_rolling_avg trigger, which
// recomputed a window over a sensor's whole history on every reading, with
// one that reads only the 16 readings before the new one. The stored
// average must not change.

func rollingTriggerSQL(t *testing.T, db *sql.DB) string {
	t.Helper()
	var s string
	require.NoError(t, db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'update_rolling_avg'`).Scan(&s))
	return s
}

// insertReadings adds n readings (values 1..n, a minute apart) for a sensor
// and returns the sensor's rolling average afterwards.
func insertReadings(t *testing.T, db *sql.DB, sensorID, n int) sql.NullFloat64 {
	t.Helper()
	for i := 1; i <= n; i++ {
		_, err := db.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, datetime('2026-10-01 00:00:00', $3))`,
			sensorID, float64(i), "+"+strconv.Itoa(i)+" minutes")
		require.NoError(t, err)
	}
	var avg sql.NullFloat64
	require.NoError(t, db.QueryRow(`SELECT avg_value FROM rolling_averages WHERE sensor_id = $1`, sensorID).Scan(&avg))
	return avg
}

func seedSensors(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO sensors (id, name, source, device, type) VALUES (1, 'A', 's', 'd', 'a')`,
		`INSERT INTO sensors (id, name, source, device, type) VALUES (2, 'B', 's', 'd', 'b')`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
}

func TestMigration033_RollingAverageSameResultWithoutFullScan(t *testing.T) {
	t.Parallel()
	initTestLogger()

	// Old trigger (schema at 032) and new trigger give the same averages.
	results := map[string][3]sql.NullFloat64{}
	for name, version := range map[string]uint{"old": 32, "new": 33} {
		db := freshSQLite(t)
		require.NoError(t, newSQLiteMigrator(t, db).Migrate(version))
		seedSensors(t, db)
		first := insertReadings(t, db, 2, 1)
		long := insertReadings(t, db, 1, 40)
		short := insertReadings(t, db, 2, 5) // sensor 2 now has 6 readings: 1, then 1..5
		results[name] = [3]sql.NullFloat64{first, long, short}
		if name == "new" {
			assert.NotContains(t, rollingTriggerSQL(t, db), "OVER", "no window over the whole history")
		}
	}
	old, cur := results["old"], results["new"]
	assert.False(t, cur[0].Valid, "a sensor's first reading has no earlier readings to average")
	assert.Equal(t, old[0], cur[0])
	require.True(t, cur[1].Valid)
	assert.InDelta(t, 31.5, cur[1].Float64, 1e-9, "average of the 16 readings before the 40th (24..39)")
	assert.InDelta(t, old[1].Float64, cur[1].Float64, 1e-9)
	assert.InDelta(t, old[2].Float64, cur[2].Float64, 1e-9, "other sensors' readings are not mixed in")
}

func TestMigration033_Down(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(33))
	require.NoError(t, m.Migrate(32))
	assert.Contains(t, rollingTriggerSQL(t, db), "OVER", "down restores the original trigger")
	require.NoError(t, m.Migrate(33), "migrating up again works")
	assert.NotContains(t, rollingTriggerSQL(t, db), "OVER")
}
