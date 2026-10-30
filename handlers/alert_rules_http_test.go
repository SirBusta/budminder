package handlers_test

// Sensor alerts: the Settings routes, and the background check against a real
// database and fake mail server.

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
	"isley/handlers"
	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

// co2Sensor adds a sensor no climate check looks at.
func (a *alertEnv) co2Sensor(t *testing.T) int {
	t.Helper()
	id := testutil.SeedSensor(t, a.db, "acinfinity", "D1", "CO2")
	a.exec(t, `UPDATE sensors SET name = 'CO2', zone_id = $1, unit = 'ppm' WHERE id = $2`, a.tent, id)
	return id
}

func (a *alertEnv) addRule(t *testing.T, body map[string]any) (int, map[string]any) {
	t.Helper()
	resp := a.client.SessionPostJSON(t, "/settings/alerts/rules", a.token, body)
	defer testutil.DrainAndClose(resp)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func ruleBody(sensor int) map[string]any {
	return map[string]any{"sensor_id": sensor, "side": "high", "limit": 1000, "hold_minutes": 5, "enabled": true}
}

func (a *alertEnv) rule(t *testing.T, sensor int) int {
	t.Helper()
	code, out := a.addRule(t, ruleBody(sensor))
	require.Equal(t, http.StatusOK, code)
	return int(out["id"].(float64))
}

func TestAlertRules_Validation(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	co2 := a.co2Sensor(t)
	for what, mod := range map[string]func(m map[string]any){
		"unknown sensor": func(m map[string]any) { m["sensor_id"] = 99999 },
		"bad side":       func(m map[string]any) { m["side"] = "sideways" },
		"no limit":       func(m map[string]any) { delete(m, "limit") },
		"huge limit":     func(m map[string]any) { m["limit"] = 1e9 },
		"hold too short": func(m map[string]any) { m["hold_minutes"] = 0 },
		"hold too long":  func(m map[string]any) { m["hold_minutes"] = 1441 },
		"no enabled":     func(m map[string]any) { delete(m, "enabled") },
	} {
		body := ruleBody(co2)
		mod(body)
		code, _ := a.addRule(t, body)
		assert.Equal(t, http.StatusBadRequest, code, what)
	}
	var n int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM alert_rule`).Scan(&n))
	assert.Zero(t, n)

	// The page lists a saved rule.
	id := a.rule(t, co2)
	assert.Contains(t, a.page(t, "/settings"), `"id":`+strconv.Itoa(id)+`,"sensor_id":`+strconv.Itoa(co2))
}

func TestAlertRules_TooMany(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	co2 := a.co2Sensor(t)
	for range alerts.MaxRules {
		a.exec(t, `INSERT INTO alert_rule (sensor_id, side, limit_value) VALUES ($1, 'high', 1)`, co2)
	}
	code, _ := a.addRule(t, ruleBody(co2))
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestAlertRules_EmailCycle(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.mode(t, alerts.ModeEmail)
	co2 := a.co2Sensor(t)
	a.rule(t, co2)

	a.readings(t, co2, 900, 900, 900)
	a.check()
	assert.Empty(t, a.events(t), "under the limit")

	a.fresh(t, co2)
	a.readings(t, co2, 1500, 1500, 1500, 1500, 1500, 1500)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)
	assert.Contains(t, f.Mail()[0].Data, "CO2 (Tent) at or above your limit")
	assert.Contains(t, f.Mail()[0].Data, "1500 ppm")
	assert.Contains(t, f.Mail()[0].Data, "1000 ppm")
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.KindSensor, ev[0].Kind)
	assert.Equal(t, co2, *ev[0].SensorID)

	// No second start, on the next poll or after a restart.
	a.check()
	handlers.NewAlertEngine(a.db, a.engine.Store, a.dataDir).Check(t.Context())
	a.send()
	assert.Len(t, f.Mail(), 1)

	// Still high after the repeat time.
	a.exec(t, `UPDATE alert_event SET notified_at = $1`, utcDB(time.Now().Add(-3*time.Hour)))
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)
	assert.Contains(t, f.Mail()[1].Data, "Still active.")

	// Clear for the whole hold: one back-to-normal email.
	a.fresh(t, co2)
	a.readings(t, co2, 800, 800, 800, 800, 800, 800)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 3)
	assert.Contains(t, f.Mail()[2].Data, "CO2 (Tent) back to normal")
	assert.NotNil(t, a.events(t)[0].Resolved)
}

func TestAlertRules_ClosesQuietly(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(a *alertEnv, t *testing.T, rule, sensor int){
		"switched off": func(a *alertEnv, t *testing.T, rule, sensor int) {
			body := ruleBody(sensor)
			body["enabled"] = false
			resp := a.client.SessionPutJSON(t, "/settings/alerts/rules/"+strconv.Itoa(rule), a.token, body)
			defer testutil.DrainAndClose(resp)
			require.Equal(t, http.StatusOK, resp.StatusCode)
		},
		"deleted": func(a *alertEnv, t *testing.T, rule, _ int) {
			resp := a.client.SessionDeleteJSON(t, "/settings/alerts/rules/"+strconv.Itoa(rule), a.token, nil)
			defer testutil.DrainAndClose(resp)
			require.Equal(t, http.StatusOK, resp.StatusCode)
		},
		"sensor deleted": func(a *alertEnv, t *testing.T, _, sensor int) {
			require.NoError(t, handlers.DeleteSensorByID(a.db, strconv.Itoa(sensor)))
		},
		"mode off": func(a *alertEnv, t *testing.T, _, _ int) { a.mode(t, alerts.ModeOff) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newAlertEnv(t)
			f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
			a.mail(t, f)
			a.mode(t, alerts.ModeEmail)
			co2 := a.co2Sensor(t)
			rule := a.rule(t, co2)
			a.readings(t, co2, 1500, 1500, 1500, 1500, 1500, 1500)
			a.check()
			a.send()
			require.Len(t, f.Mail(), 1)
			change(a, t, rule, co2)
			a.check()
			a.send()
			assert.Len(t, f.Mail(), 1, "closing sends nothing")
			ev := a.events(t)
			require.Len(t, ev, 1)
			assert.NotNil(t, ev[0].Resolved)
		})
	}
}

func TestAlertRules_EditStartsOver(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	co2 := a.co2Sensor(t)
	rule := a.rule(t, co2)
	a.readings(t, co2, 1500, 1500, 1500, 1500, 1500, 1500)
	a.check()
	require.Len(t, a.events(t), 1)

	body := ruleBody(co2)
	body["limit"] = 1200
	resp := a.client.SessionPutJSON(t, "/settings/alerts/rules/"+strconv.Itoa(rule), a.token, body)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 2)
	assert.Nil(t, ev[0].Resolved)
	assert.InDelta(t, 1200, *ev[0].Limit, 0.001, "the new limit")
	assert.NotNil(t, ev[1].Resolved)
}

func TestAlertRules_SensorDeleteRemovesRule(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	co2 := a.co2Sensor(t)
	a.rule(t, co2)
	require.NoError(t, handlers.DeleteSensorByID(a.db, strconv.Itoa(co2)))
	var n int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM alert_rule`).Scan(&n))
	assert.Zero(t, n)
}
