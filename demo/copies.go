// Package demo keeps a private copy of the demo's database for each visitor
// of the public demo (demo mode, BUDMINDER_DEMO=1; see handlers/demo.go).
//
// The engine's own database is the base: the demo as built. Visitors read
// the base until their first change; then they get their own copy (VACUUM
// INTO, a few MB), used for everything they do until it has been idle for an
// hour. Nobody sees anyone else's changes, and reading alone (bots, a quick
// look) never makes a copy.
//
// The grow itself stays at the same point forever: once a day the base's
// dates (plant starts, stage changes, notebook days, photos, seed and smoke
// dates) move forward by the days that passed, so it is always "Day 76,
// watered yesterday". Only the calendar dates shown move along.
//
// Sensor readings stay live: every minute the base and each copy repeat each
// sensor's reading from a day earlier, as the demo build does. Only two days
// of per-minute readings are kept (charts use the hourly table beyond 24
// hours), which keeps copies small.
//
// Photos live in uploads/, shared by everyone, so the demo blocks every route
// that adds or deletes files (handlers/demo.go).
package demo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"isley/config"
	"isley/handlers"
	"isley/logger"
)

const (
	// DefaultMaxCopies caps how many visitor copies exist at once; past it,
	// the copy used longest ago is dropped.
	DefaultMaxCopies = 200
	// Idle is how long a copy lives without being used.
	Idle = time.Hour
	// CopiesPerIPPerHour caps how many new copies one address can start.
	CopiesPerIPPerHour = 10
	// keepRaw is how much per-minute sensor history is kept.
	keepRaw = 48 * time.Hour
)

// ErrBusy means this visitor started too many copies; try again later.
var ErrBusy = errors.New("demo: too many new visits from this address")

const layout = "2006-01-02 15:04:05"

type visitorCopy struct {
	db    *sql.DB
	store *config.Store
	path  string
	used  time.Time
}

// Copies hands each visitor their database and settings store.
type Copies struct {
	mu        sync.Mutex
	dir       string
	base      *sql.DB // the demo as built; visitors never write to it
	baseRead  *sql.DB // what visitors without a copy read (read-only when possible)
	baseStore *config.Store
	max       int
	copies    map[string]*visitorCopy
	datesOn   string                 // the local day the base's dates were last rolled to
	started   map[string][]time.Time // new copies per address, for CopiesPerIPPerHour
	now       func() time.Time
}

// New prepares the base (trims old per-minute readings and fills them up to
// now) and an empty folder for the copies. basePath is the base's file, for
// a read-only connection; "" (tests, in-memory databases) reads the base
// through its own connection.
func New(base *sql.DB, basePath string, store *config.Store, dir string, max int) (*Copies, error) {
	if max <= 0 {
		max = DefaultMaxCopies
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Copies{dir: dir, base: base, baseRead: base, baseStore: store, max: max,
		copies: map[string]*visitorCopy{}, started: map[string][]time.Time{}, now: time.Now}
	if basePath != "" {
		// The file: form is needed for mode=ro to take effect.
		abs, _ := filepath.Abs(basePath)
		ro, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro&_pragma=busy_timeout(5000)")
		if err == nil && ro.Ping() == nil && readOnly(ro) {
			m.baseRead = ro
		} else {
			logger.Log.Warn("Demo: couldn't open the base read-only; visitors without a copy read it directly")
		}
	}
	m.keepLive(base, m.now())
	m.roll(m.now())
	if _, err := base.Exec(rollupSQL); err != nil {
		logger.Log.WithError(err).Warn("Demo: hourly rollup failed")
	}
	if _, err := base.Exec(`VACUUM`); err != nil {
		logger.Log.WithError(err).Warn("Demo: couldn't compact the base")
	}
	return m, nil
}

// readOnly checks that db refuses a write (one that would change nothing).
func readOnly(db *sql.DB) bool {
	_, err := db.Exec(`UPDATE settings SET name = name WHERE 1 = 0`)
	return err != nil
}

// Start keeps the readings live and drops idle copies until ctx ends.
func (m *Copies) Start(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			m.closeAll()
			return
		case <-t.C:
			n++
			m.Tick(m.now(), n%10 == 0)
		}
	}
}

// Tick does one minute's work: live readings for the base and every copy,
// idle copies dropped, and (rollup) the hourly table refreshed.
func (m *Copies) Tick(now time.Time, rollup bool) {
	if now.Format("2006-01-02") != m.datesOn {
		m.roll(now)
	}
	m.mu.Lock()
	dbs := []*sql.DB{m.base}
	for id, cp := range m.copies {
		if now.Sub(cp.used) > Idle {
			m.drop(id)
			continue
		}
		dbs = append(dbs, cp.db)
	}
	m.mu.Unlock()
	for _, db := range dbs {
		m.keepLive(db, now)
		if rollup {
			if _, err := db.Exec(rollupSQL); err != nil {
				logger.Log.WithError(err).Warn("Demo: hourly rollup failed")
			}
		}
	}
}

// rolledColumns are the base's dates that describe the grow. Record-keeping
// timestamps (created_at and the like) stay as they are, and sensor readings
// are kept live by keepLive instead.
var rolledColumns = []struct{ table, col string }{
	{"plant", "start_dt"},
	{"plant_status_log", "date"},
	{"plant_activity", "date"},
	{"plant_measurements", "date"},
	{"plant_day_entry", "entry_date"},
	{"plant_images", "image_date"},
	{"strain", "seeds_added_on"},
	{"smoke_report", "smoked_on"},
	{"smoke_report", "harvested_on"},
}

// datesOnSetting records the local day the demo's dates are set to.
const datesOnSetting = "demo_dates_on"

// RollDates moves every date in rolledColumns forward by the days between
// the day the data was last set to and today (local time), keeping each
// value's format, and records today. It returns how many days it moved.
// A snapshot that doesn't record its day yet is taken to be from the day it
// was built (when its notebook rows were written).
func RollDates(db *sql.DB, today time.Time) (int, error) {
	var on sql.NullString
	_ = db.QueryRow(`SELECT value FROM settings WHERE name = $1`, datesOnSetting).Scan(&on)
	if !on.Valid || on.String == "" {
		_ = db.QueryRow(`SELECT date(MAX(created_at), 'localtime') FROM plant_day_entry`).Scan(&on)
	}
	day := today.Format("2006-01-02")
	from, err := time.ParseInLocation("2006-01-02", on.String, time.Local)
	if err != nil {
		return 0, saveDatesOn(db, day) // nothing to go by: start counting today
	}
	to, _ := time.ParseInLocation("2006-01-02", day, time.Local)
	days := int(to.Sub(from).Hours()/24 + 0.5)
	if days <= 0 {
		return 0, saveDatesOn(db, day)
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	// Two steps, far ahead and then back: one step would trip unique dates
	// (one notebook entry per plant per day) whenever a row moves onto a
	// date its neighbor hasn't left yet.
	const away = 40000 // days, about 110 years
	for _, shift := range []string{fmt.Sprintf("+%d days", days+away), fmt.Sprintf("-%d days", away)} {
		for _, rc := range rolledColumns {
			// Table and column names are fixed above, never visitor input.
			q := fmt.Sprintf(`UPDATE %[1]s SET %[2]s = CASE
					WHEN length(%[2]s) = 10 THEN date(%[2]s, $1)
					WHEN instr(%[2]s, 'T') > 0 THEN strftime('%%Y-%%m-%%dT%%H:%%M:%%S', %[2]s, $1)
					ELSE datetime(%[2]s, $1) END
				WHERE %[2]s IS NOT NULL AND %[2]s != ''`, rc.table, rc.col)
			if _, err := tx.Exec(q, shift); err != nil {
				return 0, fmt.Errorf("demo: roll %s.%s: %w", rc.table, rc.col, err)
			}
		}
	}
	if err := saveDatesOn(tx, day); err != nil {
		return 0, err
	}
	return days, tx.Commit()
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func saveDatesOn(db execer, day string) error {
	res, err := db.Exec(`UPDATE settings SET value = $1 WHERE name = $2`, day, datesOnSetting)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = db.Exec(`INSERT INTO settings (name, value) VALUES ($1, $2)`, datesOnSetting, day)
	}
	return err
}

// roll rolls the base's dates to now's day and reloads what the base's
// store caches from them (the strains' seed dates).
func (m *Copies) roll(now time.Time) {
	days, err := RollDates(m.base, now.In(time.Local))
	if err != nil {
		logger.Log.WithError(err).Warn("Demo: couldn't move the grow's dates forward")
		return
	}
	m.datesOn = now.In(time.Local).Format("2006-01-02")
	if days > 0 {
		handlers.LoadSettings(m.base, m.baseStore)
		logger.Log.WithField("days", days).Info("Demo: moved the grow's dates forward")
	}
}

// rollupSQL refreshes the hourly table for the last day, like the watcher's
// SQLite rollup (watcher/rollup.go), which demo mode doesn't run. (Importing
// the watcher here would loop through the test helpers.)
const rollupSQL = `INSERT OR REPLACE INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
	SELECT sensor_id, strftime('%Y-%m-%d %H:00:00', create_dt), MIN(value), MAX(value), AVG(value), COUNT(*)
	FROM sensor_data WHERE create_dt >= strftime('%Y-%m-%d %H:00:00', 'now', '-25 hours')
	GROUP BY sensor_id, strftime('%Y-%m-%d %H:00:00', create_dt)`

// keepLive repeats each sensor's readings from a day earlier up to now, and
// trims per-minute readings older than keepRaw.
func keepLive(db *sql.DB, now time.Time) error {
	now = now.UTC().Truncate(time.Minute)
	var lastS sql.NullString
	if err := db.QueryRow(`SELECT MAX(create_dt) FROM sensor_data`).Scan(&lastS); err != nil || !lastS.Valid {
		return err
	}
	last, err := time.Parse(layout, lastS.String[:min(len(lastS.String), len(layout))])
	if err != nil {
		return err
	}
	for last.Before(now) {
		upto := last.Add(24 * time.Hour)
		if upto.After(now) {
			upto = now
		}
		_, err := db.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt)
			SELECT sensor_id, value, strftime('%Y-%m-%d %H:%M:%S', create_dt, '+1 day') FROM sensor_data
			WHERE create_dt > $1 AND create_dt <= $2`,
			last.Add(-24*time.Hour).Format(layout), upto.Add(-24*time.Hour).Format(layout))
		if err != nil {
			return err
		}
		last = upto
	}
	_, err = db.Exec(`DELETE FROM sensor_data WHERE create_dt < $1`, now.Add(-keepRaw).Format(layout))
	return err
}

func (m *Copies) keepLive(db *sql.DB, now time.Time) {
	if err := keepLive(db, now); err != nil {
		logger.Log.WithError(err).Warn("Demo: couldn't bring sensor readings up to date")
	}
}

// addressKey is what the per-address limit counts: an IPv4 address, or
// an IPv6 address's /64, which one home or phone can rotate through
// freely.
func addressKey(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() != nil {
		return ip
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// ForRequest returns the database and store for this visitor. id is the
// copy named in their session ("" for none); write says the request changes
// something, which needs a copy. It returns the id to keep in the session.
func (m *Copies) ForRequest(id, ip string, write bool) (string, *sql.DB, *config.Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if cp := m.copies[id]; cp != nil {
		cp.used = now
		return id, cp.db, cp.store, nil
	}
	if !write {
		return "", m.baseRead, m.baseStore, nil
	}

	ip = addressKey(ip)
	recent := m.started[ip][:0]
	for _, t := range m.started[ip] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= CopiesPerIPPerHour {
		m.started[ip] = recent
		return "", nil, nil, ErrBusy
	}
	m.started[ip] = append(recent, now)

	for len(m.copies) >= m.max {
		oldest := ""
		for k, cp := range m.copies {
			if oldest == "" || cp.used.Before(m.copies[oldest].used) {
				oldest = k
			}
		}
		m.drop(oldest)
	}

	cp, newID, err := m.make()
	if err != nil {
		return "", nil, nil, err
	}
	cp.used = now
	m.copies[newID] = cp
	return newID, cp.db, cp.store, nil
}

func (m *Copies) make() (*visitorCopy, string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, "", err
	}
	id := hex.EncodeToString(b)
	path := filepath.Join(m.dir, id+".db")
	// The id is ours (hex), never visitor input.
	if _, err := m.base.Exec(fmt.Sprintf(`VACUUM INTO '%s'`, filepath.ToSlash(path))); err != nil {
		return nil, "", fmt.Errorf("demo: copy the base: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		_ = os.Remove(path)
		return nil, "", err
	}
	store := config.NewStore()
	handlers.LoadSettings(db, store)
	handlers.SetupDemo(store)
	return &visitorCopy{db: db, store: store, path: path}, id, nil
}

// drop closes and deletes one copy; m.mu must be held.
func (m *Copies) drop(id string) {
	cp := m.copies[id]
	if cp == nil {
		return
	}
	delete(m.copies, id)
	_ = cp.db.Close()
	for _, p := range []string{cp.path, cp.path + "-wal", cp.path + "-shm"} {
		_ = os.Remove(p)
	}
}

// Close drops every copy.
func (m *Copies) Close() { m.closeAll() }

func (m *Copies) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.copies {
		m.drop(id)
	}
}

// Count is how many visitor copies exist now.
func (m *Copies) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.copies)
}
