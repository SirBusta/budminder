package handlers_test

// Recent alerts' readings (2.3): the engine saves the reading when an alert
// started, the highest or lowest reading and each limit in use; the page shows
// them; the Sensors page reads an alert through /settings/alerts/events/:id/chart;
// and Clear All Ended can clear only the alerts a filter shows.

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
)

func fToC(f float64) float64 { return (f - 32) * 5 / 9 }

func TestAlertEngine_SavesStartingReadingAndHighest(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	// 16 minutes at or above the 90 °F limit, highest 97 °F, ending at 93 °F.
	vals := repeated(92, 16)
	vals[10], vals[15] = 97, 93
	a.readings(t, a.temp, vals...)
	a.check()

	ev := a.events(t)
	require.Len(t, ev, 1)
	require.NotNil(t, ev[0].StartValue)
	require.NotNil(t, ev[0].Peak)
	assert.InDelta(t, fToC(93), *ev[0].StartValue, 0.01, "the reading when it started, in °C")
	assert.InDelta(t, fToC(97), *ev[0].Peak, 0.01, "the 15 minutes before it started count")
	require.Len(t, ev[0].Limits, 1)
	assert.InDelta(t, fToC(90), ev[0].Limits[0].V, 0.01)
	assert.Equal(t, "veg", ev[0].Limits[0].Stage)
	assert.Empty(t, ev[0].Limits[0].Lights, "no light schedule")

	// Higher readings while it's active raise the highest; the starting reading stays.
	a.fresh(t, a.temp)
	vals = repeated(94, 10)
	vals[9] = 99
	a.readings(t, a.temp, vals...)
	a.check()
	ev = a.events(t)
	require.Len(t, ev, 1)
	assert.InDelta(t, fToC(99), *ev[0].Peak, 0.01)
	assert.InDelta(t, fToC(93), *ev[0].StartValue, 0.01)

	// A lower reading never lowers it.
	a.fresh(t, a.temp)
	a.readings(t, a.temp, repeated(91, 10)...)
	a.check()
	assert.InDelta(t, fToC(99), *a.events(t)[0].Peak, 0.01)
}

func TestAlertEngine_ListsEachLimitUsed(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.held(t, a.temp, 95)
	a.check()
	// The High limit is raised to 33 °C (91.4 °F) while readings stay above it.
	a.exec(t, `UPDATE climate_target SET high = 33 WHERE measure = 'temp'`)
	a.check()
	a.check() // unchanged: not listed twice

	ev := a.events(t)
	require.Len(t, ev, 1)
	require.Len(t, ev[0].Limits, 2)
	assert.InDelta(t, fToC(90), ev[0].Limits[0].V, 0.01)
	assert.InDelta(t, 33, ev[0].Limits[1].V, 0.01)

	w := alerts.Words{T: func(k string) string {
		return map[string]string{"al_row_limit_high": "High limit: {limits}.", "al_row_changed_to": "{from}, changed to {to}"}[k]
	}, Units: "imperial"}
	assert.Equal(t, "High limit: 90 °F, changed to 91 °F.", w.RowLine(ev[0], true))
}

func TestAlertEngine_SensorAlertReadings(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	co2 := a.co2Sensor(t)
	a.exec(t, `INSERT INTO alert_rule (sensor_id, side, limit_value, enabled) VALUES ($1, 'high', 1500, 1)`, co2)
	vals := repeated(1510, 16)
	vals[3] = 1620
	a.readings(t, co2, vals...)
	a.check()

	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.KindSensor, ev[0].Kind)
	assert.InDelta(t, 1510, *ev[0].StartValue, 0.01)
	assert.InDelta(t, 1620, *ev[0].Peak, 0.01)
	require.Len(t, ev[0].Limits, 1)
	assert.InDelta(t, 1500, ev[0].Limits[0].V, 0.01)
}

func TestAlertsPage_RowsShowReadings(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	db := r.server.DB
	limits := alerts.EncodeLimits([]alerts.LimitUsed{
		{V: fToC(78), Lights: "on", Stage: "flower"}, {V: fToC(74), Lights: "off", Stage: "flower"}})
	_, err := db.Exec(`INSERT INTO alert_event (kind, zone_id, zone_name, measure, side, value, limit_value, started_at, notified_at, resolved_at, start_value, peak_value, limits)
		VALUES ('stage', 1, 'Tent', 'temp', 'high', $1, $2, '2026-11-04 12:00:00', '2026-11-04 12:00:00', '2026-11-06 12:00:00', $3, $4, $5)`,
		fToC(76), fToC(74), fToC(79.1), fToC(81.2), limits)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO alert_event (kind, zone_id, zone_name, sensor_name, measure, side, limit_value, detail, started_at, notified_at, start_value, peak_value, limits)
		VALUES ('sensor', 0, 'Veg tent', 'CO2', 'rule:1', 'high', 1500, 'ppm', '2026-11-06 11:00:00', '2026-11-06 11:00:00', 1519, 1611, '[{"v":1500}]')`)
	require.NoError(t, err)

	page := r.settingsPage(t)
	for _, want := range []string{
		`"value":"79 °F"`,
		"Highest: 81 °F. High limit: 78 °F with lights on, 74 °F with lights off.",
		"CO2 high in Veg tent",
		`"value":"1519 ppm"`,
		"Highest so far: 1611 ppm. Your limit: 1500 ppm.",
		`"type":"rule"`, `"chart":true`,
		`id="alZone"`, `id="alType"`,
	} {
		assert.Contains(t, page, want)
	}
	assert.Regexp(t, `Nov 4, \d+:\d\d [AP]M – Nov 6, \d+:\d\d [AP]M`, page, "an end on another day shows its date")
}

func TestAlertsPage_ClearEndedShown(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	db := r.server.DB
	ins := `INSERT INTO alert_event (kind, zone_id, measure, side, started_at, notified_at, resolved_at) VALUES ('stage', 1, $1, 'low', '2026-10-30 12:00:00', '2026-10-30 12:00:00', $2) RETURNING id`
	var temp, rh, active int
	require.NoError(t, db.QueryRow(ins, "temp", "2026-10-30 13:00:00").Scan(&temp))
	require.NoError(t, db.QueryRow(ins, "rh", "2026-10-30 13:00:00").Scan(&rh))
	require.NoError(t, db.QueryRow(ins, "vpd", nil).Scan(&active))

	code, _ := r.post(t, "/settings/alerts/clear-ended", map[string]any{"ids": []int{temp, active}})
	require.Equal(t, http.StatusOK, code)
	list, err := handlers.RecentAlerts(db, true, 10)
	require.NoError(t, err)
	require.Len(t, list, 1, "only the ended alert sent, never an active one")
	assert.Equal(t, temp, list[0].ID)

	code, _ = r.post(t, "/settings/alerts/clear-ended", map[string]any{"ids": repeatedInts(51)})
	assert.Equal(t, http.StatusBadRequest, code, "more than the list shows")
}

func repeatedInts(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func (r *alertRoutes) chart(t *testing.T, id int) (int, map[string]any) {
	t.Helper()
	resp := r.c.Get("/settings/alerts/events/" + strconv.Itoa(id) + "/chart")
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func TestAlertChart(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	db := r.server.DB
	sensor := testutil.SeedSensor(t, db, "acinfinity", "D1", "ACI.1.0")
	limits := `[{"v":30,"at":"2026-11-05T03:00:00Z"},{"v":32,"at":"2026-11-05T03:30:00Z"}]`
	var first, second, clock, other int
	ins := `INSERT INTO alert_event (kind, zone_id, zone_name, sensor_id, measure, side, started_at, notified_at, resolved_at, limits)
		VALUES ($1, 7, 'Veg tent', $2, $3, 'high', $4, $4, $5, $6) RETURNING id`
	require.NoError(t, db.QueryRow(ins, "stage", sensor, "temp", "2026-11-05 03:09:00", "2026-11-05 04:57:00", limits).Scan(&first))
	require.NoError(t, db.QueryRow(ins, "stage", sensor, "temp", "2026-11-05 05:16:00", "2026-11-05 06:10:00", "").Scan(&second))
	require.NoError(t, db.QueryRow(ins, "stage", sensor, "rh", "2026-11-05 05:00:00", nil, "").Scan(&other))
	require.NoError(t, db.QueryRow(ins, "clock", sensor, "", "2026-11-05 05:00:00", nil, "").Scan(&clock))

	code, ch := r.chart(t, first)
	require.Equal(t, http.StatusOK, code)
	ms := func(s string) float64 {
		tm, err := time.Parse(time.RFC3339, s)
		require.NoError(t, err)
		return float64(tm.UnixMilli())
	}
	assert.EqualValues(t, 7, ch["zone_id"])
	assert.Equal(t, "limit", ch["kind"])
	assert.Equal(t, "c", ch["limit_in"], "temperature limits are in °C")
	assert.Equal(t, []any{float64(sensor)}, ch["sensors"])
	assert.Equal(t, ms("2026-11-05T03:09:00Z"), ch["start"])
	assert.Equal(t, ms("2026-11-05T04:57:00Z"), ch["end"])
	assert.Equal(t, ms("2026-11-05T01:54:00Z"), ch["from"], "an hour before the 15 minutes that started it")
	assert.Equal(t, ms("2026-11-05T05:57:00Z"), ch["to"], "an hour after it ended")
	assert.Len(t, ch["limits"], 2)
	others := ch["others"].([]any)
	require.Len(t, others, 1, "the next temperature alert, not the humidity one")
	assert.Equal(t, ms("2026-11-05T05:16:00Z"), others[0].(map[string]any)["start"])

	code, _ = r.chart(t, clock)
	assert.Equal(t, http.StatusNotFound, code, "clock notices have no chart")
	code, _ = r.chart(t, 99999)
	assert.Equal(t, http.StatusNotFound, code)
}
