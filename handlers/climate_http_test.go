package handlers_test

// Climate targets and Edit zone (handlers/climate.go, UpdateZoneHandler):
// filling targets from the old VPD ranges, saving the card, the zone's
// light schedule and sensors, and coloring readings.

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/climate"
	"isley/handlers"
	"isley/tests/testutil"
)

func targetOf(t *testing.T, cs handlers.ClimateSettings, stage, phase, measure, lights string) climate.Range {
	t.Helper()
	for _, x := range cs.Targets {
		if x.Stage == stage && x.Phase == phase && x.Measure == measure && x.Lights == lights {
			return x.Range
		}
	}
	t.Fatalf("no target %s/%s/%s/%s", stage, phase, measure, lights)
	return climate.Range{}
}

func TestClimate_FilledFromOldVPDRanges(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	_, err := db.Exec(`DELETE FROM climate_target`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE plant_status SET vpd_low = 0.9, vpd_high = 1.3 WHERE status = 'Veg'`)
	require.NoError(t, err)

	require.NoError(t, handlers.EnsureClimateTargets(db))
	cs, err := handlers.LoadClimate(db)
	require.NoError(t, err)

	// A changed range keeps its Low and High, Target halfway, lights on and off.
	for _, lights := range []string{"on", "off"} {
		r := targetOf(t, cs, climate.Veg, "", climate.VPD, lights)
		assert.InDelta(t, 0.9, *r.Low, 1e-9)
		assert.InDelta(t, 1.1, *r.Target, 1e-9)
		assert.InDelta(t, 1.3, *r.High, 1e-9)
	}
	// An untouched 019 range gets the design's defaults.
	r := targetOf(t, cs, climate.Seedling, "", climate.VPD, "off")
	assert.InDelta(t, 0.5, *r.Target, 1e-9)
	// Temperatures are stored in °C.
	r = targetOf(t, cs, climate.Flower, climate.Bloom, climate.Temp, "on")
	assert.InDelta(t, 23.8889, *r.Target, 1e-4) // 75 °F
	assert.Len(t, cs.Targets, 3*(6*2+1))
	assert.Equal(t, [2]int{4, 8}, cs.PhaseWeeks)
	assert.True(t, cs.SplitLights)
	assert.False(t, cs.FlowerPhases)

	// It only fills an empty table.
	_, err = db.Exec(`UPDATE plant_status SET vpd_low = 0.1 WHERE status = 'Veg'`)
	require.NoError(t, err)
	require.NoError(t, handlers.EnsureClimateTargets(db))
	cs, err = handlers.LoadClimate(db)
	require.NoError(t, err)
	assert.InDelta(t, 0.9, *targetOf(t, cs, climate.Veg, "", climate.VPD, "on").Low, 1e-9)
}

func TestClimate_SaveCard(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	require.NoError(t, handlers.EnsureClimateTargets(e.db))
	cs, err := handlers.LoadClimate(e.db)
	require.NoError(t, err)

	body := func(mod func(m map[string]any)) map[string]any {
		m := map[string]any{
			"split_lights": false, "flower_phases": true, "phase_weeks": []int{3, 7},
			"temp_by_stage": true, "rh_by_stage": false,
			"temp_limits": []any{12.5, nil}, "rh_limits": []any{25, 80},
			"targets": []map[string]any{{"stage": "veg", "phase": "", "measure": "rh", "lights": "on", "low": 50, "target": 55, "high": 65}},
		}
		if mod != nil {
			mod(m)
		}
		return m
	}
	post := func(m map[string]any) int {
		resp := e.client.SessionPostJSON(t, "/settings/climate", e.token, m)
		testutil.DrainAndClose(resp)
		return resp.StatusCode
	}
	require.Equal(t, http.StatusOK, post(body(nil)))

	got, err := handlers.LoadClimate(e.db)
	require.NoError(t, err)
	assert.False(t, got.SplitLights)
	assert.True(t, got.FlowerPhases)
	assert.True(t, got.TempByStage)
	assert.Equal(t, [2]int{3, 7}, got.PhaseWeeks)
	assert.Equal(t, 12.5, *got.TempLimits[0])
	assert.Nil(t, got.TempLimits[1], "an empty safety limit is skipped")
	assert.Equal(t, 55.0, *targetOf(t, got, climate.Veg, "", climate.RH, "on").Target)
	// Rows not sent stay as they were.
	assert.Equal(t, *targetOf(t, cs, climate.Veg, "", climate.VPD, "on").Target, *targetOf(t, got, climate.Veg, "", climate.VPD, "on").Target)

	refused := map[string]func(m map[string]any){
		"low above high": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "veg", "phase": "", "measure": "vpd", "lights": "on", "low": 1.2, "target": 1.0, "high": 0.8}}
		},
		"weeks backwards": func(m map[string]any) { m["phase_weeks"] = []int{6, 3} },
		"unknown stage": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "harvest", "phase": "", "measure": "vpd", "lights": "on", "low": 1, "target": 1, "high": 1}}
		},
		"drying has no lights-on row": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "drying", "phase": "", "measure": "vpd", "lights": "on", "low": 1, "target": 1, "high": 1}}
		},
		"humidity over 100": func(m map[string]any) { m["rh_limits"] = []any{25, 120} },
		"limits backwards":  func(m map[string]any) { m["temp_limits"] = []any{30, 20} },
		"vpd out of range": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "veg", "phase": "", "measure": "vpd", "lights": "off", "low": 1, "target": 2, "high": 9}}
		},
		"phase on a whole stage": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "veg", "phase": "bloom", "measure": "vpd", "lights": "on", "low": 1, "target": 1, "high": 1}}
		},
	}
	for name, mod := range refused {
		assert.Equal(t, http.StatusBadRequest, post(body(mod)), name)
	}
}

func TestZone_ClimateFields(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	tent := testutil.SeedZone(t, e.db, "Tent")
	other := testutil.SeedZone(t, e.db, "Other")
	sensor := func(zone int, kind string) int {
		id := testutil.SeedSensor(t, e.db, "acinfinity", "D"+strconv.Itoa(zone), kind)
		e.exec(t, `UPDATE sensors SET zone_id = $1 WHERE id = $2`, zone, id)
		return id
	}
	temp, hum, vpd, light := sensor(tent, "ACI.1.0"), sensor(tent, "ACI.1.2"), sensor(tent, "ACI.1.3"), sensor(tent, "ACIP.1")
	elsewhere := sensor(other, "ACI.1.0")
	plant := livingPlant(t, e, "Tent plant", tent, "Flower")
	away := livingPlant(t, e, "Away plant", other, "Veg")

	put := func(body map[string]any) int {
		resp := e.client.SessionPutJSON(t, "/zones/"+strconv.Itoa(tent), e.token, body)
		testutil.DrainAndClose(resp)
		return resp.StatusCode
	}
	ok := func() map[string]any {
		return map[string]any{
			"zone_name": "Tent", "light_schedule": map[string]any{"veg": []string{"06:00", "00:00"}, "flower": []string{"06:00", "18:00"}},
			"light_sensor_id": light, "light_on_above": 0, "stage_plant_id": plant,
			"vpd_temp_sensor_id": temp, "vpd_humidity_sensor_id": hum, "vpd_source": "sensor", "vpd_sensor_id": vpd,
			"leaf_temp_offset": -2,
		}
	}
	require.Equal(t, http.StatusOK, put(ok()))
	setups, err := handlers.LoadZoneSetups(e.db)
	require.NoError(t, err)
	z := setups[tent]
	assert.Equal(t, climate.Schedule{"veg": {"06:00", "00:00"}, "flower": {"06:00", "18:00"}}, z.Schedule)
	assert.Equal(t, light, *z.LightSensorID)
	assert.Equal(t, plant, *z.StagePlantID)
	assert.Equal(t, "sensor", z.VPDSource)
	assert.Equal(t, vpd, *z.VPDSensorID)
	assert.Nil(t, z.LeafTempOffset, "a sensor's VPD leaves the calculated one off")

	// Calculated VPD keeps the offset and makes the derived sensor.
	b := ok()
	b["vpd_source"], b["vpd_sensor_id"] = "calc", nil
	require.Equal(t, http.StatusOK, put(b))
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM sensors WHERE source = 'derived' AND type = 'VPD' AND zone_id = $1`, tent).Scan(&n))
	assert.Equal(t, 1, n)
	setups, _ = handlers.LoadZoneSetups(e.db)
	assert.Equal(t, -2.0, *setups[tent].LeafTempOffset)

	// No schedule: lights-on targets all the time, so no light check.
	b = ok()
	b["light_schedule"] = nil
	require.Equal(t, http.StatusOK, put(b))
	setups, _ = handlers.LoadZoneSetups(e.db)
	assert.Nil(t, setups[tent].Schedule)
	assert.Nil(t, setups[tent].LightSensorID)

	refused := map[string]func(m map[string]any){
		"sensor from another zone": func(m map[string]any) { m["vpd_temp_sensor_id"] = elsewhere },
		"plant from another zone":  func(m map[string]any) { m["stage_plant_id"] = away },
		"bad time":                 func(m map[string]any) { m["light_schedule"] = map[string]any{"veg": []string{"25:00", "06:00"}} },
		"drying schedule":          func(m map[string]any) { m["light_schedule"] = map[string]any{"drying": []string{"06:00", "18:00"}} },
		"unknown vpd source":       func(m map[string]any) { m["vpd_source"] = "guess" },
	}
	for name, mod := range refused {
		m := ok()
		mod(m)
		assert.Equal(t, http.StatusBadRequest, put(m), name)
	}
	resp := e.client.SessionPutJSON(t, "/zones/99999", e.token, map[string]any{"zone_name": "Ghost"})
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestClimate_ReadingColors(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	require.NoError(t, handlers.EnsureClimateTargets(e.db))
	tent := testutil.SeedZone(t, e.db, "Tent")
	empty := testutil.SeedZone(t, e.db, "Empty")
	reading := func(zone int, kind, unit string, v float64) int {
		id := testutil.SeedSensor(t, e.db, "acinfinity", "D"+strconv.Itoa(zone), kind)
		e.exec(t, `UPDATE sensors SET zone_id = $1, unit = $2 WHERE id = $3`, zone, unit, id)
		e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`, id, v, time.Now().UTC().Format("2006-01-02 15:04:05"))
		return id
	}
	vpd := reading(tent, "ACI.1.3", "kPa", 0.8)    // Veg lights on: 0.8 · 1.0 · 1.2, reaching Low
	temp := reading(tent, "ACI.1.0", "°F", 95)     // over a 90 °F safety limit
	hum := reading(tent, "ACI.1.2", "%", 60)       // inside the limits: no color
	other := reading(tent, "ACI.2.11", "ppm", 900) // not a climate sensor
	quiet := reading(empty, "ACI.1.3", "kPa", 3)   // no plants: no color
	livingPlant(t, e, "Veg plant", tent, "Veg")
	e.exec(t, `UPDATE zones SET vpd_temp_sensor_id = $1, vpd_humidity_sensor_id = $2, vpd_source = 'sensor', vpd_sensor_id = $3 WHERE id = $4`, temp, hum, vpd, tent)
	e.exec(t, `UPDATE zones SET vpd_source = 'sensor', vpd_sensor_id = $1 WHERE id = $2`, quiet, empty)
	testutil.UpsertSetting(t, e.db, "climate_temp_limits", "12.7778,32.2222")
	testutil.UpsertSetting(t, e.db, "climate_rh_limits", "25,80")

	checks := handlers.ClimateChecks(e.db, handlers.ListManagedSensors(e.db, 60), time.UTC)
	assert.Equal(t, climate.Reached, checks[vpd].State)
	assert.InDelta(t, 1.0, *checks[vpd].Range.Target, 1e-9)
	assert.Equal(t, climate.Reached, checks[temp].State)
	assert.InDelta(t, 90, *checks[temp].Limits[1], 1e-3, "limits in the sensor's own unit")
	assert.Equal(t, climate.None, checks[hum].State)
	assert.NotContains(t, checks, other)
	assert.Equal(t, climate.None, checks[quiet].State)

	// The Settings page carries the zone's stage and readings.
	body := e.page(t, "/settings")
	assert.Contains(t, body, `id="clData"`)
	assert.Contains(t, body, `"stage":"veg"`)
}
