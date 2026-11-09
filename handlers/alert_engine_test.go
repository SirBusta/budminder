package handlers_test

// The background check behind environment alerts (handlers/alert_engine.go),
// against a real database and fake mail server.

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
	"isley/config"
	"isley/handlers"
	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

type alertEnv struct {
	*sensorEnv
	engine         *handlers.AlertEngine
	tent           int
	temp, hum, vpd int
	dataDir        string
}

// newAlertEnv makes a zone "Tent" with a Veg plant and chosen climate sensors,
// the default limits (temperature 55 to 90 °F, humidity 45% to 85%, VPD none)
// and no light schedule.
func newAlertEnv(t *testing.T) *alertEnv {
	t.Helper()
	return newAlertEnvOn(t, testutil.NewTestDB(t))
}

func newAlertEnvOn(t *testing.T, db *sql.DB) *alertEnv {
	t.Helper()
	e := newSensorEnvOn(t, db)
	require.NoError(t, handlers.EnsureClimateTargets(e.db))
	a := &alertEnv{sensorEnv: e, dataDir: t.TempDir()}
	a.tent = testutil.SeedZone(t, e.db, "Tent")
	sensor := func(kind, unit string) int {
		id := testutil.SeedSensor(t, e.db, "acinfinity", "D1", kind)
		e.exec(t, `UPDATE sensors SET zone_id = $1, unit = $2 WHERE id = $3`, a.tent, unit, id)
		return id
	}
	a.temp, a.hum, a.vpd = sensor("ACI.1.0", "°F"), sensor("ACI.1.2", "%"), sensor("ACI.1.3", "kPa")
	livingPlant(t, e, "Veg plant", a.tent, "Veg")
	e.exec(t, `UPDATE zones SET vpd_temp_sensor_id = $1, vpd_humidity_sensor_id = $2, vpd_source = 'sensor', vpd_sensor_id = $3 WHERE id = $4`,
		a.temp, a.hum, a.vpd, a.tent)
	store := config.NewStore()
	store.SetPollingInterval(60)
	a.engine = handlers.NewAlertEngine(e.db, store, a.dataDir)
	return a
}

// readings saves one reading a minute, the last one now.
func (a *alertEnv) readings(t *testing.T, sensor int, vals ...float64) {
	t.Helper()
	now := time.Now().UTC()
	for i, v := range vals {
		at := now.Add(time.Duration(i-len(vals)+1) * time.Minute).Format("2006-01-02 15:04:05")
		a.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`, sensor, v, at)
	}
}

// fresh removes a sensor's readings, to write a new story.
func (a *alertEnv) fresh(t *testing.T, sensor int) {
	t.Helper()
	a.exec(t, `DELETE FROM sensor_data WHERE sensor_id = $1`, sensor)
}

// held saves 16 readings of v, one a minute: enough to start an alert.
func (a *alertEnv) held(t *testing.T, sensor int, v float64) {
	t.Helper()
	a.readings(t, sensor, repeated(v, 16)...)
}

// settled replaces a sensor's readings with 31 of v: enough to end an alert.
func (a *alertEnv) settled(t *testing.T, sensor int, v float64) {
	t.Helper()
	a.fresh(t, sensor)
	a.readings(t, sensor, repeated(v, 31)...)
}

func repeated(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func (a *alertEnv) mail(t *testing.T, f *fakes.FakeSMTP) {
	t.Helper()
	sealed, err := alerts.SealPassword(a.dataDir, f.Pass)
	require.NoError(t, err)
	for k, v := range map[string]string{
		alerts.SettingEmailOn: "1", alerts.SettingEmailHost: f.Host, alerts.SettingEmailPort: strconv.Itoa(f.Port),
		alerts.SettingEmailSecurity: "none", alerts.SettingEmailUser: f.User, alerts.SettingEmailPassword: sealed,
		alerts.SettingEmailFrom: "budminder@example.com", alerts.SettingEmailTo: "grower@example.com",
		alerts.SettingLinkAddress: "http://192.168.1.10:8080",
	} {
		testutil.UpsertSetting(t, a.db, k, v)
	}
}

func (a *alertEnv) events(t *testing.T) []alerts.Event {
	t.Helper()
	ev, err := handlers.RecentAlerts(a.db, false, 100)
	require.NoError(t, err)
	return ev
}

func (a *alertEnv) check() { a.engine.Check(context.Background()) }

func (a *alertEnv) send() { a.engine.SendPending(context.Background()) }

func TestAlertEngine_NotificationsOffListsAndSendsNothing(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	testutil.UpsertSetting(t, a.db, alerts.SettingEmailOn, "0")
	a.held(t, a.temp, 95) // °F, 90 °F limit
	a.check()
	a.send()

	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.KindStage, ev[0].Kind)
	assert.Equal(t, "high", ev[0].Side)
	assert.Equal(t, "Tent", ev[0].Zone)
	assert.Equal(t, alerts.DeliveryPaused, ev[0].Delivery, "email set up, then turned off")
	assert.InDelta(t, 35, *ev[0].Value, 0.01, "stored in °C")
	assert.InDelta(t, 32.22, *ev[0].Limit, 0.01)
	assert.Empty(t, ev[0].Pending)
	assert.Empty(t, f.Mail())
}

func TestAlertEngine_NeverSetUpRecordsOff(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.held(t, a.temp, 95)
	a.check()
	a.send()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.DeliveryOff, ev[0].Delivery)
}

func TestAlertEngine_NoAlertsOutOfTheBoxForVPD(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.held(t, a.vpd, 3)
	a.check()
	assert.Empty(t, a.events(t), "VPD has a Target and no limits")
}

func TestAlertEngine_BriefSpikeSendsNothing(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.readings(t, a.temp, 80, 80, 80, 95, 95, 95, 95, 95, 95)
	a.check()
	assert.Empty(t, a.events(t))
	a.fresh(t, a.temp)
	vals := repeated(95, 16)
	vals[7] = 89
	a.readings(t, a.temp, vals...)
	a.check()
	assert.Empty(t, a.events(t), "one reading in range restarts the 15 minutes")
}

func TestAlertEngine_EmailGroupsAndNeverFiresTwice(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain, fakes.WithSMTPLogin("grower@example.com", "app-secret-99"))
	a.mail(t, f)
	a.held(t, a.temp, 95)
	a.held(t, a.hum, 90) // Veg High is 85%
	a.check()
	a.send()

	mail := f.Mail()
	require.Len(t, mail, 1, "both alerts in one email")
	assert.Contains(t, mail[0].Data, "and 1 more")
	assert.Contains(t, mail[0].Data, "Temperature high in Tent")
	assert.Contains(t, mail[0].Data, "95 =C2=B0F")
	assert.Contains(t, mail[0].Data, "Humidity high in Tent")
	assert.Contains(t, mail[0].Data, "widen the limits")
	assert.Contains(t, mail[0].Data, "http://192.168.1.10:8080/settings#alerts")
	assert.NotContains(t, mail[0].Data, "app-secret-99")

	// The next poll changes nothing, and neither does a restart.
	a.check()
	a.send()
	restarted := handlers.NewAlertEngine(a.db, a.engine.Store, a.dataDir)
	restarted.Check(context.Background())
	restarted.SendPending(context.Background())
	assert.Len(t, f.Mail(), 1)
	ev := a.events(t)
	require.Len(t, ev, 2)
	for _, e := range ev {
		assert.Equal(t, alerts.DeliverySent, e.Delivery)
		assert.True(t, e.Emailed)
		assert.Nil(t, e.Resolved)
		assert.NotContains(t, e.DeliveryError, "app-secret-99")
	}
}

func TestAlertEngine_RemindersComeTogetherAndOneAllClear(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.held(t, a.temp, 95)
	a.held(t, a.hum, 90)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)

	// Not yet six hours.
	a.exec(t, `UPDATE alert_event SET notified_at = $1`, utcDB(time.Now().Add(-5*time.Hour)))
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)

	// Six hours since one of them: both go out in one reminder.
	a.exec(t, `UPDATE alert_event SET notified_at = $1 WHERE measure = 'temp'`, utcDB(time.Now().Add(-6*time.Hour-time.Minute)))
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)
	assert.Contains(t, f.Mail()[1].Data, "Still active.")
	assert.Contains(t, f.Mail()[1].Data, "Temperature high")
	assert.Contains(t, f.Mail()[1].Data, "Humidity high")

	// Temperature settles first: no email, because the zone isn't all clear yet.
	a.settled(t, a.temp, 80)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)

	// Humidity settles: one all clear.
	a.settled(t, a.hum, 60)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 3)
	assert.Contains(t, f.Mail()[2].Data, "Climate back to normal in Tent")
	for _, e := range a.events(t) {
		require.NotNil(t, e.Resolved)
		assert.Empty(t, e.Pending)
	}
}

func TestAlertEngine_AllClearWaitsThirtyMinutes(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.held(t, a.temp, 95)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)

	a.readings(t, a.temp, 80, 80, 80, 80, 80)
	a.check()
	a.send()
	assert.Len(t, f.Mail(), 1, "only 5 minutes in range")
	assert.Nil(t, a.events(t)[0].Resolved)
}

func TestAlertEngine_NoAllClearWithoutAnEmailedAlert(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.held(t, a.temp, 95)
	a.check() // notifications off: listed, not emailed
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.settled(t, a.temp, 80)
	a.check()
	a.send()
	assert.Empty(t, f.Mail())
	assert.NotNil(t, a.events(t)[0].Resolved)
}

func TestAlertEngine_FailedSendRetries(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
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
	assert.NotEmpty(t, ev[0].DeliveryError)
	assert.NotContains(t, ev[0].DeliveryError, "wrong-pass")
	assert.Equal(t, alerts.NoticeFired, ev[0].Pending, "still due")

	// Waits for the retry time, then goes through with the password fixed.
	sealed, err = alerts.SealPassword(a.dataDir, "right-pass")
	require.NoError(t, err)
	testutil.UpsertSetting(t, a.db, alerts.SettingEmailPassword, sealed)
	a.send()
	assert.Empty(t, f.Mail(), "too soon")
	a.exec(t, `UPDATE alert_event SET retry_at = $1`, time.Now().UTC().Add(-time.Minute).Format("2006-01-02 15:04:05"))
	a.send()
	assert.Len(t, f.Mail(), 1)
	ev = a.events(t)
	assert.Equal(t, alerts.DeliverySent, ev[0].Delivery)
	assert.Empty(t, ev[0].DeliveryError)
}

// webhook turns the webhook method on, sending to f.
func (a *alertEnv) webhook(t *testing.T, f *fakes.FakeNotify) {
	t.Helper()
	sealed, err := alerts.SealPassword(a.dataDir, f.URL+"/hook")
	require.NoError(t, err)
	testutil.UpsertSetting(t, a.db, alerts.SettingWebhookOn, "1")
	testutil.UpsertSetting(t, a.db, alerts.SettingWebhookURL, sealed)
}

func TestAlertEngine_EveryMethodAndRetryOnlyTheOneThatFailed(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	hook := fakes.NewFakeNotify(t)
	a.webhook(t, hook)
	hook.SetStatus(503, "")
	a.held(t, a.temp, 95)
	a.check()
	a.send()

	assert.Len(t, f.Mail(), 1, "email worked")
	require.Len(t, hook.Requests(), 1)
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.DeliveryFailed, ev[0].Delivery)
	assert.Contains(t, ev[0].DeliveryError, "Webhook: HTTP 503")
	assert.NotContains(t, ev[0].DeliveryError, hook.URL, "the address is a secret")
	assert.Equal(t, "webhook", ev[0].RetryMethods)
	assert.True(t, ev[0].Emailed, "it went out once, so an all clear follows")

	hook.SetStatus(200, "")
	a.exec(t, `UPDATE alert_event SET retry_at = $1`, time.Now().UTC().Add(-time.Minute).Format("2006-01-02 15:04:05"))
	a.send()
	assert.Len(t, f.Mail(), 1, "email isn't sent twice")
	require.Len(t, hook.Requests(), 2)
	assert.Contains(t, hook.Requests()[1].Body, `"event":"start"`)
	ev = a.events(t)
	assert.Equal(t, alerts.DeliverySent, ev[0].Delivery)
	assert.Empty(t, ev[0].RetryMethods)

	// The all clear goes to both.
	a.settled(t, a.temp, 80)
	a.check()
	a.send()
	assert.Len(t, f.Mail(), 2)
	require.Len(t, hook.Requests(), 3)
	assert.Contains(t, hook.Requests()[2].Body, `"event":"end"`)
}

func TestAlertEngine_WebhookAloneCountsAsOn(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	hook := fakes.NewFakeNotify(t)
	a.webhook(t, hook)
	a.held(t, a.temp, 95)
	a.check()
	a.send()
	require.Len(t, hook.Requests(), 1)
	ev := a.events(t)
	assert.Equal(t, alerts.DeliverySent, ev[0].Delivery)

	// Turned off with something set up: paused.
	testutil.UpsertSetting(t, a.db, alerts.SettingWebhookOn, "0")
	a.held(t, a.hum, 90)
	a.check()
	for _, e := range a.events(t) {
		if e.Measure == "rh" {
			assert.Equal(t, alerts.DeliveryPaused, e.Delivery)
		}
	}
}

func TestAlertEngine_ClosesQuietlyWhenItNoLongerApplies(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(a *alertEnv, t *testing.T){
		"limit removed": func(a *alertEnv, t *testing.T) {
			a.exec(t, `UPDATE climate_target SET high = NULL WHERE measure = 'temp'`)
		},
		"sensor unassigned": func(a *alertEnv, t *testing.T) {
			a.exec(t, `UPDATE zones SET vpd_temp_sensor_id = NULL WHERE id = $1`, a.tent)
		},
		"zone deleted": func(a *alertEnv, t *testing.T) {
			a.exec(t, `UPDATE plant SET zone_id = NULL WHERE zone_id = $1`, a.tent)
			a.exec(t, `UPDATE sensors SET zone_id = NULL WHERE zone_id = $1`, a.tent)
			a.exec(t, `DELETE FROM zones WHERE id = $1`, a.tent)
		},
		"no plants left": func(a *alertEnv, t *testing.T) {
			a.exec(t, `UPDATE plant SET zone_id = NULL WHERE zone_id = $1`, a.tent)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newAlertEnv(t)
			f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
			a.mail(t, f)
			a.held(t, a.temp, 95)
			a.check()
			a.send()
			require.Len(t, f.Mail(), 1)
			change(a, t)
			a.check()
			a.send()
			assert.Len(t, f.Mail(), 1, "closing sends nothing")
			ev := a.events(t)
			require.Len(t, ev, 1)
			assert.NotNil(t, ev[0].Resolved)
		})
	}
}

func TestAlertEngine_FlatLimitsWhenByStageIsOff(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	testutil.UpsertSetting(t, a.db, "climate_temp_by_stage", "0")
	testutil.UpsertSetting(t, a.db, "climate_temp_limits", ",26.6667") // 80 °F
	a.held(t, a.temp, 85)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.InDelta(t, 26.6667, *ev[0].Limit, 1e-3)
}

func TestAlertEngine_StaleSensorHolds(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.held(t, a.temp, 95)
	a.check()
	require.Len(t, a.events(t), 1)
	a.exec(t, `UPDATE sensor_data SET create_dt = $1 WHERE sensor_id = $2`, time.Now().UTC().Add(-10*time.Minute).Format("2006-01-02 15:04:05"), a.temp)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Nil(t, ev[0].Resolved, "not reporting isn't back to normal")
}

func TestAlertEngine_SensorsOffline(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	offline := func(ev []alerts.Event) *alerts.Event {
		for i := range ev {
			if ev[i].Kind == alerts.KindOffline {
				return &ev[i]
			}
		}
		return nil
	}
	a.exec(t, `UPDATE sensors SET name = 'Tent temp' WHERE id = $1`, a.temp)
	a.exec(t, `UPDATE sensors SET name = 'Tent humidity' WHERE id = $1`, a.hum)
	a.readings(t, a.hum, 60, 60)

	// 10 minutes without a reading: not yet. A sensor with no readings at all hasn't started.
	a.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 70, $2)`, a.temp, utcDB(time.Now().Add(-10*time.Minute)))
	a.check()
	assert.Nil(t, offline(a.events(t)))

	a.fresh(t, a.temp)
	a.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 70, $2)`, a.temp, utcDB(time.Now().Add(-16*time.Minute)))
	a.check()
	a.send()
	ev := offline(a.events(t))
	require.NotNil(t, ev)
	assert.Equal(t, "Tent temp", ev.Detail)
	assert.Equal(t, alerts.Critical, alerts.Severity(ev.Kind))
	require.Len(t, f.Mail(), 1)
	assert.Contains(t, f.Mail()[0].Data, "Sensors offline in Tent")
	assert.Contains(t, f.Mail()[0].Data, "No readings for 15 minutes from Tent temp.")

	// A second sensor stops: the same alert names both, with no second email.
	a.exec(t, `UPDATE sensor_data SET create_dt = $1 WHERE sensor_id = $2`, utcDB(time.Now().Add(-50*time.Minute)), a.hum)
	a.check()
	a.send()
	ev = offline(a.events(t))
	assert.Contains(t, ev.Detail, "Tent temp")
	assert.Contains(t, ev.Detail, "Tent humidity")
	assert.Len(t, f.Mail(), 1)

	// Back with one new reading: still open. Readings from before the alert
	// started don't count toward being back.
	a.exec(t, `UPDATE alert_event SET started_at = $1 WHERE kind = 'offline'`, utcDB(time.Now().Add(-20*time.Minute)))
	for _, s := range []int{a.temp, a.hum} {
		a.fresh(t, s)
		a.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 65, $2)`, s, utcDB(time.Now().Add(-25*time.Minute)))
		a.readings(t, s, 65)
	}
	a.check()
	assert.Nil(t, offline(a.events(t)).Resolved)

	// Back for 15 minutes: one email.
	a.settled(t, a.temp, 70)
	a.settled(t, a.hum, 60)
	a.check()
	a.send()
	assert.NotNil(t, offline(a.events(t)).Resolved)
	require.Len(t, f.Mail(), 2)
	assert.Contains(t, f.Mail()[1].Data, "Sensors back online in Tent")
}

func TestAlertEngine_OfflineCoversSensorAlerts(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	co2 := a.co2Sensor(t)
	a.rule(t, co2)
	a.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 800, $2)`, co2, utcDB(time.Now().Add(-time.Hour)))
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.KindOffline, ev[0].Kind)
	assert.Equal(t, "CO2", ev[0].Detail)

	// A sensor nothing reads isn't watched.
	a2 := newAlertEnv(t)
	other := a2.co2Sensor(t)
	a2.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 800, $2)`, other, utcDB(time.Now().Add(-time.Hour)))
	a2.check()
	assert.Empty(t, a2.events(t))
}

func TestAlertEngine_MetricSensorAndUnits(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.exec(t, `UPDATE sensors SET unit = '°C' WHERE id = $1`, a.temp)
	a.held(t, a.temp, 31)
	a.check()
	assert.Empty(t, a.events(t), "31 °C is under the limit")
	a.fresh(t, a.temp)
	a.held(t, a.temp, 33)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.InDelta(t, 33, *ev[0].Value, 0.001)
}

// lightZone gives the zone a light sensor and a schedule with now safely in
// the middle of the lights-on period (6 h after on, 6 h before off), in UTC.
func (a *alertEnv) lightZone(t *testing.T) (light int, now time.Time) {
	t.Helper()
	a.engine.Store.SetTimezone("UTC")
	a.server.ConfigStore.SetTimezone("UTC")
	light = testutil.SeedSensor(t, a.db, "acinfinity", "D1", "ACIP.1")
	a.exec(t, `UPDATE sensors SET zone_id = $1, unit = '%' WHERE id = $2`, a.tent, light)
	now = time.Now().UTC()
	on := now.Add(-6 * time.Hour).Format("15:04")
	off := now.Add(6 * time.Hour).Format("15:04")
	a.exec(t, `UPDATE zones SET light_schedule = $1, light_sensor_id = $2, light_on_above = 0 WHERE id = $3`,
		`{"veg":["`+on+`","`+off+`"]}`, light, a.tent)
	return light, now
}

func TestAlertEngine_ClockOffsetReplacesMismatch(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	light, now := a.lightZone(t)
	// Four days of readings, lights switching an hour before the schedule.
	on := now.Add(-6 * time.Hour)
	tx, err := a.db.Begin()
	require.NoError(t, err)
	for at := now.Add(-96 * time.Hour); !at.After(now); at = at.Add(5 * time.Minute) {
		// Hours into the 12-hour lights-on period, for the sensor an hour early.
		into := int(at.Add(time.Hour).Sub(on).Minutes()) % 1440
		if into < 0 {
			into += 1440
		}
		v := 0.0
		if into < 720 {
			v = 100
		}
		_, err := tx.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`, light, v, at.Format("2006-01-02 15:04:05"))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	// A mismatch alert from before the offset was found closes when it is.
	a.exec(t, `INSERT INTO alert_event (kind, zone_id, side, started_at, notified_at) VALUES ('light', $1, 'off', $2, $2)`, a.tent, now.Format("2006-01-02 15:04:05"))

	a.check()
	var clock, light2 *alerts.Event
	for _, e := range a.events(t) {
		switch e.Kind {
		case alerts.KindClock:
			clock = &e
		case alerts.KindLight:
			light2 = &e
		}
	}
	require.NotNil(t, clock)
	assert.Equal(t, alerts.SideEarly, clock.Side)
	assert.InDelta(t, 60, *clock.Limit, 6)
	assert.Nil(t, clock.Resolved)
	assert.Contains(t, clock.Detail, "|")
	require.NotNil(t, light2)
	assert.NotNil(t, light2.Resolved, "the offset explains the disagreement")
}

func TestAlertEngine_LightMismatch(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	light, now := a.lightZone(t)

	vals := make([]float64, 16) // all zero: lights off for 15 minutes
	a.readings(t, light, vals...)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.KindLight, ev[0].Kind)
	assert.Equal(t, alerts.SideOff, ev[0].Side)
	assert.WithinDuration(t, now.Add(-6*time.Hour), ev[0].Started, 2*time.Minute, "since the switch it missed")

	// Edit zone's Light check says the same, with the open alert.
	body := a.page(t, "/settings")
	assert.Contains(t, body, `"mismatch":true`)
	assert.Contains(t, body, `"alert":"off"`)

	// The sensor comes back on: the alert ends after 30 minutes of agreeing.
	a.fresh(t, light)
	a.readings(t, light, 100, 100)
	a.check()
	assert.Nil(t, a.events(t)[0].Resolved)
	a.settled(t, light, 100)
	a.check()
	ev = a.events(t)
	require.Len(t, ev, 1)
	assert.NotNil(t, ev[0].Resolved)
}

func TestAlertEngine_LightFailsAfterItsSwitch(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	light, now := a.lightZone(t)

	vals := make([]float64, 60) // on for 40 minutes, then off for 20
	for i := range 40 {
		vals[i] = 100
	}
	a.readings(t, light, vals...)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.WithinDuration(t, now.Add(-19*time.Minute), ev[0].Started, 2*time.Minute, "since the lights went off, not the switch")

	body := a.page(t, "/settings")
	assert.Contains(t, body, `"mismatch":true`)
}

func TestRecentAlerts_OpenFirst(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	ins := `INSERT INTO alert_event (kind, zone_id, side, started_at, notified_at, resolved_at) VALUES ($1, 1, $2, $3, $3, $4)`
	at := func(h int) string { return time.Date(2026, 10, 30, h, 0, 0, 0, time.UTC).Format("2006-01-02 15:04:05") }
	_, err := db.Exec(ins, alerts.KindStage, "low", at(10), nil)
	require.NoError(t, err)
	_, err = db.Exec(ins, alerts.KindStage, "high", at(12), at(13))
	require.NoError(t, err)
	got, err := handlers.RecentAlerts(db, false, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "low", got[0].Side, "the open one first, though older")

	// The Cleared tab has only what was cleared.
	_, err = db.Exec(`UPDATE alert_event SET cleared = 1 WHERE side = 'high'`)
	require.NoError(t, err)
	got, err = handlers.RecentAlerts(db, false, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "low", got[0].Side)
	got, err = handlers.RecentAlerts(db, true, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "high", got[0].Side)
	assert.True(t, got[0].Cleared)
}

func TestAlertEvent_OneOpenRowPerAlert(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	ins := `INSERT INTO alert_event (kind, zone_id, side, started_at, notified_at) VALUES ('stage', 1, 'low', '2026-10-30 10:00:00', '2026-10-30 10:00:00')`
	_, err := db.Exec(ins)
	require.NoError(t, err)
	_, err = db.Exec(ins)
	assert.Error(t, err)
	_, err = db.Exec(`UPDATE alert_event SET resolved_at = '2026-10-30 11:00:00'`)
	require.NoError(t, err)
	_, err = db.Exec(ins)
	assert.NoError(t, err, "a new alert after the old one ended")
}
