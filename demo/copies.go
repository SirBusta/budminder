// Package demo keeps a private copy of the demo's database for each visitor
// of the public demo (demo mode, BUDMINDER_DEMO=1; see handlers/demo.go).
//
// The engine's own database is the base: the demo as built. Visitors read
// the base until their first change; then they get their own copy (VACUUM
// INTO, a few MB), used for everything they do until it has been idle for an
// hour. Nobody sees anyone else's changes, and reading alone (bots, a quick
// look) never makes a copy.
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

// rollupSQL refreshes the hourly table for the last day, like the watcher's
// SQLite rollup (watcher/rollup.go), which demo mode doesn't run. (Importing
// the watcher here would loop through the test helpers.)
const rollupSQL = `INSERT OR REPLACE INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
	SELECT sensor_id, strftime('%Y-%m-%d %H:00:00', create_dt), MIN(value), MAX(value), AVG(value), COUNT(*)
	FROM sensor_data WHERE create_dt > datetime('now', '-25 hours')
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
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
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
