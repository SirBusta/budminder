//go:build integration_postgres

// Environment alerts on real PostgreSQL: its parameter typing is stricter than
// SQLite's (the reused $3 in the End update, CASE results), and the partial
// unique index and ON CONFLICT DO NOTHING need the real thing.
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./handlers/...

package handlers_test

import (
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

func TestAlertEngine_EmailLifecycle_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)

	a.held(t, a.temp, 95)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.DeliverySent, ev[0].Delivery)
	assert.True(t, ev[0].Emailed)
	assert.InDelta(t, 32.22, *ev[0].Limit, 0.01)
	assert.WithinDuration(t, time.Now(), ev[0].Started, time.Minute, "started_at reads back in UTC")

	a.exec(t, `UPDATE alert_event SET notified_at = $1`, utcDB(time.Now().Add(-6*time.Hour-time.Minute)))
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)
	assert.Contains(t, f.Mail()[1].Data, "Still active.")

	a.settled(t, a.temp, 80)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 3)
	assert.Contains(t, f.Mail()[2].Data, "Climate back to normal in Tent")
	ev = a.events(t)
	require.Len(t, ev, 1)
	require.NotNil(t, ev[0].Resolved)
	assert.Empty(t, ev[0].Pending)
}

func TestAlertEngine_NotificationsOffRepeatEndAndClose_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))

	a.held(t, a.temp, 95)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.DeliveryOff, ev[0].Delivery)

	// A reminder with notifications off only keeps time.
	old := time.Now().Add(-6*time.Hour - time.Minute).UTC()
	a.exec(t, `UPDATE alert_event SET notified_at = $1`, utcDB(old))
	a.check()
	assert.True(t, a.events(t)[0].Notified.After(old.Add(time.Minute)))

	// End, with no email to send ($3 = 0).
	a.settled(t, a.temp, 80)
	a.check()
	ev = a.events(t)
	require.NotNil(t, ev[0].Resolved)
	assert.Equal(t, alerts.DeliveryOff, ev[0].Delivery)

	// A new alert, closed because its limit was removed.
	a.fresh(t, a.temp)
	a.held(t, a.temp, 95)
	a.check()
	a.exec(t, `UPDATE climate_target SET high = NULL WHERE measure = 'temp'`)
	a.check()
	ev = a.events(t)
	require.Len(t, ev, 2)
	assert.NotNil(t, ev[0].Resolved)
	assert.NotNil(t, ev[1].Resolved)
}

func TestAlertEngine_SensorsOffline_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 70, $2)`, a.temp, utcDB(time.Now().Add(-16*time.Minute)))
	a.check()
	a.send()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.KindOffline, ev[0].Kind)
	require.Len(t, f.Mail(), 1)

	// A reminder with no reading to record (a nil value in COALESCE).
	a.exec(t, `UPDATE alert_event SET notified_at = $1`, utcDB(time.Now().Add(-6*time.Hour-time.Minute)))
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)

	// Back for 15 minutes since the alert started.
	a.exec(t, `UPDATE alert_event SET started_at = $1 WHERE kind = 'offline'`, utcDB(time.Now().Add(-20*time.Minute)))
	a.settled(t, a.temp, 70)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 3)
	assert.Contains(t, f.Mail()[2].Data, "Sensors back online in Tent")
}

func TestAlertEvents_Clear_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	a.exec(t, `INSERT INTO alert_event (kind, zone_id, side, started_at, notified_at, resolved_at) VALUES ('stage', $1, 'low', $2, $2, $2)`, a.tent, utcDB(time.Now()))
	id := a.events(t)[0].ID
	resp := a.client.SessionPostJSON(t, "/settings/alerts/events/"+strconv.Itoa(id)+"/clear", a.token, map[string]any{})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, a.events(t))
	got, err := handlers.RecentAlerts(a.db, true, 10)
	require.NoError(t, err)
	assert.Len(t, got, 1)
	resp = a.client.SessionPostJSON(t, "/settings/alerts/clear-ended", a.token, map[string]any{})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAlertEngine_FailedSendRetries_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain, fakes.WithSMTPLogin("grower@example.com", "right-pass"))
	a.mail(t, f)
	sealed, err := alerts.SealPassword(a.dataDir, "wrong-pass")
	require.NoError(t, err)
	testutil.UpsertSetting(t, a.db, alerts.SettingEmailPassword, sealed)
	a.held(t, a.temp, 95)
	a.check()
	a.send()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.DeliveryFailed, ev[0].Delivery)
	assert.Equal(t, alerts.NoticeFired, ev[0].Pending)

	a.exec(t, `UPDATE alert_event SET retry_at = $1`, utcDB(time.Now().Add(-time.Minute)))
	sealed, err = alerts.SealPassword(a.dataDir, "right-pass")
	require.NoError(t, err)
	testutil.UpsertSetting(t, a.db, alerts.SettingEmailPassword, sealed)
	a.send()
	assert.Len(t, f.Mail(), 1)
	assert.Equal(t, alerts.DeliverySent, a.events(t)[0].Delivery)
}

func TestAlertEngine_LightMismatchAndPage_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	light, now := a.lightZone(t)
	a.readings(t, light, make([]float64, 16)...)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.KindLight, ev[0].Kind)
	assert.NotNil(t, ev[0].SensorID, "keeps the sensor")
	assert.WithinDuration(t, now.Add(-6*time.Hour), ev[0].Started, 2*time.Minute)

	body := a.page(t, "/settings")
	assert.Contains(t, body, `"mismatch":true`)
	assert.Contains(t, body, `"alert":"off"`)

	a.settled(t, light, 100)
	a.check()
	assert.NotNil(t, a.events(t)[0].Resolved)
}

func TestAlertEngine_PrunesOldEnded_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	old := utcDB(time.Now().Add(-100 * 24 * time.Hour))
	a.exec(t, `INSERT INTO alert_event (kind, zone_id, side, started_at, notified_at, resolved_at) VALUES ('stage', $1, 'low', $2, $2, $2)`, a.tent, old)
	a.check()
	assert.Empty(t, a.events(t))
}

func TestAlertRules_RoutesAndCycle_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	co2 := a.co2Sensor(t)
	rule := a.rule(t, co2)
	assert.Contains(t, a.page(t, "/settings"), `"sensor_id":`+strconv.Itoa(co2))

	a.held(t, co2, 1500)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)
	assert.Contains(t, f.Mail()[0].Data, "1500 ppm")

	a.settled(t, co2, 800)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)

	// An edit ends an open alert quietly, and a delete leaves no rule.
	a.fresh(t, co2)
	a.held(t, co2, 1500)
	a.check()
	body := ruleBody(co2)
	body["limit"] = 1200.5
	resp := a.client.SessionPutJSON(t, "/settings/alerts/rules/"+strconv.Itoa(rule), a.token, body)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotNil(t, a.events(t)[0].Resolved)

	resp = a.client.SessionDeleteJSON(t, "/settings/alerts/rules/"+strconv.Itoa(rule), a.token, nil)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var n int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM alert_rule`).Scan(&n))
	assert.Zero(t, n)
}

func TestAlertEvent_OneOpenRowPerAlert_Postgres(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	ins := `INSERT INTO alert_event (kind, zone_id, side, started_at, notified_at) VALUES ('stage', 1, 'low', '2026-10-30 10:00:00', '2026-10-30 10:00:00')`
	_, err := db.Exec(ins)
	require.NoError(t, err)
	_, err = db.Exec(ins)
	assert.Error(t, err)
	res, err := db.Exec(ins + ` ON CONFLICT DO NOTHING`)
	require.NoError(t, err, "what the engine's Start runs")
	n, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Zero(t, n)
	_, err = db.Exec(`UPDATE alert_event SET resolved_at = '2026-10-30 11:00:00'`)
	require.NoError(t, err)
	_, err = db.Exec(ins)
	assert.NoError(t, err, "a new alert after the old one ended")

	got, err := handlers.RecentAlerts(db, false, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Nil(t, got[0].Resolved, "the open one first")
}

// The readings Recent alerts shows (2.3): DOUBLE PRECISION columns, the limits
// text, NULL peaks in the UPDATE, the filtered Clear All Ended and the chart query.
func TestAlertReadings_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	vals := repeated(92, 16)
	vals[10] = 97
	a.readings(t, a.temp, vals...)
	a.check()
	a.exec(t, `UPDATE climate_target SET high = 33 WHERE measure = 'temp'`)
	a.fresh(t, a.temp)
	a.readings(t, a.temp, repeated(99, 10)...)
	a.check()

	ev := a.events(t)
	require.Len(t, ev, 1)
	require.NotNil(t, ev[0].StartValue)
	require.NotNil(t, ev[0].Peak)
	assert.InDelta(t, fToC(99), *ev[0].Peak, 0.01)
	require.Len(t, ev[0].Limits, 2)
	assert.Equal(t, "veg", ev[0].Limits[0].Stage)

	resp := a.client.Get("/settings/alerts/events/" + strconv.Itoa(ev[0].ID) + "/chart")
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	a.settled(t, a.temp, 80)
	a.exec(t, `UPDATE climate_target SET high = 32.22 WHERE measure = 'temp'`)
	a.check()
	resp = a.client.SessionPostJSON(t, "/settings/alerts/clear-ended", a.token, map[string]any{"ids": []int{ev[0].ID}})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, err := handlers.RecentAlerts(a.db, true, 10)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}
