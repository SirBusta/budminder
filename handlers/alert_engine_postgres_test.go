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
	a.mode(t, alerts.ModeEmail)

	a.readings(t, a.temp, 95, 95)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.DeliverySent, ev[0].Delivery)
	assert.True(t, ev[0].Emailed)
	assert.InDelta(t, 32.22, *ev[0].Limit, 0.01)
	assert.WithinDuration(t, time.Now(), ev[0].Started, time.Minute, "started_at reads back in UTC")

	a.exec(t, `UPDATE alert_event SET notified_at = $1`, utcDB(time.Now().Add(-31*time.Minute)))
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)
	assert.Contains(t, f.Mail()[1].Data, "Still active.")

	a.fresh(t, a.temp)
	a.readings(t, a.temp, 80, 80)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 3)
	assert.Contains(t, f.Mail()[2].Data, "Temperature back to normal in Tent")
	ev = a.events(t)
	require.Len(t, ev, 1)
	require.NotNil(t, ev[0].Resolved)
	assert.Empty(t, ev[0].Pending)
}

func TestAlertEngine_DryRunRepeatEndAndClose_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))

	a.readings(t, a.temp, 95, 95)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.DeliveryDryRun, ev[0].Delivery)

	// Repeat in dry run only keeps time.
	old := time.Now().Add(-31 * time.Minute).UTC()
	a.exec(t, `UPDATE alert_event SET notified_at = $1`, utcDB(old))
	a.check()
	assert.True(t, a.events(t)[0].Notified.After(old.Add(time.Minute)))

	// End, with the email switch off ($3 = 0).
	a.fresh(t, a.temp)
	a.readings(t, a.temp, 80, 80)
	a.check()
	ev = a.events(t)
	require.NotNil(t, ev[0].Resolved)
	assert.Equal(t, alerts.DeliveryDryRun, ev[0].Delivery)

	// A new alert, closed because the mode went Off.
	a.fresh(t, a.temp)
	a.readings(t, a.temp, 95, 95)
	a.check()
	a.mode(t, alerts.ModeOff)
	a.check()
	ev = a.events(t)
	require.Len(t, ev, 2)
	assert.NotNil(t, ev[0].Resolved)
	assert.NotNil(t, ev[1].Resolved)
}

func TestAlertEngine_FailedSendRetries_Postgres(t *testing.T) {
	t.Parallel()
	a := newAlertEnvOn(t, testutil.NewTestPostgresDB(t))
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain, fakes.WithSMTPLogin("grower@example.com", "right-pass"))
	a.mail(t, f)
	sealed, err := alerts.SealPassword(a.dataDir, "wrong-pass")
	require.NoError(t, err)
	testutil.UpsertSetting(t, a.db, alerts.SettingEmailPassword, sealed)
	a.mode(t, alerts.ModeEmail)
	a.readings(t, a.temp, 95, 95)
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
	assert.Contains(t, body, `"alert":"dry_run"`)

	a.fresh(t, light)
	a.readings(t, light, 100, 100)
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
	a.mode(t, alerts.ModeEmail)
	co2 := a.co2Sensor(t)
	rule := a.rule(t, co2)
	assert.Contains(t, a.page(t, "/settings"), `"sensor_id":`+strconv.Itoa(co2))

	a.readings(t, co2, 1500, 1500, 1500, 1500, 1500, 1500)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)
	assert.Contains(t, f.Mail()[0].Data, "1500 ppm")

	a.fresh(t, co2)
	a.readings(t, co2, 800, 800, 800, 800, 800, 800)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)

	// An edit ends an open alert quietly, and a delete leaves no rule.
	a.fresh(t, co2)
	a.readings(t, co2, 1500, 1500, 1500, 1500, 1500, 1500)
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

	got, err := handlers.RecentAlerts(db, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Nil(t, got[0].Resolved, "the open one first")
}
