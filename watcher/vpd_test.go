package watcher

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
	"isley/utils"
)

// A temperature sensor whose unit is spelled out in lower case ("celsius",
// as some ingest scripts send it) used to count as Fahrenheit, so 25 °C was
// read as 25 °F (-3.9 °C) and the zone's VPD came out far too low.
func TestComputeZoneVPD_TemperatureUnitSpellings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		unit  string
		temp  float64
		wantC float64
	}{
		{"°C", 25, 25},
		{"celsius", 25, 25},
		{"Celsius", 25, 25},
		{"c", 25, 25},
		{"°F", 77, 25},
		{"fahrenheit", 77, 25},
	}
	for _, tc := range cases {
		t.Run(tc.unit, func(t *testing.T) {
			t.Parallel()
			db := testutil.NewTestDB(t)
			zoneID := testutil.SeedZone(t, db, "Tent")
			tempID := seedSensor(t, db, "api", "probe", "temp")
			humID := seedSensor(t, db, "api", "probe", "hum")
			_, err := db.Exec(`UPDATE sensors SET unit = $1 WHERE id = $2`, tc.unit, tempID)
			require.NoError(t, err)
			_, err = db.Exec(`UPDATE zones SET leaf_temp_offset = 0, vpd_temp_sensor_id = $1, vpd_humidity_sensor_id = $2 WHERE id = $3`,
				tempID, humID, zoneID)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2), ($3, 50)`, tempID, tc.temp, humID)
			require.NoError(t, err)

			newTestWatcher(t, db).computeZoneVPD(context.Background())

			var vpd float64
			require.NoError(t, db.QueryRow(`
				SELECT sd.value FROM sensor_data sd JOIN sensors s ON s.id = sd.sensor_id
				WHERE s.source = 'derived' AND s.type = 'VPD'`).Scan(&vpd))
			assert.InDelta(t, utils.VPD(tc.wantC, 50, 0), vpd, 0.01)
		})
	}
}
