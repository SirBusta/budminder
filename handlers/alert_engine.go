package handlers

// The background check behind alerts: after each sensor poll it compares each
// zone's chosen climate sensors and light check with their limits (rules in
// package alerts and climate), and keeps alert_event up to date. It always
// runs: with notifications off, alerts are still listed. Email goes out from its own goroutine (RunSender), so a slow mail
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
	alertClockEvery = time.Hour // how often the clock offset is checked (days of readings)
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

	lastClock, lastPrune time.Time // Check's goroutine only
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
	started_at, notified_at, resolved_at, delivery, delivery_error, pending, emailed, cleared`

func scanAlertEvents(rows *sql.Rows) ([]alerts.Event, error) {
	defer rows.Close()
	var out []alerts.Event
	for rows.Next() {
		var e alerts.Event
		var sensor sql.NullInt64
		var value, limit sql.NullFloat64
		var resolved sql.NullTime
		var emailed, cleared int
		if err := rows.Scan(&e.ID, &e.Kind, &e.ZoneID, &e.Zone, &sensor, &e.Sensor, &e.Measure, &e.Side, &value, &limit, &e.Detail,
			&e.Started, &e.Notified, &resolved, &e.Delivery, &e.DeliveryError, &e.Pending, &emailed, &cleared); err != nil {
			return nil, err
		}
		e.SensorID, e.Value, e.Limit = sqlIntPtr(sensor), nullPtr(value), nullPtr(limit)
		if resolved.Valid {
			t := resolved.Time
			e.Resolved = &t
		}
		e.Emailed, e.Cleared = emailed != 0, cleared != 0
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

// RecentAlerts lists alerts for Settings > Alerts, cleared ones or the others:
// open ones first, then newest.
func RecentAlerts(db *sql.DB, cleared bool, limit int) ([]alerts.Event, error) {
	rows, err := db.Query(`SELECT `+alertEventColumns+` FROM alert_event WHERE cleared = $1
		ORDER BY CASE WHEN resolved_at IS NULL THEN 0 ELSE 1 END, started_at DESC, id DESC LIMIT $2`, boolInt(cleared), limit)
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
	mailOn  bool   // notifications are on and the Email settings are complete
	notSent string // what an alert records when it isn't emailed
	now     time.Time
	open    []alerts.Event
	live    map[string]bool    // open alerts that still apply, by alertKey
	latest  map[string]float64 // the latest reading behind each, by alertKey
	watched map[int]bool       // sensors that climate, lights or sensor alerts read
	changed bool
}

// notSentAs is what an alert records when it isn't emailed: paused when email
// was set up and then turned off, off otherwise.
func notSentAs(mail alerts.Email) string {
	if !mail.On && mail.Host != "" {
		return alerts.DeliveryPaused
	}
	return alerts.DeliveryOff
}

func alertKey(kind string, zone int, measure, side string) string {
	return fmt.Sprintf("%s|%d|%s|%s", kind, zone, measure, side)
}

// Check is one pass over every zone. Errors are logged; the next poll tries again.
func (e *AlertEngine) Check(_ context.Context) {
	log := logger.Log.WithField("func", "AlertEngine.Check")
	open, err := loadOpenAlerts(e.DB)
	if err != nil {
		log.WithError(err).Error("Failed to read open alerts")
		return
	}
	run := &alertRun{e: e, now: e.Now(), open: open, live: map[string]bool{}, latest: map[string]float64{}, watched: map[int]bool{}}
	mail, err := alerts.LoadEmail(e.DB, e.DataDir)
	if err != nil {
		log.WithError(err).Error("Failed to read the email settings")
		return
	}
	run.mailOn = mail.On && !mail.PasswordLost && mail.Problem() == ""
	run.notSent = notSentAs(mail)
	if err := run.zones(); err != nil {
		log.WithError(err).Error("Failed to check climate alerts")
		return
	}
	if err := run.rules(); err != nil {
		log.WithError(err).Error("Failed to check sensor alerts")
		return
	}
	run.offline()
	run.reminders()
	// Whatever wasn't reached no longer applies: the sensor was unassigned, the
	// zone deleted, its plants gone, or the limit removed.
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
	checkClock := r.now.Sub(e.lastClock) >= alertClockEvery
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
			r.watched[sid] = true
			r.measure(cs, zc, z, byID[sid], measure, sw, pollEvery, stale)
		}
		r.lights(zc, z, byID, loc, stale, checkClock)
	}
	return nil
}

// rules checks every sensor alert that's on. One whose sensor is gone isn't
// reached, so an open alert of its closes.
func (r *alertRun) rules() error {
	e := r.e
	rules, err := loadAlertRules(e.DB)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	poll := e.Store.PollingInterval()
	byID := map[int]ManagedSensor{}
	for _, s := range ListManagedSensors(e.DB, poll) {
		byID[s.ID] = s
	}
	pollEvery := time.Duration(poll) * time.Second
	stale := staleAfter(poll)
	for _, rule := range rules {
		s, ok := byID[rule.SensorID]
		if !ok || !rule.Enabled {
			continue
		}
		r.watched[s.ID] = true
		minWindow := 2 * pollEvery
		readings, err := recentReadings(e.DB, s.ID, s.Unit, "", r.now.Add(-max(alerts.Settle, minWindow)-time.Minute))
		if err != nil {
			logger.Log.WithError(err).WithField("sensor", s.ID).Error("Failed to read sensor readings for alerts")
			continue
		}
		measure := alerts.RuleMeasure(rule.ID)
		in := alerts.RuleInput{Rule: rule, Readings: readings, Now: r.now, MinWindow: minWindow, StaleAfter: stale,
			Open: r.markLive(0, measure, alerts.KindSensor)}
		r.noteLatest(alerts.KindSensor, 0, measure, readings)
		for _, c := range alerts.EvaluateRule(in) {
			r.apply(0, s.Zone, s.ID, s.Name, c, &alertStart{detail: s.Unit})
		}
	}
	return nil
}

// noteLatest remembers the latest reading for a reminder about these alerts.
func (r *alertRun) noteLatest(kind string, zone int, measure string, readings []climate.Reading) {
	if len(readings) == 0 {
		return
	}
	for _, o := range r.open {
		if o.Kind == kind && o.ZoneID == zone && o.Measure == measure {
			r.latest[alertKey(o.Kind, o.ZoneID, o.Measure, o.Side)] = readings[len(readings)-1].Value
		}
	}
}

// offline opens one alert per zone for the watched sensors that stopped sending
// readings, and ends it once all of them have sent new readings for Hold. A sensor
// that never sent a reading isn't offline: it hasn't started. Calculated VPD
// follows the sensors behind it, so it isn't named on its own.
func (r *alertRun) offline() {
	e := r.e
	groups := map[int][]ManagedSensor{}
	names := map[int]string{}
	for _, s := range ListManagedSensors(e.DB, e.Store.PollingInterval()) {
		if r.watched[s.ID] && s.Source != "derived" && s.LastAt != 0 {
			groups[s.ZoneID] = append(groups[s.ZoneID], s)
			names[s.ZoneID] = s.Zone
		}
	}
	for zone, sensors := range groups {
		open := r.markLive(zone, "", alerts.KindOffline)
		var since time.Time
		for _, o := range r.open {
			if o.Kind == alerts.KindOffline && o.ZoneID == zone {
				since = o.Started
			}
		}
		var stopped []string
		back := true
		for _, s := range sensors {
			if r.now.Sub(time.UnixMilli(s.LastAt)) >= alerts.OfflineAfter {
				stopped = append(stopped, s.Name)
				continue
			}
			if len(open) > 0 {
				back = back && r.reportingSince(s, since)
			}
		}
		k := alerts.Key{Kind: alerts.KindOffline}
		switch {
		case len(stopped) > 0 && len(open) == 0:
			r.apply(zone, names[zone], 0, "", alerts.Change{Key: k, Action: alerts.Start}, &alertStart{detail: strings.Join(stopped, ", ")})
		case len(stopped) > 0:
			if _, err := e.DB.Exec(`UPDATE alert_event SET detail = $1 WHERE kind = $2 AND zone_id = $3 AND resolved_at IS NULL`,
				strings.Join(stopped, ", "), alerts.KindOffline, zone); err != nil {
				logger.Log.WithError(err).Error("Failed to update an offline alert")
			}
		case len(open) > 0 && back:
			r.apply(zone, names[zone], 0, "", alerts.Change{Key: k, Action: alerts.End}, nil)
		}
	}
}

// reportingSince says a sensor has sent readings for Hold since the offline
// alert started. Readings from before the gap don't count.
func (r *alertRun) reportingSince(s ManagedSensor, started time.Time) bool {
	readings, err := recentReadings(r.e.DB, s.ID, s.Unit, "", started)
	return err == nil && len(readings) > 0 && !readings[0].At.After(r.now.Add(-alerts.Hold))
}

// reminders sends every open alert again once any of them has gone Remind
// without a notification, so they arrive together in one email. Clock notices
// are one notice only.
func (r *alertRun) reminders() {
	due := false
	for _, o := range r.open {
		if o.Kind != alerts.KindClock && r.live[alertKey(o.Kind, o.ZoneID, o.Measure, o.Side)] && r.now.Sub(o.Notified) >= alerts.Remind {
			due = true
		}
	}
	if !due {
		return
	}
	for _, o := range r.open {
		key := alertKey(o.Kind, o.ZoneID, o.Measure, o.Side)
		if o.Kind == alerts.KindClock || !r.live[key] {
			continue
		}
		c := alerts.Change{Key: alerts.Key{Kind: o.Kind, Measure: o.Measure, Side: o.Side}, Action: alerts.Repeat}
		var value any
		if v, ok := r.latest[key]; ok {
			value = v
		}
		r.applyRepeat(o.ZoneID, c, value)
	}
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
func (r *alertRun) measure(cs ClimateSettings, zc ZoneClimate, z ZoneSetup, s ManagedSensor, measure string, sw time.Time, poll, stale time.Duration) {
	open := r.markLive(zc.ZoneID, measure, alerts.KindStage)
	minWindow := 3 * poll
	readings, err := recentReadings(r.e.DB, s.ID, s.Unit, measure, r.now.Add(-max(alerts.Settle, minWindow)-time.Minute))
	if err != nil {
		logger.Log.WithError(err).WithField("sensor", s.ID).Error("Failed to read sensor readings for alerts")
		return
	}
	r.noteLatest(alerts.KindStage, zc.ZoneID, measure, readings)
	in := alerts.Input{Measure: measure, Readings: readings, Range: cs.rangeNow(measure, zc.Stage.Stage, zc.Phase, zc.LightsOn),
		LastSwitch: sw, Now: r.now, MinWindow: minWindow, StaleAfter: stale, Open: open}
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
	r.watched[s.ID] = true
	lightOpen := r.markLive(zc.ZoneID, "", alerts.KindLight)
	clockOpen := r.markLive(zc.ZoneID, "", alerts.KindClock)
	above := 0.0
	if z.LightOnAbove != nil {
		above = *z.LightOnAbove
	}
	back := alerts.LightHold + time.Minute
	if len(lightOpen) > 0 {
		back = alerts.Settle + time.Minute
	}
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
	r.noteLatest(alerts.KindLight, zc.ZoneID, "", readings)

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
		}
		if !found {
			since := m.Since
			// Read back to the switch: the lights may have followed it and failed later.
			if longer, err := recentReadings(r.e.DB, s.ID, s.Unit, "", since); err == nil {
				if full, _ := climate.CheckLights(longer, above, z.Schedule, zc.Stage.Stage, r.now, loc, alerts.LightHold); full != nil && full.Since.After(since) {
					since = full.Since
				}
			}
			r.apply(zc.ZoneID, z.Name, s.ID, s.Name, alerts.Change{Key: k, Action: alerts.Start, Value: latest}, &alertStart{since: since})
		}
	case agree && climate.LightsAgreed(readings, above, z.Schedule, zc.Stage.Stage, r.now, loc, alerts.Settle, alerts.LightHold):
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

// forget takes an alert out of r.open once it has ended in this run.
func (r *alertRun) forget(c alerts.Change, zone int) {
	for i, o := range r.open {
		if o.Kind == c.Kind && o.ZoneID == zone && o.Measure == c.Measure && o.Side == c.Side {
			r.open = append(r.open[:i:i], r.open[i+1:]...)
			return
		}
	}
}

// climateStillOpen says another of a zone's climate alerts is still open, so
// it isn't all clear yet: the zone gets one all-clear, when the last one ends.
func (r *alertRun) climateStillOpen(zone int) bool {
	for _, o := range r.open {
		if o.Kind == alerts.KindStage && o.ZoneID == zone {
			return true
		}
	}
	return false
}

// applyRepeat sends an alert again and records the latest reading; value is nil
// when there isn't one. A notice still waiting to go out stays as it is, and
// with notifications off it only keeps time.
func (r *alertRun) applyRepeat(zone int, c alerts.Change, value any) {
	var err error
	if r.mailOn {
		_, err = r.e.DB.Exec(`UPDATE alert_event SET value = COALESCE($1, value), notified_at = $2, delivery = $3,
			pending = CASE WHEN pending = '' THEN $4 ELSE pending END
			WHERE kind = $5 AND zone_id = $6 AND measure = $7 AND side = $8 AND resolved_at IS NULL`,
			value, dbTime(r.now), alerts.DeliveryPending, alerts.NoticeRepeat, c.Kind, zone, c.Measure, c.Side)
	} else {
		_, err = r.e.DB.Exec(`UPDATE alert_event SET value = COALESCE($1, value), notified_at = $2
			WHERE kind = $3 AND zone_id = $4 AND measure = $5 AND side = $6 AND resolved_at IS NULL`,
			value, dbTime(r.now), c.Kind, zone, c.Measure, c.Side)
	}
	if err != nil {
		logger.Log.WithError(err).WithField("kind", c.Kind).Error("Failed to save an alert")
		return
	}
	r.changed = true
}

// apply writes one change. With notifications off alerts are listed but never sent.
func (r *alertRun) apply(zone int, zoneName string, sensorID int, sensorName string, c alerts.Change, start *alertStart) {
	log := logger.Log.WithField("func", "AlertEngine.apply")
	db := r.e.DB
	value := any(nil)
	if c.Action != alerts.Close && c.Kind != alerts.KindOffline {
		value = c.Value
	}
	var limit any
	if c.Limit != nil {
		limit = *c.Limit
	}
	var err error
	switch c.Action {
	case alerts.Start:
		pending, delivery := "", r.notSent
		if r.mailOn {
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
	case alerts.End:
		r.forget(c, zone)
		// The all clear only follows an alert that was really emailed; one that
		// never got out needs no second message.
		notify := r.mailOn && !(c.Kind == alerts.KindStage && r.climateStillOpen(zone))
		_, err = db.Exec(`UPDATE alert_event SET value = COALESCE($1, value), resolved_at = $2, retry_at = NULL,
			pending = CASE WHEN emailed = 1 AND $3 = 1 THEN $4 ELSE '' END,
			delivery = CASE WHEN emailed = 1 AND $3 = 1 THEN $5 ELSE delivery END
			WHERE kind = $6 AND zone_id = $7 AND measure = $8 AND side = $9 AND resolved_at IS NULL`,
			value, dbTime(r.now), boolInt(notify), alerts.NoticeResolved, alerts.DeliveryPending, c.Kind, zone, c.Measure, c.Side)
	case alerts.Close:
		r.forget(c, zone)
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

// alertWords builds event text for email, in the language saved with the Email settings.
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
	lang, err := alerts.LoadLanguage(e.DB)
	if err != nil {
		log.WithError(err).Error("Failed to read the language for alert emails")
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
	mail, err := alerts.LoadEmail(e.DB, e.DataDir)
	if err != nil {
		log.WithError(err).Error("Failed to read the email settings")
		return
	}
	if !mail.On || mail.PasswordLost || mail.Problem() != "" {
		mark(`UPDATE alert_event SET pending = '', retry_at = NULL, delivery = $1 WHERE id IN (IDS)`, notSentAs(mail))
		return
	}
	link := ""
	if mail.Link != "" {
		link = mail.Link + "/settings#alerts"
	}
	if err := mail.Send(ctx, alerts.BuildMessage(e.alertWords(lang), events, link)); err != nil {
		log.WithError(err).WithField("host", mail.Host).Warn("Alert email failed; trying again in 5 minutes")
		mark(`UPDATE alert_event SET delivery = $1, delivery_error = $2, retry_at = $3 WHERE id IN (IDS)`,
			alerts.DeliveryFailed, err.Error(), dbTime(now.Add(alertRetryAfter)))
		return
	}
	mark(`UPDATE alert_event SET pending = '', retry_at = NULL, delivery = $1, delivery_error = '', emailed = 1 WHERE id IN (IDS)`, alerts.DeliverySent)
}
