package handlers_test

// The background check behind environment alerts (handlers/alert_engine.go),
// against a real database and fake mail server.

import (
	"context"
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
// a 32.2 °C (90 °F) high safety limit, and no light schedule.
func newAlertEnv(t *testing.T) *alertEnv {
	t.Helper()
	e := newSensorEnv(t)
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
	testutil.UpsertSetting(t, e.db, "climate_temp_limits", ",32.2222")
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

func (a *alertEnv) mode(t *testing.T, mode string) {
	t.Helper()
	testutil.UpsertSetting(t, a.db, alerts.SettingClimateMode, mode)
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
	ev, err := handlers.RecentAlerts(a.db, 100)
	require.NoError(t, err)
	return ev
}

func (a *alertEnv) check() { a.engine.Check(context.Background()) }

func (a *alertEnv) send() { a.engine.SendPending(context.Background()) }

func TestAlertEngine_DryRunListsAndSendsNothing(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	// Dry run is the default: no mode saved.
	a.readings(t, a.temp, 80, 95, 96) // °F, 90 °F limit
	a.check()
	a.send()

	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.KindSafety, ev[0].Kind)
	assert.Equal(t, "high", ev[0].Side)
	assert.Equal(t, "Tent", ev[0].Zone)
	assert.Equal(t, alerts.DeliveryDryRun, ev[0].Delivery)
	assert.InDelta(t, 35.56, *ev[0].Value, 0.01, "stored in °C")
	assert.InDelta(t, 32.22, *ev[0].Limit, 0.01)
	assert.Empty(t, ev[0].Pending)
	assert.Empty(t, f.Mail())
}

func TestAlertEngine_EmailGroupsAndNeverFiresTwice(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain, fakes.WithSMTPLogin("grower@example.com", "app-secret-99"))
	a.mail(t, f)
	a.mode(t, alerts.ModeEmail)
	a.readings(t, a.temp, 95, 95, 95)
	vals := make([]float64, 11)
	for i := range vals {
		vals[i] = 2.0 // Veg High is 1.2 (lights on, no schedule)
	}
	a.readings(t, a.vpd, vals...)
	a.check()
	a.send()

	mail := f.Mail()
	require.Len(t, mail, 1, "both alerts in one email")
	assert.Contains(t, mail[0].Data, "Temperature high in Tent", "the most serious names the subject")
	assert.Contains(t, mail[0].Data, "and 1 more")
	assert.Contains(t, mail[0].Data, "95 =C2=B0F")
	assert.Contains(t, mail[0].Data, "VPD high in Tent")
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

func TestAlertEngine_RepeatAndBackToNormal(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.mode(t, alerts.ModeEmail)
	a.readings(t, a.temp, 95, 95)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 1)

	// 31 minutes later, still too hot: a repeat.
	a.exec(t, `UPDATE alert_event SET notified_at = $1`, time.Now().UTC().Add(-31*time.Minute).Format("2006-01-02 15:04:05"))
	a.check()
	a.send()
	require.Len(t, f.Mail(), 2)
	assert.Contains(t, f.Mail()[1].Data, "Still active.")

	// Cool again: one back-to-normal email and the alert ends.
	a.fresh(t, a.temp)
	a.readings(t, a.temp, 80, 80)
	a.check()
	a.send()
	require.Len(t, f.Mail(), 3)
	assert.Contains(t, f.Mail()[2].Data, "Temperature back to normal in Tent")
	ev := a.events(t)
	require.Len(t, ev, 1)
	require.NotNil(t, ev[0].Resolved)
	assert.Empty(t, ev[0].Pending)
	assert.InDelta(t, 26.67, *ev[0].Value, 0.01)
}

func TestAlertEngine_NoBackToNormalWithoutAnEmailedAlert(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.readings(t, a.temp, 95, 95)
	a.check() // dry run: listed, not emailed
	a.mode(t, alerts.ModeEmail)
	a.fresh(t, a.temp)
	a.readings(t, a.temp, 80, 80)
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
	a.mode(t, alerts.ModeEmail)
	a.readings(t, a.temp, 95, 95)
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

func TestAlertEngine_EmailOffIsMarkedNotSent(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.mode(t, alerts.ModeEmail)
	a.readings(t, a.temp, 95, 95)
	a.check()
	a.send()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Equal(t, alerts.DeliveryOff, ev[0].Delivery)
	assert.Empty(t, ev[0].Pending)
}

func TestAlertEngine_ClosesQuietlyWhenItNoLongerApplies(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(a *alertEnv, t *testing.T){
		"mode off": func(a *alertEnv, t *testing.T) { a.mode(t, alerts.ModeOff) },
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
			a.mode(t, alerts.ModeEmail)
			a.readings(t, a.temp, 95, 95)
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

func TestAlertEngine_StaleSensorHolds(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.readings(t, a.temp, 95, 95)
	a.check()
	require.Len(t, a.events(t), 1)
	a.exec(t, `UPDATE sensor_data SET create_dt = $1 WHERE sensor_id = $2`, time.Now().UTC().Add(-20*time.Minute).Format("2006-01-02 15:04:05"), a.temp)
	a.check()
	ev := a.events(t)
	require.Len(t, ev, 1)
	assert.Nil(t, ev[0].Resolved, "not reporting isn't back to normal")
}

func TestAlertEngine_MetricSensorAndUnits(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.exec(t, `UPDATE sensors SET unit = '°C' WHERE id = $1`, a.temp)
	a.readings(t, a.temp, 31, 31)
	a.check()
	assert.Empty(t, a.events(t), "31 °C is under the limit")
	a.fresh(t, a.temp)
	a.readings(t, a.temp, 33, 33)
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
	assert.Contains(t, body, `"alert":"dry_run"`)

	// The sensor comes back on: the alert ends.
	a.fresh(t, light)
	a.readings(t, light, 100, 100)
	a.check()
	ev = a.events(t)
	require.Len(t, ev, 1)
	assert.NotNil(t, ev[0].Resolved)
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
	got, err := handlers.RecentAlerts(db, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "low", got[0].Side, "the open one first, though older")
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
