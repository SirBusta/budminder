package handlers_test

// Home Assistant from the dashboard (handlers/homeassistant.go and the
// Home Assistant part of POST /settings): saving the connection, Test
// connection, and the Manage sensors scan, through a real login + CSRF.

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

// saveHASettings posts a settings save like the Settings page does; ha nil
// leaves the Home Assistant part out of the body.
func (e *sensorEnv) saveHASettings(t *testing.T, ha map[string]any) {
	t.Helper()
	body := map[string]any{
		"polling_interval": "60", "stream_grab_interval": "30000", "sensor_retention_days": "90",
		"log_level": "info", "units": "imperial",
	}
	if ha != nil {
		body["homeassistant"] = ha
	}
	resp := e.client.SessionPostJSON(t, "/settings", e.token, body)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func (e *sensorEnv) setting(t *testing.T, name string) string {
	t.Helper()
	var v string
	_ = e.db.QueryRow(`SELECT value FROM settings WHERE name = $1`, name).Scan(&v)
	return v
}

func TestHomeAssistant_SettingsKeepTheTokenPrivate(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	const secret = "ha-secret-token-123"

	e.saveHASettings(t, map[string]any{"enabled": true, "url": " http://192.168.1.20:8123 ", "token": secret})
	assert.Equal(t, "1", e.setting(t, "homeassistant.enabled"))
	assert.Equal(t, "http://192.168.1.20:8123", e.setting(t, "homeassistant.url"))
	assert.Equal(t, secret, e.setting(t, "homeassistant.token"))

	resp := e.client.Get("/settings")
	page, err := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.NoError(t, err)
	assert.NotContains(t, string(page), secret, "the token is never sent back to the page")
	assert.Contains(t, string(page), "http://192.168.1.20:8123")

	// A blank token keeps the saved one; a save without Home Assistant
	// settings leaves them all alone.
	e.saveHASettings(t, map[string]any{"enabled": true, "url": "http://192.168.1.20:8123", "token": ""})
	assert.Equal(t, secret, e.setting(t, "homeassistant.token"))
	e.saveHASettings(t, nil)
	assert.Equal(t, "1", e.setting(t, "homeassistant.enabled"))
	assert.Equal(t, secret, e.setting(t, "homeassistant.token"))

	e.saveHASettings(t, map[string]any{"enabled": false, "url": "http://192.168.1.20:8123"})
	assert.Equal(t, "0", e.setting(t, "homeassistant.enabled"))
}

func TestHomeAssistant_TestConnection(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	ha := fakes.FakeHomeAssistant(t, "states")

	test := func(body map[string]any) (int, map[string]any) {
		resp := e.client.SessionPostJSON(t, "/settings/homeassistant/test", e.token, body)
		defer testutil.DrainAndClose(resp)
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	code, out := test(map[string]any{"url": ha.URL, "token": fakes.HomeAssistantToken})
	require.Equal(t, http.StatusOK, code, out)
	assert.EqualValues(t, 9, out["sensors"], "numeric sensors")
	assert.EqualValues(t, 7, out["grow"], "of which grow-related")

	code, out = test(map[string]any{"url": ha.URL, "token": "wrong"})
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Contains(t, out["error"], "token")

	code, _ = test(map[string]any{"url": "192.168.1.20:8123", "token": "x"})
	assert.Equal(t, http.StatusBadRequest, code, "an address without http:// is explained, not tried")

	code, _ = test(map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code, "nothing saved or typed yet")

	// With the connection saved, an empty body tests the saved settings.
	e.saveHASettings(t, map[string]any{"enabled": true, "url": ha.URL, "token": fakes.HomeAssistantToken})
	code, _ = test(map[string]any{})
	assert.Equal(t, http.StatusOK, code)
}

func TestHomeAssistant_ScanListsNewSensorsAndAddWorks(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	ha := fakes.FakeHomeAssistant(t, "states")
	e.saveHASettings(t, map[string]any{"enabled": true, "url": ha.URL, "token": fakes.HomeAssistantToken})
	testutil.SeedSensor(t, e.db, "homeassistant", "homeassistant", "sensor.tent_humidity")
	countSensors := func() int {
		var n int
		require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM sensors`).Scan(&n))
		return n
	}
	before := countSensors()

	resp := e.client.SessionPostJSON(t, "/sensors/scanHA", e.token, map[string]any{"preview": true})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Sensors []struct {
			Source, Device, Type, Name, Unit string
			Value                            *float64
			More                             bool
		} `json:"sensors"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))

	byType := map[string]int{}
	for i, s := range out.Sensors {
		byType[s.Type] = i
		assert.Equal(t, "homeassistant", s.Source)
		assert.Equal(t, "homeassistant", s.Device)
	}
	assert.Len(t, out.Sensors, 8, "9 numeric sensors minus the one already added")
	assert.NotContains(t, byType, "sensor.tent_humidity", "already added")
	assert.NotContains(t, byType, "sensor.garage_temperature", "unavailable right now")
	temp := out.Sensors[byType["sensor.tent_temperature"]]
	assert.Equal(t, "Tent temperature", temp.Name)
	assert.Equal(t, "°F", temp.Unit)
	require.NotNil(t, temp.Value)
	assert.InDelta(t, 77.4, *temp.Value, 0.001)
	assert.False(t, temp.More)
	assert.True(t, out.Sensors[byType["sensor.tent_probe_battery"]].More, "batteries only with Show all")
	assert.Greater(t, byType["sensor.dehumidifier_power"], byType["sensor.tent_vpd"], "grow sensors are listed first")
	assert.Equal(t, before, countSensors(), "a scan saves nothing")

	add := e.client.SessionPostJSON(t, "/sensors/add", e.token, map[string]any{
		"source": "homeassistant", "device": "homeassistant", "type": "sensor.tent_co2",
		"name": "Tent CO2", "unit": "ppm", "new_zone": "HA tent",
	})
	defer testutil.DrainAndClose(add)
	require.Equal(t, http.StatusOK, add.StatusCode)
	assert.Equal(t, before+1, countSensors())
}

func TestHomeAssistant_ScanExplainsMissingConnection(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	resp := e.client.SessionPostJSON(t, "/sensors/scanHA", e.token, map[string]any{"preview": true})
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHomeAssistant_NeedsLoginAndCSRF(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	for _, path := range []string{"/settings/homeassistant/test", "/sensors/scanHA"} {
		resp := e.client.SessionPostJSON(t, path, "wrong-token", map[string]any{})
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, path)

		anon := e.server.NewClient(t)
		req, err := http.NewRequest(http.MethodPost, anon.BaseURL+path, nil)
		require.NoError(t, err)
		r, err := anon.Do(req)
		require.NoError(t, err)
		testutil.DrainAndClose(r)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, r.StatusCode, path)
	}
}
