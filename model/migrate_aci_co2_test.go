package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 036 multiplies stored AC Infinity CO2 (sensorType 11) by 100:
// before 1.0.1 it was divided by 100 like the x100 temperature values.
func TestMigration036_ScalesACInfinityCO2Only(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(35))

	exec := func(q string, args ...any) {
		t.Helper()
		_, err := db.Exec(q, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO sensors (id, name, source, device, type, unit) VALUES (1, 'CO2', 'acinfinity', 'DEV', 'ACI.2.11', 'ppm')`)
	exec(`INSERT INTO sensors (id, name, source, device, type, unit) VALUES (2, 'Inside temp', 'acinfinity', 'DEV', 'ACI.1.0', '°F')`)
	exec(`INSERT INTO sensors (id, name, source, device, type, unit) VALUES (3, 'Custom CO2', 'custom', 'ESP', 'ACI.2.11', 'ppm')`)
	exec(`INSERT INTO sensor_data (sensor_id, value) VALUES (1, 7.7), (2, 72.6), (3, 650)`)
	exec(`INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES (1, '2026-10-14 10:00:00', 4.7, 14.5, 7.0, 60)`)
	exec(`INSERT OR REPLACE INTO rolling_averages (sensor_id, avg_value) VALUES (1, 7.5)`)

	require.NoError(t, m.Migrate(36))

	value := func(id int) float64 {
		var v float64
		require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE sensor_id = $1`, id).Scan(&v))
		return v
	}
	assert.InDelta(t, 770, value(1), 0.001, "AC Infinity CO2 back to ppm")
	assert.InDelta(t, 72.6, value(2), 0.001, "other AC Infinity sensors untouched")
	assert.InDelta(t, 650, value(3), 0.001, "only AC Infinity's source")

	var lo, hi, avg, roll float64
	require.NoError(t, db.QueryRow(`SELECT min_val, max_val, avg_val FROM sensor_data_hourly WHERE sensor_id = 1`).Scan(&lo, &hi, &avg))
	assert.InDelta(t, 470, lo, 0.001)
	assert.InDelta(t, 1450, hi, 0.001)
	assert.InDelta(t, 700, avg, 0.001)
	require.NoError(t, db.QueryRow(`SELECT avg_value FROM rolling_averages WHERE sensor_id = 1`).Scan(&roll))
	assert.InDelta(t, 750, roll, 0.001)
}
