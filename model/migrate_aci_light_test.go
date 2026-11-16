package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 051 multiplies stored AC Infinity light sensor readings
// (sensorType 12) by 10: before 2.8 they were divided by 100, not by 10, so
// 76% was saved as 7.6. Limits on that sensor scale with it, and a missing or
// hand-typed "on/off" unit becomes "%".
func TestMigration051_ScalesACInfinityLightOnly(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(50))

	exec := func(q string, args ...any) {
		t.Helper()
		_, err := db.Exec(q, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO sensors (id, name, source, device, type, unit) VALUES (1, 'Light', 'acinfinity', 'DEV', 'ACI.2.12', 'on/off')`)
	exec(`INSERT INTO sensors (id, name, source, device, type, unit) VALUES (2, 'CO2', 'acinfinity', 'DEV', 'ACI.2.11', 'ppm')`)
	exec(`INSERT INTO sensors (id, name, source, device, type, unit) VALUES (3, 'Other light', 'custom', 'ESP', 'ACI.2.12', 'lx')`)
	exec(`INSERT INTO sensors (id, name, source, device, type, unit) VALUES (4, 'Light 2', 'acinfinity', 'DEV2', 'ACI.3.12', 'brightness')`)
	exec(`INSERT INTO sensor_data (sensor_id, value) VALUES (1, 7.6), (2, 588), (3, 300), (4, 10)`)
	exec(`INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES (1, '2026-11-16 10:00:00', 0, 10, 7.5, 60)`)
	exec(`INSERT OR REPLACE INTO rolling_averages (sensor_id, avg_value) VALUES (1, 7.4)`)
	exec(`INSERT INTO alert_rule (sensor_id, side, limit_value) VALUES (1, 'low', 5), (2, 'high', 1500)`)
	exec(`INSERT INTO zones (id, name, light_sensor_id, light_on_above) VALUES (1, 'Tent', 1, 0.5), (2, 'Other', 2, 0.5)`)

	require.NoError(t, m.Migrate(51))

	value := func(id int) float64 {
		var v float64
		require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE sensor_id = $1`, id).Scan(&v))
		return v
	}
	assert.InDelta(t, 76, value(1), 0.001, "AC Infinity light now 0-100%")
	assert.InDelta(t, 100, value(4), 0.001, "on any port and device")
	assert.InDelta(t, 588, value(2), 0.001, "other AC Infinity sensors untouched")
	assert.InDelta(t, 300, value(3), 0.001, "only AC Infinity's source")

	var lo, hi, avg, roll float64
	require.NoError(t, db.QueryRow(`SELECT min_val, max_val, avg_val FROM sensor_data_hourly WHERE sensor_id = 1`).Scan(&lo, &hi, &avg))
	assert.InDelta(t, 0, lo, 0.001)
	assert.InDelta(t, 100, hi, 0.001)
	assert.InDelta(t, 75, avg, 0.001)
	require.NoError(t, db.QueryRow(`SELECT avg_value FROM rolling_averages WHERE sensor_id = 1`).Scan(&roll))
	assert.InDelta(t, 74, roll, 0.001)

	var limit1, limit2, above1, above2 float64
	require.NoError(t, db.QueryRow(`SELECT limit_value FROM alert_rule WHERE sensor_id = 1`).Scan(&limit1))
	require.NoError(t, db.QueryRow(`SELECT limit_value FROM alert_rule WHERE sensor_id = 2`).Scan(&limit2))
	assert.InDelta(t, 50, limit1, 0.001, "sensor alert limit follows the reading")
	assert.InDelta(t, 1500, limit2, 0.001)
	require.NoError(t, db.QueryRow(`SELECT light_on_above FROM zones WHERE id = 1`).Scan(&above1))
	require.NoError(t, db.QueryRow(`SELECT light_on_above FROM zones WHERE id = 2`).Scan(&above2))
	assert.InDelta(t, 5, above1, 0.001, "light check threshold follows the reading")
	assert.InDelta(t, 0.5, above2, 0.001)

	unit := func(id int) string {
		var u string
		require.NoError(t, db.QueryRow(`SELECT unit FROM sensors WHERE id = $1`, id).Scan(&u))
		return u
	}
	assert.Equal(t, "%", unit(1), `the typed "on/off" becomes %`)
	assert.Equal(t, "brightness", unit(4), "a unit the owner chose otherwise is kept")
	assert.Equal(t, "lx", unit(3))
}
