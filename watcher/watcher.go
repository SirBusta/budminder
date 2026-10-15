// Package watcher polls AC Infinity, EcoWitt and Home Assistant at a
// configurable interval, persists readings into sensor_data, prunes
// rows past their retention window, and refreshes the hourly rollup
// table.
//
// The polling loop is dependency-injected via the Watcher struct so
// tests can substitute the HTTP client, the clock, and the config
// getters without touching package globals. See docs/TEST_PLAN.md
// Phase 2 for the rationale.
package watcher

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"isley/config"
	"isley/homeassistant"
	"isley/logger"
	"isley/model"
	"isley/model/types"
	"isley/utils"
)

const (
	// httpClientTimeout is the default timeout for outbound sensor API requests.
	httpClientTimeout = 10 * time.Second
	// pruneInterval is how often the sensor data pruner runs inside Run.
	pruneInterval = 24 * time.Hour
	// rollupInterval is how often hourly rollups are refreshed inside Run.
	rollupInterval = 10 * time.Minute
	// maxSensorResponseBytes caps the size of a single sensor API response
	// body. The wall-clock timeout already bounds the request, but a
	// slow-drip server (notably user-supplied EcoWitt LAN addresses)
	// could otherwise stream hundreds of MB inside that window.
	maxSensorResponseBytes = 4 * 1024 * 1024

	// defaultACIBaseURL is the production AC Infinity API host. Tests
	// override this on the Watcher struct to point at httptest fakes.
	defaultACIBaseURL = "https://www.acinfinityserver.com"
)

// HTTPDoer is the minimum surface from net/http needed by the watcher.
// *http.Client implements it directly. Tests can pass a stub that
// records request URLs and returns canned responses without standing
// up an httptest server, though most tests prefer httptest because the
// real client + fake server combination matches production behavior.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Watcher owns the dependencies the polling loop needs. Construct it
// once at process startup via New, or assemble one by hand in tests.
//
// All getter functions (PollingInterval, RestoreInProgress, etc.) are
// invoked on every iteration so a Watcher built from the per-engine
// *config.Store stays current with runtime settings changes.
type Watcher struct {
	DB         *sql.DB
	HTTP       HTTPDoer
	ACIBaseURL string
	Logger     *logrus.Logger

	Now               func() time.Time
	PollingInterval   func() time.Duration
	RestoreInProgress func() bool
	ACIEnabled        func() bool
	ACIToken          func() string
	ECEnabled         func() bool
	ECDevices         func() []string
	HAEnabled         func() bool
	HAURL             func() string
	HAToken           func() string
	// HAHTTP is the client for Home Assistant (nil = its no-redirect default).
	HAHTTP          homeassistant.Doer
	SensorRetention func() int
	// ReportHealth records how each integration's poll went (kind "" = it
	// worked), for the status shown in Manage sensors and on Today. Optional.
	ReportHealth func(source, kind, detail string)
	// ACIRelogin signs in to AC Infinity again with the saved password
	// ("Stay signed in") when it refuses the token. attempted is false when
	// no password is saved. Optional; main wires handlers.NewACIRelogin.
	ACIRelogin func(ctx context.Context) (attempted bool, err error)

	lastACIRelogin time.Time // Run's goroutine only
}

// aciReloginEvery limits automatic AC Infinity sign-ins to one an hour, so a
// changed password doesn't hammer AC Infinity with failing sign-ins.
const aciReloginEvery = time.Hour

// pollACIStayingSignedIn polls AC Infinity; if it refuses the token and a
// password is saved, it signs in again and polls once more.
func (w *Watcher) pollACIStayingSignedIn(ctx context.Context, token string) error {
	err := w.PollACI(ctx, token)
	var pe *PollError
	if !errors.As(err, &pe) || pe.Kind != "auth" || w.ACIRelogin == nil {
		return err
	}
	if !w.lastACIRelogin.IsZero() && w.Now().Sub(w.lastACIRelogin) < aciReloginEvery {
		return err
	}
	w.lastACIRelogin = w.Now()
	attempted, rerr := w.ACIRelogin(ctx)
	switch {
	case !attempted:
		return err
	case rerr != nil:
		w.Logger.WithError(rerr).Warn("Automatic AC Infinity sign-in failed; sign in again in Settings")
		return &PollError{Kind: "auth", Detail: "automatic sign-in failed: " + rerr.Error(), Err: rerr}
	}
	return w.PollACI(ctx, w.ACIToken())
}

// aciSuccessCode is what AC Infinity's API returns in "code" when it
// accepted the request.
const aciSuccessCode = 200

// aciCO2SensorType is AC Infinity's CO2 sensor. It reports plain ppm; the
// other sensors report x100 (7260 = 72.60 °F). Migration 036 rescaled the
// history recorded before 1.0.1, which divided CO2 by 100 too.
const aciCO2SensorType = 11

// PollError says why a poll failed: Kind is "auth" (the service refused the
// sign-in or token), "unreachable" or "error"; Detail is the service's own
// message when it gave one.
type PollError struct {
	Kind, Detail string
	Err          error
}

func (e *PollError) Error() string {
	if e.Detail != "" {
		return e.Kind + ": " + e.Detail
	}
	if e.Err != nil {
		return e.Kind + ": " + e.Err.Error()
	}
	return e.Kind
}

func (e *PollError) Unwrap() error { return e.Err }

// report passes a poll's outcome to ReportHealth.
func (w *Watcher) report(source string, err error) {
	if w.ReportHealth == nil {
		return
	}
	if err == nil {
		w.ReportHealth(source, "", "")
		return
	}
	var pe *PollError
	if errors.As(err, &pe) {
		w.ReportHealth(source, pe.Kind, pe.Detail)
		return
	}
	w.ReportHealth(source, "error", "")
}

// New returns a Watcher wired to the supplied per-engine *config.Store
// and a default HTTP client. Production code (main.go) calls this with
// the engine's Store; tests typically construct a Watcher literal so
// each field can be controlled.
func New(db *sql.DB, store *config.Store) *Watcher {
	return &Watcher{
		DB:         db,
		HTTP:       &http.Client{Timeout: httpClientTimeout},
		ACIBaseURL: defaultACIBaseURL,
		Logger:     logger.Log,

		Now:               time.Now,
		PollingInterval:   func() time.Duration { return time.Duration(store.PollingInterval()) * time.Second },
		RestoreInProgress: config.RestoreInProgress.Load,
		ACIEnabled:        func() bool { return store.ACIEnabled() == 1 },
		ACIToken:          store.ACIToken,
		ECEnabled:         func() bool { return store.ECEnabled() == 1 },
		ECDevices:         store.ECDevices,
		HAEnabled:         func() bool { return store.HAEnabled() == 1 },
		HAURL:             store.HAURL,
		HAToken:           store.HAToken,
		SensorRetention:   store.SensorRetention,
		ReportHealth: func(source, kind, detail string) {
			store.ReportIntegration(source, kind, detail, time.Now())
		},
	}
}

// Run is the main polling loop. It blocks until ctx is cancelled and
// returns after the in-flight iteration completes — callers should
// run it in its own goroutine and use a sync.WaitGroup or similar to
// wait for shutdown.
//
// Each iteration:
//  1. If a backup restore is in progress, do nothing this cycle.
//  2. Otherwise poll AC Infinity and EcoWitt according to enabled flags.
//  3. Run prune and rollup if their tickers have fired.
//  4. Sleep for PollingInterval, or return early if ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	w.Logger.Info("Started Sensor Watcher")

	pruneTicker := time.NewTicker(pruneInterval)
	defer pruneTicker.Stop()

	rollupTicker := time.NewTicker(rollupInterval)
	defer rollupTicker.Stop()

	// Run an initial rollup at startup to backfill if needed.
	if err := w.RefreshHourlyRollups(); err != nil {
		w.Logger.WithError(err).Error("Initial hourly rollup failed")
	}

	for {
		if w.RestoreInProgress() {
			w.Logger.Debug("Backup restore in progress, skipping sensor poll")
		} else {
			if w.ACIEnabled() {
				if token := w.ACIToken(); token != "" {
					w.report("acinfinity", w.pollACIStayingSignedIn(ctx, token))
				}
			}
			if w.ECEnabled() {
				if servers := w.ECDevices(); len(servers) > 0 {
					var firstErr error
					for _, ecServer := range servers {
						if err := w.PollEcoWitt(ctx, ecServer); err != nil && firstErr == nil {
							firstErr = err
						}
					}
					w.report("ecowitt", firstErr)
				}
			}
			if w.HAEnabled != nil && w.HAEnabled() {
				if contacted, err := w.PollHomeAssistant(ctx); contacted {
					w.report(homeassistant.Source, err)
				}
			}

			w.computeZoneVPD(ctx)

			select {
			case <-pruneTicker.C:
				if err := w.PruneSensorData(); err != nil {
					w.Logger.WithError(err).Error("Scheduled sensor data prune failed")
				} else {
					w.Logger.Info("Scheduled sensor data prune completed")
				}
			default:
			}

			select {
			case <-rollupTicker.C:
				if err := w.RefreshHourlyRollups(); err != nil {
					w.Logger.WithError(err).Error("Scheduled hourly rollup failed")
				}
			default:
			}
		}

		// Wait for either the polling interval or context cancellation.
		select {
		case <-ctx.Done():
			w.Logger.Info("Sensor Watcher shutting down")
			return
		case <-time.After(w.PollingInterval()):
		}
	}
}

// PollEcoWitt fetches livedata from a single EcoWitt server and writes
// matching sensor rows. server is the host[:port] portion as it
// appears in the Store's ECDevices() slice.
func (w *Watcher) PollEcoWitt(ctx context.Context, server string) error {
	w.Logger.WithField("timestamp", w.Now()).Info("Updating EC sensor data")

	url := "http://" + server + "/get_livedata_info"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		w.Logger.WithError(err).Error("Error creating EcoWitt request")
		return &PollError{Kind: "error", Detail: server, Err: err}
	}

	resp, err := w.HTTP.Do(req)
	if err != nil {
		w.Logger.WithError(err).Error("Error sending EcoWitt request")
		return &PollError{Kind: "unreachable", Detail: server, Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxSensorResponseBytes))
	if err != nil {
		w.Logger.WithError(err).Error("Error reading EcoWitt response body")
		return &PollError{Kind: "unreachable", Detail: server, Err: err}
	}

	var apiResponse types.ECWAPIResponse
	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		w.Logger.WithError(err).Error("Error parsing EcoWitt JSON response")
		return &PollError{Kind: "error", Detail: server, Err: err}
	}

	device := server
	source := "ecowitt"
	dataMap := map[string]string{}

	for _, wh := range apiResponse.WH25 {
		dataMap["WH25.InTemp"] = wh.InTemp
		dataMap["WH25.InHumi"] = trimTrailingPercent(wh.InHumi)
	}

	for _, ch := range apiResponse.CHSoil {
		dataMap["Soil."+ch.Channel] = trimTrailingPercent(ch.Humidity)
	}

	for _, ch := range apiResponse.CHAisle {
		dataMap["Aisle."+ch.Channel+".Temp"] = ch.Temp
		dataMap["Aisle."+ch.Channel+".Humi"] = trimTrailingPercent(ch.Humidity)
	}

	for _, ch := range apiResponse.CHEc {
		dataMap["SoilEC."+ch.Channel+".Moisture"] = trimTrailingPercent(ch.Humidity)
		dataMap["SoilEC."+ch.Channel+".Temp"] = ch.Temp
		if ec := ecToMilliSiemens(ch.EC); ec != "" {
			dataMap["SoilEC."+ch.Channel+".EC"] = ec
		}
	}

	for _, item := range apiResponse.CommonList {
		meta, ok := types.ECWCommonSensors[types.NormalizeECWID(item.ID)]
		if !ok {
			continue
		}
		if v := trimCommonVal(item.Val); v != "" {
			dataMap[meta.TypeKey] = v
		}
	}

	for key, value := range dataMap {
		w.addSensorData(source, device, key, value)
	}
	return nil
}

// PollACI fetches the device list from AC Infinity using the supplied
// access token and writes matching sensor rows.
func (w *Watcher) PollACI(ctx context.Context, token string) error {
	w.Logger.WithField("timestamp", w.Now()).Info("Updating ACI sensor data")

	url := w.ACIBaseURL + "/api/user/devInfoListAll?userId=" + token
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		w.Logger.WithError(err).Error("Error creating ACI request")
		return &PollError{Kind: "error", Err: err}
	}

	req.Header.Add("token", token)
	req.Header.Add("Host", "www.acinfinityserver.com")
	req.Header.Add("User-Agent", "okhttp/3.10.0")
	req.Header.Add("Content-Encoding", "gzip")

	resp, err := w.HTTP.Do(req)
	if err != nil {
		w.Logger.WithError(err).Error("Error sending ACI request")
		return &PollError{Kind: "unreachable", Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxSensorResponseBytes))
	if err != nil {
		w.Logger.WithError(err).Error("Error reading ACI response body")
		return &PollError{Kind: "unreachable", Err: err}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		w.Logger.WithField("status", resp.StatusCode).Warn("AC Infinity refused the saved sign-in; sign in again in Settings")
		return &PollError{Kind: "auth", Detail: resp.Status}
	}

	var jsonResponse types.ACIResponse
	if err := json.Unmarshal(respBody, &jsonResponse); err != nil {
		w.Logger.WithError(err).Error("Error unmarshalling ACI JSON response")
		return &PollError{Kind: "error", Err: err}
	}
	// AC Infinity answers 200 OK with its own code: anything but 200 means it
	// refused the request, almost always because the saved token no longer
	// works. Before 1.0.1 this was ignored and polling silently got nothing.
	if jsonResponse.Code != 0 && jsonResponse.Code != aciSuccessCode {
		w.Logger.WithFields(logrus.Fields{"code": jsonResponse.Code, "message": jsonResponse.Msg}).
			Warn("AC Infinity refused the request; sign in again in Settings")
		return &PollError{Kind: "auth", Detail: strings.TrimSpace(fmt.Sprintf("%d %s", jsonResponse.Code, jsonResponse.Msg))}
	}

	source := "acinfinity"
	for _, deviceData := range jsonResponse.Data {
		dataMap := map[string]float64{}
		device := deviceData.DevCode

		dataMap["ACI.tempF"] = float64(deviceData.DeviceInfo.TemperatureF) / 100.0
		dataMap["ACI.tempC"] = float64(deviceData.DeviceInfo.Temperature) / 100.0
		dataMap["ACI.humidity"] = float64(deviceData.DeviceInfo.Humidity) / 100.0

		for _, sensor := range deviceData.DeviceInfo.Sensors {
			value := float64(sensor.SensorData) / 100.0
			if sensor.SensorType == aciCO2SensorType {
				value = float64(sensor.SensorData)
			}
			dataMap[fmt.Sprintf("ACI.%d.%d", sensor.AccessPort, sensor.SensorType)] = value
		}

		for _, port := range deviceData.DeviceInfo.Ports {
			dataMap[fmt.Sprintf("ACIP.%d", port.Port)] = float64(port.Speak) * 10
		}

		for key, value := range dataMap {
			w.addSensorData(source, device, key, fmt.Sprintf("%f", value))
		}
	}
	return nil
}

// PollHomeAssistant reads every state from Home Assistant in one request
// and saves readings for the Home Assistant sensors that were added in
// Manage sensors. Sensors that are unavailable or not a number are skipped.
// If Home Assistant's unit for a temperature changed since the sensor was
// added (°C ↔ °F), the reading is converted to the sensor's saved unit.
//
// contacted is false when nothing was asked of Home Assistant (no sensors
// added yet), so there's nothing to report.
func (w *Watcher) PollHomeAssistant(ctx context.Context) (contacted bool, err error) {
	log := w.Logger.WithField("func", "PollHomeAssistant")

	tracked := map[string]struct {
		id   int
		unit string
	}{}
	rows, err := w.DB.Query("SELECT id, type, COALESCE(unit, '') FROM sensors WHERE source = $1", homeassistant.Source)
	if err != nil {
		log.WithError(err).Error("Error loading Home Assistant sensors")
		return false, nil
	}
	for rows.Next() {
		var id int
		var entityID, unit string
		if err := rows.Scan(&id, &entityID, &unit); err == nil {
			tracked[entityID] = struct {
				id   int
				unit string
			}{id, unit}
		}
	}
	rows.Close()
	if len(tracked) == 0 {
		return false, nil // nothing added yet: don't contact Home Assistant
	}

	client, err := homeassistant.New(w.HAURL(), w.HAToken())
	if err != nil {
		log.WithError(err).Warn("Home Assistant is on but its address or token is missing or invalid")
		return false, nil
	}
	if w.HAHTTP != nil {
		client.HTTP = w.HAHTTP
	}
	states, err := client.States(ctx)
	if err != nil {
		log.WithError(err).Error("Error reading Home Assistant")
		kind := "error"
		switch {
		case errors.Is(err, homeassistant.ErrUnauthorized):
			kind = "auth"
		case errors.Is(err, homeassistant.ErrUnreachable):
			kind = "unreachable"
		}
		return true, &PollError{Kind: kind, Err: err}
	}

	saved := 0
	for _, e := range states {
		t, ok := tracked[e.EntityID]
		if !ok {
			continue
		}
		v, ok := e.Value()
		if !ok {
			log.WithFields(logrus.Fields{"entity": e.EntityID, "state": e.State}).Debug("Home Assistant sensor has no reading")
			continue
		}
		v = homeassistant.ConvertTemperature(v, e.Attributes.Unit, t.unit)
		if _, err := w.DB.Exec("INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2)", t.id, v); err != nil {
			log.WithError(err).WithField("sensorID", t.id).Error("Error writing sensor data to database")
			continue
		}
		saved++
	}
	log.WithField("readings", saved).Debug("Home Assistant poll done")
	return true, nil
}

// addSensorData writes a single (source, device, key, value) tuple to
// sensor_data. Sensors that are not pre-registered in the sensors table
// are silently skipped — that's how the UI lets users opt in to
// tracking specific sensors. Non-numeric values are also silently
// skipped after logging.
func (w *Watcher) addSensorData(source string, device string, key string, value string) {
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		w.Logger.WithFields(logrus.Fields{
			"source": source,
			"device": device,
			"type":   key,
			"value":  value,
			"error":  err,
		}).Error("Error parsing sensor value")
		return
	}

	var sensorID int
	err := w.DB.QueryRow("SELECT id FROM sensors WHERE source = $1 AND device = $2 AND type = $3", source, device, key).Scan(&sensorID)
	if err != nil {
		w.Logger.WithFields(logrus.Fields{
			"source": source,
			"device": device,
			"type":   key,
		}).Debug("Sensor not tracked, skipping")
		return
	}

	if _, err := w.DB.Exec("INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2)", sensorID, value); err != nil {
		w.Logger.WithFields(logrus.Fields{
			"sensorID": sensorID,
			"value":    value,
			"error":    err,
		}).Error("Error writing sensor data to database")
	}
}

// PruneSensorData deletes sensor_data rows older than the configured
// retention window. With retention <= 0 pruning is disabled and the
// method returns nil. SQLite databases additionally run VACUUM,
// ANALYZE, and PRAGMA optimize after the delete so the file shrinks.
func (w *Watcher) PruneSensorData() error {
	w.Logger.Info("Pruning old sensor data")
	days := w.SensorRetention()
	if days <= 0 {
		w.Logger.Info("Sensor data pruning is disabled (sensor_retention_days = 0)")
		// The ANALYZE further down only runs after a prune, so with pruning
		// off (the default) SQLite would never gather planner statistics and
		// time-window queries over the ever-growing sensor_data table could
		// degrade into full scans. Keep statistics fresh regardless.
		if model.IsSQLite() {
			w.refreshSQLiteStatistics()
		}
		return nil
	}

	var pruneQuery string
	if model.IsPostgres() {
		pruneQuery = fmt.Sprintf("DELETE FROM sensor_data WHERE create_dt < NOW() - INTERVAL '%d days'", days)
	} else {
		pruneQuery = fmt.Sprintf("DELETE FROM sensor_data WHERE create_dt < datetime('now', 'localtime', '-%d days')", days)
	}
	if _, err := w.DB.Exec(pruneQuery); err != nil {
		w.Logger.WithError(err).Error("Error pruning sensor data")
		return err
	}
	// rolling_averages is not pruned — it's a trigger-maintained cache with only
	// one row per sensor, so it stays small and self-maintaining.

	if model.IsSQLite() {
		if _, err := w.DB.Exec("VACUUM"); err != nil {
			w.Logger.WithError(err).Warn("SQLite VACUUM failed")
		}
		if _, err := w.DB.Exec("ANALYZE"); err != nil {
			w.Logger.WithError(err).Warn("SQLite ANALYZE failed")
		}
		if _, err := w.DB.Exec("PRAGMA optimize"); err != nil {
			w.Logger.WithError(err).Warn("SQLite PRAGMA optimize failed")
		}
		w.Logger.Info("SQLite post-prune maintenance completed")
	}

	w.Logger.WithField("days", days).Info("Sensor data pruned")
	return nil
}

// sqliteAnalysisLimit caps how many rows ANALYZE samples per index. It is
// approximate by design: the planner only needs to know that create_dt is
// high-cardinality, and a bounded sample keeps ANALYZE (which takes the
// write lock) down to milliseconds even on a multi-million-row table.
const sqliteAnalysisLimit = 1000

// refreshSQLiteStatistics runs a bounded ANALYZE so the query planner has
// sqlite_stat1 data. Failures are logged and otherwise ignored: statistics
// are an optimisation, and the rollup no longer depends on them.
func (w *Watcher) refreshSQLiteStatistics() {
	ctx := context.Background()

	// analysis_limit is per-connection, so the PRAGMA and the ANALYZE must
	// run on the same pooled connection.
	conn, err := w.DB.Conn(ctx)
	if err != nil {
		w.Logger.WithError(err).Warn("SQLite statistics refresh skipped: no connection")
		return
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA analysis_limit = %d", sqliteAnalysisLimit)); err != nil {
		w.Logger.WithError(err).Warn("SQLite statistics refresh failed setting analysis_limit")
		return
	}
	// Put the connection back as it was so a later full ANALYZE that lands
	// on this pooled connection isn't silently limited.
	defer conn.ExecContext(ctx, "PRAGMA analysis_limit = 0") //nolint:errcheck

	start := time.Now()
	if _, err := conn.ExecContext(ctx, "ANALYZE"); err != nil {
		w.Logger.WithError(err).Warn("SQLite statistics refresh failed")
		return
	}
	w.Logger.WithField("duration_ms", time.Since(start).Milliseconds()).Info("SQLite planner statistics refreshed")
}

// trimTrailingPercent strips a single trailing '%' from a value like
// "55%" → "55". The EcoWitt firmware reports humidity values with the
// percent sign included; the historical implementation sliced the
// last byte unconditionally, which would corrupt a future firmware
// that ever drops the suffix. This safer variant matches behavior
// when the suffix is present and is a no-op when it isn't.
func trimTrailingPercent(v string) string {
	if n := len(v); n > 0 && v[n-1] == '%' {
		return v[:n-1]
	}
	return v
}

// trimCommonVal extracts the leading numeric portion from an EcoWitt
// common_list val string. Returns "" for dash-placeholder values
// ("--", "--.-") that indicate no sensor is connected. Handles the
// range of embedded-unit formats the firmware emits: "65%", "0.00mph",
// "29.85 inHg", "43.65 W/m2".
func trimCommonVal(v string) string {
	if len(v) == 0 {
		return ""
	}
	if len(v) >= 2 && v[0] == '-' && v[1] == '-' {
		return ""
	}
	i := 0
	if i < len(v) && (v[i] == '-' || v[i] == '+') {
		i++
	}
	for i < len(v) && ((v[i] >= '0' && v[i] <= '9') || v[i] == '.') {
		i++
	}
	return v[:i]
}

// ecToMilliSiemens converts a WH52 soil-EC reading to milliSiemens/cm. The
// gateway reports EC in microSiemens/cm with the unit inline (e.g.
// "470 uS/cm"), but EC is conventionally displayed in mS/cm, so the numeric
// part is divided by 1000. Returns "" for dash placeholders or unparseable
// input so the caller can skip storing a row.
func ecToMilliSiemens(v string) string {
	n := trimCommonVal(v)
	if n == "" {
		return ""
	}
	f, err := strconv.ParseFloat(n, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatFloat(f/1000.0, 'f', -1, 64)
}

// computeZoneVPD calculates and stores derived VPD sensor readings for every
// zone that has VPD enabled (leaf_temp_offset IS NOT NULL) and both source
// sensor IDs configured. It is called once per poll cycle, after device polling.
func (w *Watcher) computeZoneVPD(_ context.Context) {
	fieldLogger := w.Logger.WithField("func", "computeZoneVPD")

	// Load all zones that have VPD configured
	rows, err := w.DB.Query(`
		SELECT id, leaf_temp_offset, vpd_temp_sensor_id, vpd_humidity_sensor_id
		FROM zones
		WHERE leaf_temp_offset IS NOT NULL
		  AND vpd_temp_sensor_id IS NOT NULL
		  AND vpd_humidity_sensor_id IS NOT NULL`)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query VPD-enabled zones")
		return
	}
	defer rows.Close()

	type vpdZone struct {
		ID                  int
		LeafTempOffset      float64
		VPDTempSensorID     int
		VPDHumiditySensorID int
	}

	var zones []vpdZone
	for rows.Next() {
		var z vpdZone
		if err := rows.Scan(&z.ID, &z.LeafTempOffset, &z.VPDTempSensorID, &z.VPDHumiditySensorID); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan VPD zone row")
			continue
		}
		zones = append(zones, z)
	}
	rows.Close()

	for _, z := range zones {
		// Read the latest value and unit for the temperature sensor
		var tempValue float64
		var tempUnit string
		err := w.DB.QueryRow(`
			SELECT sd.value, s.unit
			FROM sensor_data sd
			JOIN sensors s ON s.id = sd.sensor_id
			WHERE sd.sensor_id = $1
			ORDER BY sd.id DESC LIMIT 1`, z.VPDTempSensorID).Scan(&tempValue, &tempUnit)
		if err != nil {
			fieldLogger.WithFields(logrus.Fields{"zone_id": z.ID, "sensor_id": z.VPDTempSensorID}).
				Debug("No temperature reading for VPD zone, skipping")
			continue
		}

		// Read the latest humidity value
		var humValue float64
		err = w.DB.QueryRow(`
			SELECT value FROM sensor_data
			WHERE sensor_id = $1
			ORDER BY id DESC LIMIT 1`, z.VPDHumiditySensorID).Scan(&humValue)
		if err != nil {
			fieldLogger.WithFields(logrus.Fields{"zone_id": z.ID, "sensor_id": z.VPDHumiditySensorID}).
				Debug("No humidity reading for VPD zone, skipping")
			continue
		}

		// The offset is stored in °F as a temperature *delta*, so it always
		// converts to a °C delta by *5/9 (no -32 — it is a difference, not an
		// absolute temperature). The air temperature is converted to °C only
		// when the source sensor reports Fahrenheit (unit lacks "C").
		airTempC := tempValue
		if !strings.Contains(tempUnit, "C") {
			airTempC = utils.FahrenheitToCelsius(tempValue)
		}
		leafOffsetC := z.LeafTempOffset * 5.0 / 9.0

		vpd := utils.VPD(airTempC, humValue, leafOffsetC)

		// Ensure the derived VPD sensor row exists for this zone
		zoneIDStr := strconv.Itoa(z.ID)
		sensorName := "VPD (Zone " + zoneIDStr + ")"
		var vpdSensorID int
		err = w.DB.QueryRow(
			`SELECT id FROM sensors WHERE source = $1 AND device = $2 AND type = $3`,
			"derived", zoneIDStr, "VPD",
		).Scan(&vpdSensorID)
		if err == sql.ErrNoRows {
			// Create the derived VPD sensor
			err = w.DB.QueryRow(
				`INSERT INTO sensors (name, source, device, type, zone_id, unit, visibility)
                 VALUES ($1, $2, $3, $4, $5, $6, 'zone_plant')
                 RETURNING id`,
				sensorName, "derived", zoneIDStr, "VPD", z.ID, "kPa",
			).Scan(&vpdSensorID)
			if err != nil {
				fieldLogger.WithError(err).WithField("zone_id", z.ID).Error("Failed to create derived VPD sensor")
				continue
			}
			fieldLogger.WithField("zone_id", z.ID).Info("Created derived VPD sensor")
		} else if err != nil {
			fieldLogger.WithError(err).WithField("zone_id", z.ID).Error("Failed to look up derived VPD sensor")
			continue
		}

		// Insert the computed VPD reading
		if _, err := w.DB.Exec(
			"INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2)",
			vpdSensorID, vpd,
		); err != nil {
			fieldLogger.WithError(err).WithField("zone_id", z.ID).Error("Failed to insert VPD sensor data")
		}
	}
}
