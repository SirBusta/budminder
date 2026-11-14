package handlers_test

// Zones whose climate limits are never checked (handlers/climate_unchecked.go):
// plants and sensors in the zone, but no climate sensor chosen in Edit zone.
// Settings marks them for its warnings and the Today page lists them.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func TestClimateUnchecked_Zones(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	require.NoError(t, handlers.EnsureClimateTargets(e.db))
	sensor := func(zone int, kind, unit string) int {
		id := testutil.SeedSensor(t, e.db, "acinfinity", "D"+strconv.Itoa(zone), kind)
		e.exec(t, `UPDATE sensors SET zone_id = $1, unit = $2 WHERE id = $3`, zone, unit, id)
		return id
	}
	tent := testutil.SeedZone(t, e.db, "Tent")     // plants and sensors, none chosen: listed
	bare := testutil.SeedZone(t, e.db, "Bare")     // plants, only a CO2 sensor: nothing to choose
	empty := testutil.SeedZone(t, e.db, "Empty")   // sensors, no plants
	chosen := testutil.SeedZone(t, e.db, "Chosen") // a climate sensor chosen
	sensor(tent, "ACI.1.0", "°F")
	sensor(tent, "ACI.1.2", "%")
	sensor(bare, "ACI.2.11", "ppm")
	sensor(empty, "ACI.1.0", "°F")
	temp := sensor(chosen, "ACI.1.0", "°F")
	livingPlant(t, e.sensorEnv, "Tent plant", tent, "Veg")
	livingPlant(t, e.sensorEnv, "Bare plant", bare, "Veg")
	livingPlant(t, e.sensorEnv, "Chosen plant", chosen, "Flower")
	e.exec(t, `UPDATE zones SET vpd_temp_sensor_id = $1 WHERE id = $2`, temp, chosen)

	list := handlers.ClimateUnchecked(e.db, handlers.ListManagedSensors(e.db, 60), time.UTC)
	assert.Equal(t, []handlers.UncheckedZone{{ID: tent, Name: "Tent"}}, list)

	// The Today page lists it; Settings marks which zones have sensors to choose.
	type todayData struct {
		Attention struct {
			Climate []handlers.UncheckedZone `json:"climate"`
		} `json:"attention"`
	}
	var d todayData
	resp := e.client.Get("/home/data")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&d))
	testutil.DrainAndClose(resp)
	assert.Equal(t, list, d.Attention.Climate)
	body := e.page(t, "/settings")
	assert.Contains(t, body, `"choosable":true`)
	assert.Contains(t, body, `"choosable":false`)
	assert.Contains(t, body, `id="clNcBanner"`)
	assert.Contains(t, body, `id="dzNcNote"`)

	// A sensor chosen in Edit zone ends it.
	e.exec(t, `UPDATE zones SET vpd_humidity_sensor_id = (SELECT id FROM sensors WHERE zone_id = $1 AND type = 'ACI.1.2') WHERE id = $1`, tent)
	assert.Empty(t, handlers.ClimateUnchecked(e.db, handlers.ListManagedSensors(e.db, 60), time.UTC))
}

func TestClimateUnchecked_NoLimitsNoWarning(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	require.NoError(t, handlers.EnsureClimateTargets(e.db))
	tent := testutil.SeedZone(t, e.db, "Tent")
	id := testutil.SeedSensor(t, e.db, "acinfinity", "D1", "ACI.1.0")
	e.exec(t, `UPDATE sensors SET zone_id = $1, unit = '°F' WHERE id = $2`, tent, id)
	livingPlant(t, e.sensorEnv, "Plant", tent, "Veg")
	require.Len(t, handlers.ClimateUnchecked(e.db, handlers.ListManagedSensors(e.db, 60), time.UTC), 1)

	// With no Low or High limit anywhere, no zone is checked, so nothing to warn about.
	cs, err := handlers.LoadClimate(e.db)
	require.NoError(t, err)
	for i := range cs.Targets {
		cs.Targets[i].Range.Low, cs.Targets[i].Range.High = nil, nil
	}
	cs.TempLimits, cs.RHLimits = [2]*float64{}, [2]*float64{}
	require.NoError(t, handlers.SaveClimate(e.db, cs))
	assert.Empty(t, handlers.ClimateUnchecked(e.db, handlers.ListManagedSensors(e.db, 60), time.UTC))
}
