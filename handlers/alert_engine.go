package handlers

// The background check behind environment alerts: after each sensor poll it
// compares each zone's chosen climate sensors and light check with their
// targets (rules in package alerts and climate), and keeps alert_event up to
// date. Email goes out from its own goroutine (RunSender), so a slow mail
// server can't hold up polling; what's due waits in alert_event.pending, so
// it survives a restart.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"isley/alerts"
	"isley/climate"
	"isley/config"
	"isley/logger"
	"isley/utils"
)

const (
	alertDriftEvery = 10 * time.Minute // how often drifting is checked (it reads hours of readings)
	alertClockEvery = time.Hour        // how often the clock offset is checked (days of readings)
	alertPruneEvery = 24 * time.Hour
	alertKeepFor    = 90 * 24 * time.Hour // ended alerts are kept this long
	alertRetryAfter = 5 * time.Minute     // a failed send waits this long
	alertSendWait   = time.Minute         // the sender also looks this often, for retries
	alertMaxPerMail = 50
)

// AlertEngine runs the checks. Check runs on the watcher's goroutine and
// RunSender on its own; they share only the database.
type AlertEngine struct {
	DB      *sql.DB
	Store   *config.Store
	DataDir string
	Now     func() time.Time

	wake chan struct{}

	lastDrift, lastClock, lastPrune time.Time // Check's goroutine only
}

// NewAlertEngine returns an engine for one store and data folder.
func NewAlertEngine(db *sql.DB, store *config.Store, dataDir string) *AlertEngine {
	return &AlertEngine{DB: db, Store: store, DataDir: dataDir, Now: time.Now, wake: make(chan struct{}, 1)}
}

// settingsLocation is the Settings timezone, or the server's own when none is
// set or it doesn't load.
func settingsLocation(store *config.Store) *time.Location {
	if tz := store.Timezone(); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

func dbTime(t time.Time) string { return t.UTC().Format(utils.LayoutDB) }

const alertEventColumns = `id, kind, zone_id, zone_name, sensor_id, sensor_name, measure, side, value, limit_value, detail,
	started_at, notified_at, resolved_at, delivery, delivery_error, pending, emailed`

func scanAlertEvents(rows *sql.Rows) ([]alerts.Event, error) {
	defer rows.Close()
	var out []alerts.Event
	for rows.Next() {
		var e alerts.Event
		var sensor sql.NullInt64
		var value, limit sql.NullFloat64
		var resolved sql.NullTime
		var emailed int
		if err := rows.Scan(&e.ID, &e.Kind, &e.ZoneID, &e.Zone, &sensor, &e.Sensor, &e.Measure, &e.Side, &value, &limit, &e.Detail,
			&e.Started, &e.Notified, &resolved, &e.Delivery, &e.DeliveryError, &e.Pending, &emailed); err != nil {
			return nil, err
		}
		e.SensorID, e.Value, e.Limit = sqlIntPtr(sensor), nullPtr(value), nullPtr(limit)
		if resolved.Valid {
			t := resolved.Time
			e.Resolved = &t
		}
		e.Emailed = emailed != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// loadOpenAlerts reads every alert that hasn't ended.
func loadOpenAlerts(db *sql.DB) ([]alerts.Event, error) {
	rows, err := db.Query("SELECT " + alertEventColumns + " FROM alert_event WHERE resolved_at IS NULL")
	if err != nil {
		return nil, err
	}
	return scanAlertEvents(rows)
}

// RecentAlerts lists alerts for Settings > Alerts: open ones first, then newest.
func RecentAlerts(db *sql.DB, limit int) ([]alerts.Event, error) {
	rows, err := db.Query(`SELECT `+alertEventColumns+` FROM alert_event
		ORDER BY CASE WHEN resolved_at IS NULL THEN 0 ELSE 1 END, started_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return scanAlertEvents(rows)
}

// recentReadings reads a sensor's readings since a time, oldest first.
// Temperatures come back in °C.
func recentReadings(db *sql.DB, sensorID int, unit, measure string, since time.Time) ([]climate.Reading, error) {
	rows, err := db.Query(`SELECT value, create_dt FROM sensor_data WHERE sensor_id = $1 AND create_dt >= $2 ORDER BY create_dt, id`,
		sensorID, dbTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []climate.Reading
	for rows.Next() {
		var r climate.Reading
		if err := rows.Scan(&r.Value, &r.At); err != nil {
			return nil, err
		}
		if measure == climate.Temp && !utils.IsCelsius(unit) {
			r.Value = utils.FahrenheitToCelsius(r.Value)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// alertRun is one Check.
type alertRun struct {
	e       *AlertEngine
	cfg     alerts.ClimateConfig
	now     time.Time
	open    []alerts.Event
	live    map[string]bool // open alerts that still apply, by alertKey
	changed bool
}

func alertKey(kind string, zone int, measure, side string) string {
	return fmt.Sprintf("%s|%d|%s|%s", kind, zone, measure, side)
}

// Check is one pass over every zone. Errors are logged; the next poll tries again.
func (e *AlertEngine) Check(_ context.Context) {
	log := logger.Log.WithField("func", "AlertEngine.Check")
	cfg, err := alerts.LoadClimateConfig(e.DB)
	if err != nil {
		log.WithError(err).Error("Failed to read the Climate alerts settings")
		return
	}
	open, err := loadOpenAlerts(e.DB)
	if err != nil {
		log.WithError(err).Error("Failed to read open alerts")
		return
	}
	run := &alertRun{e: e, cfg: cfg, now: e.Now(), open: open, live: map[string]bool{}}
	if cfg.Mode != alerts.ModeOff {
		if err := run.zones(); err != nil {
			log.WithError(err).Error("Failed to check climate alerts")
			return
		}
	}
	// Whatever wasn't reached no longer applies: the sensor was unassigned, the
	// zone deleted, its plants gone, or alerts turned off.
	for _, o := range run.open {
		if !run.live[alertKey(o.Kind, o.ZoneID, o.Measure, o.Side)] {
			run.apply(o.ZoneID, "", 0, "", alerts.Change{Key: alerts.Key{Kind: o.Kind, Measure: o.Measure, Side: o.Side}, Action: alerts.Close}, nil)
		}
	}
	if run.now.Sub(e.lastPrune) >= alertPruneEvery {
		e.lastPrune = run.now
		if _, err := e.DB.Exec("DELETE FROM alert_event WHERE resolved_at IS NOT NULL AND resolved_at < $1", dbTime(run.now.Add(-alertKeepFor))); err != nil {
			log.WithError(err).Warn("Failed to prune old alerts")
		}
	}
	if run.changed {
		select {
		case e.wake <- struct{}{}:
		default:
		}
	}
}

// zones checks every zone with a stage.
func (r *alertRun) zones() error {
	e := r.e
	cs, err := LoadClimate(e.DB)
	if err != nil {
		return err
	}
	poll := e.Store.PollingInterval()
	loc := settingsLocation(e.Store)
	sensors := ListManagedSensors(e.DB, poll)
	zones, setups, err := ZoneClimates(e.DB, cs, sensors, r.now, loc)
	if err != nil {
		return err
	}
	byID := map[int]ManagedSensor{}
	for _, s := range sensors {
		byID[s.ID] = s
	}
	checkDrift := r.now.Sub(e.lastDrift) >= alertDriftEvery
	checkClock := r.now.Sub(e.lastClock) >= alertClockEvery
	if checkDrift {
		e.lastDrift = r.now
	}
	if checkClock {
		e.lastClock = r.now
	}
	pollEvery := time.Duration(poll) * time.Second
	stale := staleAfter(poll)
	for id, zc := range zones {
		if zc.Stage.Stage == "" {
			continue
		}
		z := setups[id]
		sw, _ := climate.LastSwitch(z.Schedule, zc.Stage.Stage, r.now, loc)
		for measure, sid := range zc.Sensors {
			r.measure(cs, zc, z, byID[sid], measure, sw, pollEvery, stale, checkDrift)
		}
		r.lights(zc, z, byID, loc, stale, checkClock)
	}
	return nil
}

// markLive keeps a zone's open alerts of these kinds and measure from closing.
func (r *alertRun) markLive(zone int, measure string, kinds ...string) []alerts.OpenAlert {
	var out []alerts.OpenAlert
	for _, o := range r.open {
		if o.ZoneID != zone || o.Measure != measure {
			continue
		}
		for _, k := range kinds {
			if o.Kind == k {
				r.live[alertKey(o.Kind, o.ZoneID, o.Measure, o.Side)] = true
				out = append(out, alerts.OpenAlert{Key: alerts.Key{Kind: o.Kind, Measure: o.Measure, Side: o.Side}, NotifiedAt: o.Notified})
			}
		}
	}
	return out
}

// measure checks one of a zone's climate sensors.
func (r *alertRun) measure(cs ClimateSettings, zc ZoneClimate, z ZoneSetup, s ManagedSensor, measure string, sw time.Time, poll, stale time.Duration, checkDrift bool) {
	t := r.cfg.Timing
	open := r.markLive(zc.ZoneID, measure, alerts.KindSafety, alerts.KindStage, alerts.KindDrift)
	minWindow := 3 * poll
	back := t.Window
	if minWindow > back {
		back = minWindow
	}
	// Safety limits need their readings in a row even from a slow sensor.
	if need := time.Duration(t.SafetyReadings+1) * poll; need > back {
		back = need
	}
	if checkDrift && t.Drift > back {
		back = t.Drift
	}
	readings, err := recentReadings(r.e.DB, s.ID, s.Unit, measure, r.now.Add(-back-time.Minute))
	if err != nil {
		logger.Log.WithError(err).WithField("sensor", s.ID).Error("Failed to read sensor readings for alerts")
		return
	}
	in := alerts.Input{Measure: measure, Readings: readings, LastSwitch: sw, Now: r.now, Timing: t,
		MinWindow: minWindow, StaleAfter: stale, CheckDrift: checkDrift, Open: open}
	switch measure {
	case climate.Temp:
		in.Limits = cs.TempLimits
	case climate.RH:
		in.Limits = cs.RHLimits
	}
	if cs.byStage(measure) {
		in.Range = cs.target(measure, zc.Stage.Stage, zc.Phase, zc.LightsOn)
	}
	for _, c := range alerts.Evaluate(in) {
		r.apply(zc.ZoneID, z.Name, s.ID, s.Name, c, nil)
	}
}

// lights checks a zone's Light check sensor against its schedule.
func (r *alertRun) lights(zc ZoneClimate, z ZoneSetup, byID map[int]ManagedSensor, loc *time.Location, stale time.Duration, checkClock bool) {
	if z.LightSensorID == nil || !zc.Scheduled {
		return
	}
	s, ok := byID[*z.LightSensorID]
	if !ok || s.ZoneID != zc.ZoneID {
		return
	}
	lightOpen := r.markLive(zc.ZoneID, "", alerts.KindLight)
	clockOpen := r.markLive(zc.ZoneID, "", alerts.KindClock)
	above := 0.0
	if z.LightOnAbove != nil {
		above = *z.LightOnAbove
	}
	back := alerts.LightHold + time.Minute
	if checkClock {
		back = climate.ClockOffsetLookback
	}
	readings, err := recentReadings(r.e.DB, s.ID, s.Unit, "", r.now.Add(-back))
	if err != nil {
		logger.Log.WithError(err).WithField("sensor", s.ID).Error("Failed to read light sensor readings for alerts")
		return
	}
	n := len(readings)
	if n == 0 || r.now.Sub(readings[n-1].At) > stale {
		return // not reporting: hold
	}
	latest := readings[n-1].Value

	if checkClock {
		shift := climate.ClockOffset(readings, above, z.Schedule, zc.Stage.Stage, r.now, loc)
		switch {
		case shift != nil && len(clockOpen) == 0:
			side := alerts.SideLate
			if shift.Early {
				side = alerts.SideEarly
			}
			minutes := float64(shift.Minutes)
			times := z.Schedule[zc.Stage.Stage]
			detail := strings.Join([]string{shift.SeenOn.In(loc).Format("15:04"), shift.SeenOff.In(loc).Format("15:04"), times[0], times[1]}, "|")
			c := alerts.Change{Key: alerts.Key{Kind: alerts.KindClock, Side: side}, Action: alerts.Start, Value: latest, Limit: &minutes}
			r.apply(zc.ZoneID, z.Name, s.ID, s.Name, c, &alertStart{detail: detail})
			clockOpen = []alerts.OpenAlert{{Key: c.Key, NotifiedAt: r.now}}
		case shift == nil && len(clockOpen) > 0:
			for _, o := range clockOpen {
				r.apply(zc.ZoneID, z.Name, s.ID, s.Name, alerts.Change{Key: o.Key, Action: alerts.Close}, nil)
			}
			clockOpen = nil
		}
	}
	// A steady clock offset explains the twice-daily disagreement.
	if len(clockOpen) > 0 {
		for _, o := range lightOpen {
			r.apply(zc.ZoneID, z.Name, s.ID, s.Name, alerts.Change{Key: o.Key, Action: alerts.Close}, nil)
		}
		return
	}

	m, agree := climate.CheckLights(readings, above, z.Schedule, zc.Stage.Stage, r.now, loc, alerts.LightHold)
	switch {
	case m != nil:
		side := alerts.SideOff
		if m.SeenOn {
			side = alerts.SideOn
		}
		k := alerts.Key{Kind: alerts.KindLight, Side: side}
		found := false
		for _, o := range lightOpen {
			if o.Key != k {
				r.apply(zc.ZoneID, z.Name, s.ID, s.Name, alerts.Change{Key: o.Key, Action: alerts.Close}, nil)
				continue
			}
			found = true
			if every := r.cfg.Timing.Repeat(alerts.KindLight); every > 0 && r.now.Sub(o.NotifiedAt) >= every {
				r.apply(zc.ZoneID, z.Name, s.ID, s.Name, alerts.Change{Key: k, Action: alerts.Repeat, Value: latest}, nil)
			}
		}
		if !found {
			r.apply(zc.ZoneID, z.Name, s.ID, s.Name, alerts.Change{Key: k, Action: alerts.Start, Value: latest}, &alertStart{since: m.Since})
		}
	case agree:
		for _, o := range lightOpen {
			r.apply(zc.ZoneID, z.Name, s.ID, s.Name, alerts.Change{Key: o.Key, Action: alerts.End, Value: latest}, nil)
		}
	}
}

// alertStart carries what a light or clock alert adds when it starts.
type alertStart struct {
	detail string
	since  time.Time
}

// apply writes one change. Dry run lists alerts but never sends.
func (r *alertRun) apply(zone int, zoneName string, sensorID int, sensorName string, c alerts.Change, start *alertStart) {
	log := logger.Log.WithField("func", "AlertEngine.apply")
	db := r.e.DB
	email := r.cfg.Mode == alerts.ModeEmail
	value := any(nil)
	if c.Action != alerts.Close || c.Value != 0 {
		value = c.Value
	}
	var limit any
	if c.Limit != nil {
		limit = *c.Limit
	}
	var err error
	switch c.Action {
	case alerts.Start:
		pending, delivery := "", alerts.DeliveryDryRun
		if email {
			pending, delivery = alerts.NoticeFired, alerts.DeliveryPending
		}
		started := r.now
		detail := ""
		if start != nil {
			detail = start.detail
			if !start.since.IsZero() {
				started = start.since
			}
		}
		var sid any
		if sensorID != 0 {
			sid = sensorID
		}
		_, err = db.Exec(`INSERT INTO alert_event (kind, zone_id, zone_name, sensor_id, sensor_name, measure, side, value, limit_value, detail,
			started_at, notified_at, delivery, pending) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) ON CONFLICT DO NOTHING`,
			c.Kind, zone, zoneName, sid, sensorName, c.Measure, c.Side, value, limit, detail, dbTime(started), dbTime(r.now), delivery, pending)
	case alerts.Repeat:
		// A notice still waiting to go out stays as it is; dry run only keeps time.
		if email {
			_, err = db.Exec(`UPDATE alert_event SET value = $1, notified_at = $2, delivery = $3,
				pending = CASE WHEN pending = '' THEN $4 ELSE pending END
				WHERE kind = $5 AND zone_id = $6 AND measure = $7 AND side = $8 AND resolved_at IS NULL`,
				value, dbTime(r.now), alerts.DeliveryPending, alerts.NoticeRepeat, c.Kind, zone, c.Measure, c.Side)
		} else {
			_, err = db.Exec(`UPDATE alert_event SET value = $1, notified_at = $2
				WHERE kind = $3 AND zone_id = $4 AND measure = $5 AND side = $6 AND resolved_at IS NULL`,
				value, dbTime(r.now), c.Kind, zone, c.Measure, c.Side)
		}
	case alerts.End:
		// Back to normal only follows an alert that was really emailed; one that
		// never got out needs no second message.
		_, err = db.Exec(`UPDATE alert_event SET value = $1, resolved_at = $2, retry_at = NULL,
			pending = CASE WHEN emailed = 1 AND $3 = 1 THEN $4 ELSE '' END,
			delivery = CASE WHEN emailed = 1 AND $3 = 1 THEN $5 ELSE delivery END
			WHERE kind = $6 AND zone_id = $7 AND measure = $8 AND side = $9 AND resolved_at IS NULL`,
			value, dbTime(r.now), boolInt(email), alerts.NoticeResolved, alerts.DeliveryPending, c.Kind, zone, c.Measure, c.Side)
	case alerts.Close:
		_, err = db.Exec(`UPDATE alert_event SET resolved_at = $1, pending = '', retry_at = NULL,
			delivery = CASE WHEN delivery = $2 THEN '' ELSE delivery END
			WHERE kind = $3 AND zone_id = $4 AND measure = $5 AND side = $6 AND resolved_at IS NULL`,
			dbTime(r.now), alerts.DeliveryPending, c.Kind, zone, c.Measure, c.Side)
	}
	if err != nil {
		log.WithError(err).WithField("kind", c.Kind).Error("Failed to save an alert")
		return
	}
	r.changed = true
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RunSender sends what's due until ctx ends: when Check wakes it, and once a
// minute for retries and anything left from before a restart.
func (e *AlertEngine) RunSender(ctx context.Context) {
	tick := time.NewTicker(alertSendWait)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-tick.C:
		}
		if config.RestoreInProgress.Load() {
			continue
		}
		e.SendPending(ctx)
	}
}

// alertWords builds event text for email, in the language saved with the card.
func (e *AlertEngine) alertWords(lang string) alerts.Words {
	text := utils.TranslationService.GetTranslations(lang)
	return alerts.Words{Units: e.Store.Units(), Loc: settingsLocation(e.Store), T: func(key string) string {
		if s := text[key]; s != "" {
			return s
		}
		return key
	}}
}

// SendPending puts every notice that's due into one email.
func (e *AlertEngine) SendPending(ctx context.Context) {
	log := logger.Log.WithField("func", "AlertEngine.SendPending")
	now := e.Now()
	rows, err := e.DB.Query(`SELECT `+alertEventColumns+` FROM alert_event
		WHERE pending <> '' AND (retry_at IS NULL OR retry_at <= $1) ORDER BY id LIMIT $2`, dbTime(now), alertMaxPerMail)
	if err != nil {
		log.WithError(err).Error("Failed to read pending alerts")
		return
	}
	events, err := scanAlertEvents(rows)
	if err != nil {
		log.WithError(err).Error("Failed to read pending alerts")
		return
	}
	if len(events) == 0 {
		return
	}
	cfg, err := alerts.LoadClimateConfig(e.DB)
	if err != nil {
		log.WithError(err).Error("Failed to read the Climate alerts settings")
		return
	}
	mark := func(q string, args ...any) {
		ids := make([]string, len(events))
		for i, ev := range events {
			ids[i] = fmt.Sprint(ev.ID)
		}
		if _, err := e.DB.Exec(strings.Replace(q, "(IDS)", "("+strings.Join(ids, ",")+")", 1), args...); err != nil {
			log.WithError(err).Error("Failed to record what happened to an alert email")
		}
	}
	if cfg.Mode != alerts.ModeEmail {
		mark(`UPDATE alert_event SET pending = '', retry_at = NULL, delivery = $1 WHERE id IN (IDS)`, alerts.DeliveryDryRun)
		return
	}
	mail, err := alerts.LoadEmail(e.DB, e.DataDir)
	if err != nil {
		log.WithError(err).Error("Failed to read the email settings")
		return
	}
	if !mail.On || mail.PasswordLost || mail.Problem() != "" {
		mark(`UPDATE alert_event SET pending = '', retry_at = NULL, delivery = $1 WHERE id IN (IDS)`, alerts.DeliveryOff)
		return
	}
	link := ""
	if mail.Link != "" {
		link = mail.Link + "/settings#alerts"
	}
	if err := mail.Send(ctx, alerts.BuildMessage(e.alertWords(cfg.Language), events, link)); err != nil {
		log.WithError(err).WithField("host", mail.Host).Warn("Alert email failed; trying again in 5 minutes")
		mark(`UPDATE alert_event SET delivery = $1, delivery_error = $2, retry_at = $3 WHERE id IN (IDS)`,
			alerts.DeliveryFailed, err.Error(), dbTime(now.Add(alertRetryAfter)))
		return
	}
	mark(`UPDATE alert_event SET pending = '', retry_at = NULL, delivery = $1, delivery_error = '', emailed = 1 WHERE id IN (IDS)`, alerts.DeliverySent)
}
