package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"isley/homeassistant"
	"isley/logger"
	"isley/model"
	"isley/utils"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ----------------------------------------------------------------------
// Scanning: found sensors, previewed before they are saved
// ----------------------------------------------------------------------

// foundSensor is one sensor a scan discovered, identified (like a saved
// sensor) by source + device + type.
type foundSensor struct {
	Source string `json:"source"`
	Device string `json:"device"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Unit   string `json:"unit"`
	// Home Assistant scans also report the current reading, and More marks
	// sensors that are only listed with "Show all" (batteries, power, …).
	Value *float64 `json:"value,omitempty"`
	More  bool     `json:"more,omitempty"`
}

type sensorCollector []foundSensor

func (f *sensorCollector) add(source, device, sensorType, name, unit string) {
	*f = append(*f, foundSensor{Source: source, Device: device, Type: sensorType, Name: name, Unit: unit})
}

// newSensorsOnly keeps the found sensors that aren't saved yet (and drops
// repeats within the scan).
func newSensorsOnly(db *sql.DB, found []foundSensor) []foundSensor {
	out := []foundSensor{}
	seen := map[string]bool{}
	for _, f := range found {
		key := f.Source + "\x00" + f.Device + "\x00" + f.Type
		if seen[key] {
			continue
		}
		seen[key] = true
		var id int
		err := db.QueryRow(`SELECT id FROM sensors WHERE source = $1 AND device = $2 AND type = $3`,
			f.Source, f.Device, f.Type).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			out = append(out, f)
		} else if err != nil {
			logger.Log.WithField("func", "newSensorsOnly").WithError(err).Error("Error checking sensor")
		}
	}
	return out
}

// scannableSources are the integrations whose scan results can be added
// one by one from the Manage sensors page.
var scannableSources = map[string]bool{"acinfinity": true, "ecowitt": true, homeassistant.Source: true}

// AddScannedSensorHandler saves one sensor from a scan preview into a tent
// (POST /sensors/add). Adding the same sensor twice is a no-op.
func AddScannedSensorHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddScannedSensorHandler")
	var input struct {
		Source  string `json:"source"`
		Device  string `json:"device"`
		Type    string `json:"type"`
		Name    string `json:"name"`
		Unit    string `json:"unit"`
		ZoneID  *int   `json:"zone_id"`
		NewZone string `json:"new_zone"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.NewZone = strings.TrimSpace(input.NewZone)
	if !scannableSources[input.Source] || input.Device == "" || input.Type == "" || input.Name == "" {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	for _, v := range []struct {
		field, value string
		max          int
	}{
		{"device", input.Device, utils.MaxDeviceLength},
		{"type", input.Type, utils.MaxTypeLength},
		{"name", input.Name, utils.MaxNameLength},
		{"unit", input.Unit, utils.MaxUnitLength},
		{"new_zone", input.NewZone, utils.MaxNameLength},
	} {
		if err := utils.ValidateStringLength(v.field, v.value, v.max); err != nil {
			apiBadRequest(c, err.Error())
			return
		}
	}

	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)
	if input.ZoneID != nil && !zoneExists(db, *input.ZoneID) {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	if input.ZoneID == nil && input.NewZone != "" {
		zoneID, err := CreateNewZone(db, store, input.NewZone)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			apiInternalError(c, "api_zone_creation_failed")
			return
		}
		input.ZoneID = &zoneID
	}

	id, err := findOrCreateSensor(db, input.Name, input.Source, input.Device, input.Type, input.ZoneID, input.Unit)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add sensor")
		apiInternalError(c, "api_failed_to_update_sensor")
		return
	}
	if input.Source == "ecowitt" {
		// The watcher polls the gateways listed in the store.
		if devices, err := LoadEcDevices(db); err == nil {
			store.SetECDevices(devices)
		}
	}
	SensorCacheServiceFromContext(c).GroupedReset()
	zoneID := 0
	if input.ZoneID != nil {
		zoneID = *input.ZoneID
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "zone_id": zoneID})
}

func zoneExists(db *sql.DB, id int) bool {
	var n int
	return db.QueryRow(`SELECT COUNT(*) FROM zones WHERE id = $1`, id).Scan(&n) == nil && n > 0
}

// ----------------------------------------------------------------------
// Sensor list for the Environment and Manage sensors pages
// ----------------------------------------------------------------------

// ManagedSensor is a sensor with where it shows, its latest reading and
// the living plants linked to it.
type ManagedSensor struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Source   string   `json:"source"`
	Device   string   `json:"device"`
	Type     string   `json:"type"`
	Unit     string   `json:"unit"`
	ZoneID   int      `json:"zone_id"` // 0 = no tent
	Zone     string   `json:"zone"`
	ShowEnv  bool     `json:"show_env"`  // Environment page (sensors.show)
	ShowHome bool     `json:"show_home"` // Home dashboard (visibility zone / zone_plant)
	Hidden   bool     `json:"hidden"`    // visibility hide
	Value    *float64 `json:"value"`
	LastAt   int64    `json:"last_at"` // latest reading, epoch ms; 0 = none yet
	Stale    bool     `json:"stale"`
	PlantIDs []int    `json:"plant_ids"`
	// Climate is set for a zone's chosen climate sensors (Edit zone).
	Climate *ClimateCheck `json:"climate,omitempty"`
}

// showsOn maps the stored visibility value onto the Home / Hidden ticks.
func showsOn(visibility string) (home, hidden bool) {
	switch visibility {
	case "hide":
		return false, true
	case "plant":
		return false, false
	default: // zone, zone_plant
		return true, false
	}
}

// visibilityFor is the reverse of showsOn.
func visibilityFor(home, hidden bool) string {
	switch {
	case hidden:
		return "hide"
	case home:
		return "zone_plant"
	default:
		return "plant"
	}
}

// staleAfter is how long without a reading before a sensor counts as not
// reporting: three polling intervals, at least five minutes.
func staleAfter(pollSeconds int) time.Duration {
	d := 3 * time.Duration(pollSeconds) * time.Second
	if d < 5*time.Minute {
		d = 5 * time.Minute
	}
	return d
}

// ListManagedSensors returns every sensor with its tent, display ticks,
// latest reading and linked living plants.
func ListManagedSensors(db *sql.DB, pollSeconds int) []ManagedSensor {
	fieldLogger := logger.Log.WithField("func", "ListManagedSensors")
	rows, err := db.Query(`
		SELECT s.id, s.name, s.source, s.device, s.type, COALESCE(s.unit, ''),
		       COALESCE(s.zone_id, 0), COALESCE(z.name, ''), s.show, COALESCE(s.visibility, 'zone_plant'),
		       sd.value, sd.create_dt
		FROM sensors s
		LEFT JOIN zones z ON z.id = s.zone_id
		LEFT JOIN sensor_data sd ON sd.id = (SELECT MAX(id) FROM sensor_data WHERE sensor_id = s.id)
		ORDER BY COALESCE(z.name, ''), s.source, s.device, s.type, s.name`)
	if err != nil {
		fieldLogger.WithError(err).Error("Error querying sensors")
		return []ManagedSensor{}
	}
	defer rows.Close()

	links := map[int][]int{}
	if plants, err := loadActivePlantLinks(db); err == nil {
		for _, p := range plants {
			for _, sid := range p.SensorIDs {
				links[sid] = append(links[sid], p.ID)
			}
		}
	}

	out := []ManagedSensor{}
	now := time.Now()
	for rows.Next() {
		var s ManagedSensor
		var visibility string
		var value sql.NullFloat64
		var at sql.NullTime
		if err := rows.Scan(&s.ID, &s.Name, &s.Source, &s.Device, &s.Type, &s.Unit,
			&s.ZoneID, &s.Zone, &s.ShowEnv, &visibility, &value, &at); err != nil {
			fieldLogger.WithError(err).Error("Error scanning sensor")
			continue
		}
		s.ShowHome, s.Hidden = showsOn(visibility)
		if value.Valid {
			v := value.Float64
			s.Value = &v
		}
		if at.Valid {
			s.LastAt = at.Time.UnixMilli()
			s.Stale = now.Sub(at.Time) > staleAfter(pollSeconds)
		}
		s.PlantIDs = links[s.ID]
		if s.PlantIDs == nil {
			s.PlantIDs = []int{}
		}
		out = append(out, s)
	}
	return out
}

// activePlantLink is a living plant with its zone and linked sensor ids.
type activePlantLink struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ZoneID    int    `json:"zone_id"`
	SensorIDs []int  `json:"-"`
	raw       string
}

// loadActivePlantLinks returns the living plants (latest status is an
// active stage) with their linked sensors, sorted by name.
func loadActivePlantLinks(db *sql.DB) ([]activePlantLink, error) {
	orderExpr := "strftime('%s', psl.date)"
	if model.IsPostgres() {
		orderExpr = "EXTRACT(EPOCH FROM psl.date)"
	}
	rows, err := db.Query(fmt.Sprintf(`
		SELECT p.id, p.name, COALESCE(p.zone_id, 0), COALESCE(p.sensors, '[]')
		FROM plant p
		WHERE (
			SELECT ps.active
			FROM plant_status_log psl
			JOIN plant_status ps ON psl.status_id = ps.id
			WHERE psl.plant_id = p.id
			ORDER BY %s DESC
			LIMIT 1
		) = 1
		ORDER BY p.name`, orderExpr))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []activePlantLink
	for rows.Next() {
		var p activePlantLink
		if err := rows.Scan(&p.ID, &p.Name, &p.ZoneID, &p.raw); err != nil {
			return nil, err
		}
		if strings.TrimSpace(p.raw) != "" {
			if err := json.Unmarshal([]byte(p.raw), &p.SensorIDs); err != nil {
				p.SensorIDs = nil
				p.raw = "!" // unreadable: never rewritten
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------------
// Editing a sensor (Manage sensors → Edit)
// ----------------------------------------------------------------------

// UpdateSensorHandler saves the Edit sensor window (PUT /sensors/:id):
// name, tent, unit, where it shows, and which living plants link to it.
func UpdateSensorHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateSensorHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	var input struct {
		Name     string `json:"name"`
		ZoneID   *int   `json:"zone_id"` // nil or 0 = no tent
		Unit     string `json:"unit"`
		ShowEnv  bool   `json:"show_env"`
		ShowHome bool   `json:"show_home"`
		Hidden   bool   `json:"hidden"`
		PlantIDs []int  `json:"plant_ids"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	if err := utils.ValidateStringLength("name", input.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStringLength("unit", input.Unit, utils.MaxUnitLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	db := DBFromContext(c)
	var zoneID any
	if input.ZoneID != nil && *input.ZoneID != 0 {
		if !zoneExists(db, *input.ZoneID) {
			apiBadRequest(c, "api_invalid_input")
			return
		}
		zoneID = *input.ZoneID
	}

	plants, err := loadActivePlantLinks(db)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to load plants")
		apiInternalError(c, "api_failed_to_update_sensor")
		return
	}
	living := map[int]bool{}
	for _, p := range plants {
		living[p.ID] = true
	}
	want := map[int]bool{}
	for _, pid := range input.PlantIDs {
		if !living[pid] {
			apiBadRequest(c, "api_invalid_input")
			return
		}
		want[pid] = true
	}

	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_failed_to_update_sensor")
		return
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	res, err := tx.Exec(`UPDATE sensors SET name = $1, zone_id = $2, unit = $3, show = $4, visibility = $5 WHERE id = $6`,
		input.Name, zoneID, input.Unit, input.ShowEnv, visibilityFor(input.ShowHome, input.Hidden), id)
	if err != nil {
		fieldLogger.WithError(err).Error("Error updating sensor")
		apiInternalError(c, "api_failed_to_update_sensor")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		apiError(c, http.StatusNotFound, "api_invalid_input")
		return
	}

	// Living plants only: finished plants keep the links they had.
	for _, p := range plants {
		if p.raw == "!" {
			continue
		}
		next, changed := withSensorLink(p.SensorIDs, id, want[p.ID])
		if !changed {
			continue
		}
		b, _ := json.Marshal(next)
		if _, err := tx.Exec(`UPDATE plant SET sensors = $1 WHERE id = $2`, string(b), p.ID); err != nil {
			fieldLogger.WithError(err).Error("Error updating plant sensor links")
			apiInternalError(c, "api_failed_to_update_sensor")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		apiInternalError(c, "api_failed_to_update_sensor")
		return
	}
	SensorCacheServiceFromContext(c).GroupedReset()
	apiOK(c, "api_sensor_updated")
}

// withSensorLink adds or removes sensorID in a plant's linked sensor ids.
func withSensorLink(ids []int, sensorID int, linked bool) ([]int, bool) {
	has := false
	out := make([]int, 0, len(ids)+1)
	for _, v := range ids {
		if v == sensorID {
			has = true
			if !linked {
				continue
			}
		}
		out = append(out, v)
	}
	if linked && !has {
		out = append(out, sensorID)
	}
	return out, has != linked
}

// ----------------------------------------------------------------------
// Chart data (Environment page)
// ----------------------------------------------------------------------

const (
	// seriesMaxPoints caps the points sent per sensor; more than a chart
	// can show anyway.
	seriesMaxPoints = 600
	// seriesMaxSensors and seriesMaxSpan bound one request.
	seriesMaxSensors = 50
	seriesMaxSpan    = 400 * 24 * time.Hour
)

// seriesPoint is one reading, or one hourly rollup (average and highest).
type seriesPoint struct {
	t         time.Time
	avg, peak float64
}

// SensorSeriesHandler returns readings for several sensors over a time
// range given in epoch milliseconds (GET /sensors/series?ids=&from=&to=).
// Ranges up to 24 hours come from the raw readings, longer ones from the
// hourly rollups. Each series is thinned to at most seriesMaxPoints
// (averaged; AC Infinity ports keep the highest value so short runs show).
func SensorSeriesHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SensorSeriesHandler")
	ids, ok := parseSensorIDs(c.Query("ids"))
	from, errF := strconv.ParseInt(c.Query("from"), 10, 64)
	to, errT := strconv.ParseInt(c.Query("to"), 10, 64)
	if !ok || errF != nil || errT != nil || to <= from {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	start, end := time.UnixMilli(from).UTC(), time.UnixMilli(to).UTC()
	if end.Sub(start) > seriesMaxSpan {
		apiBadRequest(c, "api_invalid_input")
		return
	}

	db := DBFromContext(c)
	isPort, err := sensorPortFlags(db, ids)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to load sensors")
		apiInternalError(c, "api_sensor_query_failed")
		return
	}
	raw := end.Sub(start) <= RollupThresholdHours*time.Hour
	points, err := querySeries(db, ids, start, end, raw)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query readings")
		apiInternalError(c, "api_sensor_query_failed")
		return
	}

	out := make(map[string][][2]float64, len(ids))
	for _, id := range ids {
		out[strconv.Itoa(id)] = thinSeries(points[id], start, end, seriesMaxPoints, isPort[id])
	}
	c.JSON(http.StatusOK, gin.H{"series": out, "raw": raw})
}

func parseSensorIDs(s string) ([]int, bool) {
	var ids []int
	seen := map[int]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil || id <= 0 {
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, len(ids) > 0 && len(ids) <= seriesMaxSensors
}

// sensorPortFlags reports which of the sensors are AC Infinity ports.
func sensorPortFlags(db *sql.DB, ids []int) (map[int]bool, error) {
	ph, args := inPlaceholders(ids)
	rows, err := db.Query(`SELECT id, type FROM sensors WHERE id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		var t string
		if err := rows.Scan(&id, &t); err != nil {
			return nil, err
		}
		out[id] = strings.HasPrefix(t, "ACIP.")
	}
	return out, rows.Err()
}

func querySeries(db *sql.DB, ids []int, start, end time.Time, raw bool) (map[int][]seriesPoint, error) {
	ph, args := inPlaceholders(ids)
	n := len(ids)
	args = append(args, start.Format(utils.LayoutDB), end.Format(utils.LayoutDB))
	var q string
	if raw {
		q = fmt.Sprintf(`SELECT sensor_id, value, value, create_dt FROM sensor_data
			WHERE sensor_id IN (%s) AND create_dt BETWEEN $%d AND $%d
			ORDER BY create_dt LIMIT %d`, ph, n+1, n+2, MaxRawDataRows*n)
	} else {
		q = fmt.Sprintf(`SELECT sensor_id, avg_val, max_val, bucket FROM sensor_data_hourly
			WHERE sensor_id IN (%s) AND bucket BETWEEN $%d AND $%d
			ORDER BY bucket LIMIT %d`, ph, n+1, n+2, MaxRawDataRows*n)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int][]seriesPoint{}
	for rows.Next() {
		var id int
		var p seriesPoint
		if err := rows.Scan(&id, &p.avg, &p.peak, &p.t); err != nil {
			return nil, err
		}
		out[id] = append(out[id], p)
	}
	return out, rows.Err()
}

// thinSeries groups points into at most n even time slots over
// [start, end] and returns one point per slot: the average, or the
// highest value for ports.
func thinSeries(points []seriesPoint, start, end time.Time, n int, usePeak bool) [][2]float64 {
	out := [][2]float64{}
	width := end.Sub(start) / time.Duration(n)
	if width <= 0 {
		width = time.Millisecond
	}
	type slot struct {
		first     time.Time
		sum, peak float64
		count     int
	}
	var slots []slot
	index := map[int64]int{}
	for _, p := range points {
		k := int64(p.t.Sub(start) / width)
		j, ok := index[k]
		if !ok {
			j = len(slots)
			index[k] = j
			slots = append(slots, slot{first: p.t, peak: p.peak})
		}
		s := &slots[j]
		s.sum += p.avg
		s.count++
		if p.peak > s.peak {
			s.peak = p.peak
		}
	}
	sort.Slice(slots, func(a, b int) bool { return slots[a].first.Before(slots[b].first) })
	for _, s := range slots {
		v := s.sum / float64(s.count)
		if usePeak {
			v = s.peak
		}
		out = append(out, [2]float64{float64(s.first.UnixMilli()), v})
	}
	return out
}

// ----------------------------------------------------------------------
// Page data
// ----------------------------------------------------------------------

type pageZone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SensorsPageData is the JSON the Environment page (prefix "en_") and the
// Manage sensors page (prefix "sm_") draw from: sensors with their latest
// readings, tents, units and the page's translated strings.
func SensorsPageData(c *gin.Context, prefix string, lang string, translations map[string]string) gin.H {
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)
	t := map[string]string{}
	for k, v := range translations {
		if strings.HasPrefix(k, prefix) || strings.HasPrefix(k, "sx_") {
			t[k] = v
		}
	}
	zones := []pageZone{}
	for _, z := range store.Zones() {
		zones = append(zones, pageZone{ID: int(z.ID), Name: z.Name})
	}
	sort.Slice(zones, func(a, b int) bool { return strings.ToLower(zones[a].Name) < strings.ToLower(zones[b].Name) })
	data := gin.H{
		"sensors": withClimate(db, ListManagedSensors(db, store.PollingInterval()), requestLocation(c)),
		"zones":   zones,
		"units":   utils.NormalizeUnits(store.Units()),
		"poll":    store.PollingInterval(),
		"now":     time.Now().UnixMilli(),
		"lang":    lang,
		"t":       t,
	}
	if prefix == "sm_" {
		plants, err := loadActivePlantLinks(db)
		if err != nil || plants == nil {
			plants = []activePlantLink{}
		}
		devices := store.ECDevices()
		data["plants"] = plants
		data["aci"] = gin.H{"enabled": store.ACIEnabled() == 1, "token_set": store.ACIToken() != ""}
		data["ecowitt"] = gin.H{"enabled": store.ECEnabled() == 1, "devices": devices}
		// The address is shown on the card; the token itself never leaves the server.
		data["homeassistant"] = gin.H{"enabled": store.HAEnabled() == 1, "url": store.HAURL(), "token_set": store.HAToken() != ""}
		data["status"] = IntegrationStatuses(db, store)
	}
	return data
}

// ZoneCounts returns, per zone id, how many living plants and how many
// sensors it has (Settings → Grow setup → Zones).
func ZoneCounts(db *sql.DB) (plants, sensors map[uint]int) {
	plants, sensors = map[uint]int{}, map[uint]int{}
	if list, err := loadActivePlantLinks(db); err == nil {
		for _, p := range list {
			if p.ZoneID > 0 {
				plants[uint(p.ZoneID)]++
			}
		}
	}
	rows, err := db.Query(`SELECT zone_id, COUNT(*) FROM sensors WHERE zone_id IS NOT NULL GROUP BY zone_id`)
	if err != nil {
		logger.Log.WithField("func", "ZoneCounts").WithError(err).Error("Error counting sensors")
		return plants, sensors
	}
	defer rows.Close()
	for rows.Next() {
		var zone, n int
		if rows.Scan(&zone, &n) == nil && zone > 0 {
			sensors[uint(zone)] = n
		}
	}
	return plants, sensors
}

// withClimate adds each zone's climate check to its chosen climate sensors.
func withClimate(db *sql.DB, sensors []ManagedSensor, loc *time.Location) []ManagedSensor {
	checks := ClimateChecks(db, sensors, loc)
	for i := range sensors {
		if ck, ok := checks[sensors[i].ID]; ok {
			sensors[i].Climate = &ck
		}
	}
	return sensors
}
