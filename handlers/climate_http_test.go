package handlers_test

// Climate targets and Edit zone (handlers/climate.go, UpdateZoneHandler):
// filling targets from the old VPD ranges, saving the card, the zone's
// light schedule and sensors, and coloring readings.

import (
	"encoding/json"
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

	// A changed range keeps its Low and High as limits, with the default
	// Target, lights on and off.
	for _, lights := range []string{"on", "off"} {
		r := targetOf(t, cs, climate.Veg, "", climate.VPD, lights)
		assert.InDelta(t, 0.9, *r.Low, 1e-9)
		assert.InDelta(t, 1.0, *r.Target, 1e-9)
		assert.InDelta(t, 1.3, *r.High, 1e-9)
	}
	// An untouched 019 range gets the defaults: a Target and no limits.
	r := targetOf(t, cs, climate.Seedling, "", climate.VPD, "off")
	assert.InDelta(t, 0.8, *r.Target, 1e-9)
	assert.Nil(t, r.Low)
	assert.Nil(t, r.High)
	// VPD's values for each Flower phase and for lights off.
	for _, c := range []struct {
		phase, lights string
		want          float64
	}{{"", "on", 1.3}, {"", "off", 1.2}, {climate.Stretch, "on", 1.2}, {climate.Stretch, "off", 1.2},
		{climate.Bloom, "on", 1.3}, {climate.Bloom, "off", 1.2}, {climate.Ripen, "on", 1.5}, {climate.Ripen, "off", 1.2}} {
		assert.InDelta(t, c.want, *targetOf(t, cs, climate.Flower, c.phase, climate.VPD, c.lights).Target, 1e-9, c.phase+c.lights)
	}
	// Temperature and humidity: limits and no Target, in every row. Temperatures are stored in °C.
	for _, phase := range []string{"", climate.Stretch, climate.Bloom, climate.Ripen} {
		for _, lights := range []string{"on", "off"} {
			r = targetOf(t, cs, climate.Flower, phase, climate.Temp, lights)
			assert.Nil(t, r.Target)
			assert.InDelta(t, 12.7778, *r.Low, 1e-4) // 55 °F
			assert.InDelta(t, 30.0, *r.High, 1e-4)   // 86 °F
			r = targetOf(t, cs, climate.Flower, phase, climate.RH, lights)
			assert.Nil(t, r.Target)
			assert.InDelta(t, 30, *r.Low, 1e-9)
			assert.InDelta(t, 70, *r.High, 1e-9)
		}
	}
	// Drying has all three for temperature and humidity, and no VPD.
	r = targetOf(t, cs, climate.Drying, "", climate.RH, "off")
	assert.InDelta(t, 60, *r.Target, 1e-9)
	assert.InDelta(t, 55, *r.Low, 1e-9)
	assert.InDelta(t, 65, *r.High, 1e-9)
	for _, x := range cs.Targets {
		assert.False(t, x.Stage == climate.Drying && x.Measure == climate.VPD, "VPD does not apply to Drying")
	}
	assert.Len(t, cs.Targets, 3*6*2+2)
	assert.Equal(t, [2]int{4, 8}, cs.PhaseWeeks)
	assert.Equal(t, [3]bool{true, false, false}, cs.SplitLights)
	assert.Equal(t, [3]bool{true, false, false}, cs.FlowerPhases)
	assert.True(t, cs.TempByStage)
	assert.True(t, cs.RHByStage)
	assert.InDelta(t, 12.7778, *cs.TempLimits[0], 1e-4)
	assert.InDelta(t, 32.2222, *cs.TempLimits[1], 1e-4)
	assert.InDelta(t, 30, *cs.RHLimits[0], 1e-9)
	assert.InDelta(t, 90, *cs.RHLimits[1], 1e-9)

	// It only fills an empty table.
	_, err = db.Exec(`UPDATE plant_status SET vpd_low = 0.1 WHERE status = 'Veg'`)
	require.NoError(t, err)
	require.NoError(t, handlers.EnsureClimateTargets(db))
	cs, err = handlers.LoadClimate(db)
	require.NoError(t, err)
	assert.InDelta(t, 0.9, *targetOf(t, cs, climate.Veg, "", climate.VPD, "on").Low, 1e-9)

	// A default Target outside the old limits can't be used: Target halfway.
	db2 := testutil.NewTestDB(t)
	_, err = db2.Exec(`DELETE FROM climate_target`)
	require.NoError(t, err)
	_, err = db2.Exec(`UPDATE plant_status SET vpd_low = 1.1, vpd_high = 1.5 WHERE status = 'Veg'`)
	require.NoError(t, err)
	require.NoError(t, handlers.EnsureClimateTargets(db2))
	cs, err = handlers.LoadClimate(db2)
	require.NoError(t, err)
	r = targetOf(t, cs, climate.Veg, "", climate.VPD, "on")
	assert.InDelta(t, 1.1, *r.Low, 1e-9)
	assert.InDelta(t, 1.3, *r.Target, 1e-9)
	assert.InDelta(t, 1.5, *r.High, 1e-9)
}

func TestClimate_SaveCard(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	require.NoError(t, handlers.EnsureClimateTargets(e.db))
	cs, err := handlers.LoadClimate(e.db)
	require.NoError(t, err)

	body := func(mod func(m map[string]any)) map[string]any {
		m := map[string]any{
			"split_lights": []bool{false, true, false}, "flower_phases": []bool{true, false, true}, "phase_weeks": []int{3, 7},
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
	assert.Equal(t, [3]bool{false, true, false}, got.SplitLights)
	assert.Equal(t, [3]bool{true, false, true}, got.FlowerPhases)
	assert.True(t, got.TempByStage)
	assert.Equal(t, [2]int{3, 7}, got.PhaseWeeks)
	assert.Equal(t, 12.5, *got.TempLimits[0])
	assert.Nil(t, got.TempLimits[1], "an empty limit stays empty")
	assert.Equal(t, 55.0, *targetOf(t, got, climate.Veg, "", climate.RH, "on").Target)
	// Rows not sent stay as they were.
	assert.Equal(t, *targetOf(t, cs, climate.Veg, "", climate.VPD, "on").Target, *targetOf(t, got, climate.Veg, "", climate.VPD, "on").Target)

	refused := map[string]func(m map[string]any){
		"low above high": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "veg", "phase": "", "measure": "vpd", "lights": "on", "low": 1.2, "target": 1.0, "high": 0.8}}
		},
		"target past a limit": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "flower", "phase": "", "measure": "rh", "lights": "on", "low": 30, "target": 75, "high": 70}}
		},
		"target equal to a limit": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "flower", "phase": "", "measure": "rh", "lights": "on", "low": 30, "target": 70, "high": 70}}
		},
		"low equal to high": func(m map[string]any) { m["rh_limits"] = []any{70, 70} },
		"vpd on drying": func(m map[string]any) {
			m["targets"] = []map[string]any{{"stage": "drying", "phase": "", "measure": "vpd", "lights": "off", "target": 1}}
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
	vpd := reading(tent, "ACI.1.3", "kPa", 1.1)    // Veg: Target 1.0 and no limits, near it
	temp := reading(tent, "ACI.1.0", "°F", 95)     // over Veg's 90 °F High limit
	hum := reading(tent, "ACI.1.2", "%", 60)       // inside the limits, no Target: no color
	other := reading(tent, "ACI.2.11", "ppm", 900) // not a climate sensor
	quiet := reading(empty, "ACI.1.3", "kPa", 3)   // no plants: no color
	livingPlant(t, e, "Veg plant", tent, "Veg")
	e.exec(t, `UPDATE zones SET vpd_temp_sensor_id = $1, vpd_humidity_sensor_id = $2, vpd_source = 'sensor', vpd_sensor_id = $3 WHERE id = $4`, temp, hum, vpd, tent)
	e.exec(t, `UPDATE zones SET vpd_source = 'sensor', vpd_sensor_id = $1 WHERE id = $2`, quiet, empty)

	checks := handlers.ClimateChecks(e.db, handlers.ListManagedSensors(e.db, 60), time.UTC)
	assert.Equal(t, climate.OK, checks[vpd].State)
	assert.InDelta(t, 1.0, *checks[vpd].Range.Target, 1e-9)
	assert.Equal(t, climate.Reached, checks[temp].State)
	assert.InDelta(t, 90, *checks[temp].Range.High, 1e-3, "limits in the sensor's own unit")
	assert.InDelta(t, 5, checks[temp].Near, 1e-9)
	assert.Equal(t, climate.None, checks[hum].State)
	assert.NotContains(t, checks, other)
	assert.Equal(t, climate.None, checks[quiet].State)

	// The Settings page carries the zone's stage and readings.
	body := e.page(t, "/settings")
	assert.Contains(t, body, `id="clData"`)
	assert.Contains(t, body, `"stage":"veg"`)

	// Zone status asks for fresh readings after a Grow setup save.
	e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 1.3, $2)`, vpd, time.Now().UTC().Add(time.Second).Format("2006-01-02 15:04:05"))
	resp := e.client.Get("/settings/climate/zones")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Zones []struct {
			ID       int `json:"id"`
			Readings map[string]struct {
				Value *float64 `json:"value"`
			} `json:"readings"`
		} `json:"zones"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	found := false
	for _, z := range out.Zones {
		if z.ID == tent {
			found = true
			require.NotNil(t, z.Readings["vpd"].Value)
			assert.InDelta(t, 1.3, *z.Readings["vpd"].Value, 1e-9)
		}
	}
	assert.True(t, found, "the tent is listed")
}
