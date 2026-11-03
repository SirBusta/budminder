package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/alerts"
	"isley/climate"
	"isley/logger"
	"isley/model"
	"isley/utils"
)

// Climate settings keys (Settings > Grow setup > Climate targets and limits).
const (
	climateSplitLightsKey  = "climate_split_lights"  // one choice per measure: vpd,temp,rh
	climateFlowerPhasesKey = "climate_flower_phases" // the same
	climatePhaseWeeksKey   = "climate_phase_weeks"
	climateTempByStageKey  = "climate_temp_by_stage"
	climateRHByStageKey    = "climate_rh_by_stage"
	climateTempLimitsKey   = "climate_temp_limits"
	climateRHLimitsKey     = "climate_rh_limits"
)

// ClimateTarget is one Low / Target / High row. Temperature is in °C.
type ClimateTarget struct {
	Stage   string `json:"stage"`
	Phase   string `json:"phase"`
	Measure string `json:"measure"`
	Lights  string `json:"lights"`
	climate.Range
}

// ClimateSettings is the whole Climate targets and limits card. SplitLights
// and FlowerPhases hold one choice per measure, in climateMeasures order. The
// Flower weeks are the same for all three.
type ClimateSettings struct {
	SplitLights  [3]bool         `json:"split_lights"`
	FlowerPhases [3]bool         `json:"flower_phases"`
	PhaseWeeks   [2]int          `json:"phase_weeks"` // last week of Stretch, last week of Full bloom
	TempByStage  bool            `json:"temp_by_stage"`
	RHByStage    bool            `json:"rh_by_stage"`
	TempLimits   [2]*float64     `json:"temp_limits"` // °C; nil = no limit; used with by stage off
	RHLimits     [2]*float64     `json:"rh_limits"`
	Targets      []ClimateTarget `json:"targets"`
}

// climateMeasures is the order of SplitLights and FlowerPhases.
var climateMeasures = [3]string{climate.VPD, climate.Temp, climate.RH}

func measureIndex(measure string) int {
	for i, m := range climateMeasures {
		if m == measure {
			return i
		}
	}
	return 0
}

// climateKey identifies a target row.
type climateKey struct{ stage, phase, measure, lights string }

// climateRows lists every row the card has: each measure for Seedling, Veg,
// Flower (whole and each phase) with lights on and off, and temperature and
// humidity for Drying (lights off). VPD doesn't apply to Drying.
func climateRows() []climateKey {
	var out []climateKey
	for _, m := range climateMeasures {
		for _, sp := range [][2]string{{climate.Seedling, ""}, {climate.Veg, ""}, {climate.Flower, ""},
			{climate.Flower, climate.Stretch}, {climate.Flower, climate.Bloom}, {climate.Flower, climate.Ripen}} {
			for _, l := range []string{"on", "off"} {
				out = append(out, climateKey{sp[0], sp[1], m, l})
			}
		}
		if m != climate.VPD {
			out = append(out, climateKey{climate.Drying, "", m, "off"})
		}
	}
	return out
}

// Allowed values per measure (temperature in °C).
var climateBounds = map[string][2]float64{climate.VPD: {0, 5}, climate.Temp: {-20, 60}, climate.RH: {0, 100}}

func fToC(f float64) float64 { return math.Round((f-32)*5/9*10000) / 10000 }

// A new install's VPD Targets, [lights on, lights off]. VPD has no limits out
// of the box. Flower as one stage uses Full bloom's values.
var climateDefaultVPD = map[string][2]float64{
	climate.Seedling: {0.8, 0.8}, climate.Veg: {1.0, 1.0}, climate.Flower: {1.3, 1.2},
	climate.Stretch: {1.2, 1.2}, climate.Bloom: {1.3, 1.2}, climate.Ripen: {1.5, 1.2},
}

// A new install's limits for temperature (°F) and humidity, the same for
// lights on and off and for each Flower phase, with no Target. They are wide,
// for serious problems only.
var climateDefaultLimits = map[string]map[string][2]float64{
	climate.Temp: {climate.Seedling: {60, 90}, climate.Veg: {55, 90}, climate.Flower: {55, 86}},
	climate.RH:   {climate.Seedling: {55, 90}, climate.Veg: {45, 85}, climate.Flower: {30, 70}},
}

// Drying starts with all three values, in °F and %.
var climateDefaultDrying = [3]float64{55, 60, 65}

// The limits with Targets and limits by stage turned off: the widest of the
// values above.
var (
	climateDefaultFlatTemp = [2]float64{55, 90} // °F
	climateDefaultFlatRH   = [2]float64{30, 90}
)

// climateDefault is a row's value on a new install.
func climateDefault(k climateKey) climate.Range {
	val := func(v float64) *float64 {
		if k.measure == climate.Temp {
			v = fToC(v)
		}
		return &v
	}
	if k.stage == climate.Drying {
		d := climateDefaultDrying
		return climate.Range{Low: val(d[0]), Target: val(d[1]), High: val(d[2])}
	}
	if k.measure == climate.VPD {
		key := k.stage
		if k.phase != "" {
			key = k.phase
		}
		t := climateDefaultVPD[key][0]
		if k.lights == "off" {
			t = climateDefaultVPD[key][1]
		}
		return climate.Range{Target: &t}
	}
	l := climateDefaultLimits[k.measure][k.stage]
	return climate.Range{Low: val(l[0]), High: val(l[1])}
}

// The VPD ranges migration 019 seeded; an install that still has them gets
// the defaults instead of a copy.
var vpd019 = map[string][2]float64{"Seedling": {0.4, 0.8}, "Veg": {0.8, 1.2}, "Flower": {1.2, 1.5}}

// statusIDs maps stage keys to plant_status ids.
func statusIDs(q interface {
	Query(string, ...any) (*sql.Rows, error)
}) (map[string]int, error) {
	rows, err := q.Query("SELECT id, status FROM plant_status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byName := map[string]int{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		byName[name] = id
	}
	out := map[string]int{}
	for stage, name := range climate.StageStatus {
		if id, ok := byName[name]; ok {
			out[stage] = id
		}
	}
	return out, rows.Err()
}

// EnsureClimateTargets fills climate_target when it is empty: on first start
// after migration 046, and after restoring a backup made before it. VPD keeps
// each stage's old Low and High as limits if someone changed them from the 019
// defaults, with the default Target (halfway when that isn't between them).
func EnsureClimateTargets(db *sql.DB) error {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM climate_target").Scan(&n); err != nil || n > 0 {
		return err
	}
	ids, err := statusIDs(db)
	if err != nil {
		return err
	}
	old := map[string][2]*float64{}
	rows, err := db.Query("SELECT status, vpd_low, vpd_high FROM plant_status")
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		var lo, hi sql.NullFloat64
		if err := rows.Scan(&name, &lo, &hi); err != nil {
			rows.Close()
			return err
		}
		var r [2]*float64
		if lo.Valid {
			r[0] = &lo.Float64
		}
		if hi.Valid {
			r[1] = &hi.Float64
		}
		old[name] = r
	}
	rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, k := range climateRows() {
		sid, ok := ids[k.stage]
		if !ok {
			continue
		}
		v := climateDefault(k)
		if k.measure == climate.VPD {
			name := climate.StageStatus[k.stage]
			if r, ok := old[name]; ok && r[0] != nil && r[1] != nil && *r[0] < *r[1] {
				if def, had := vpd019[name]; !had || *r[0] != def[0] || *r[1] != def[1] {
					t := math.Round((*r[0]+*r[1])/2*1000) / 1000
					if v.Target != nil && *v.Target > *r[0] && *v.Target < *r[1] {
						t = *v.Target
					}
					v = climate.Range{Low: r[0], Target: &t, High: r[1]}
				}
			}
		}
		if _, err := tx.Exec(`INSERT INTO climate_target (status_id, phase, measure, lights, low, target, high)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, sid, k.phase, k.measure, k.lights, v.Low, v.Target, v.High); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func settingBool(db *sql.DB, name string, def bool) bool {
	v, _ := GetSetting(db, name)
	switch v {
	case "1", "true":
		return true
	case "0", "false":
		return false
	}
	return def
}

func parseLimits(s string) [2]*float64 {
	var out [2]*float64
	parts := strings.Split(s, ",")
	for i := 0; i < 2 && i < len(parts); i++ {
		if v, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64); err == nil {
			out[i] = &v
		}
	}
	return out
}

// parseChoices reads one choice per measure, or def when never saved.
func parseChoices(s string, def [3]bool) [3]bool {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return def
	}
	var out [3]bool
	for i, p := range parts {
		out[i] = p == "1"
	}
	return out
}

func formatChoices(c [3]bool) string {
	parts := make([]string, 3)
	for i, on := range c {
		parts[i] = "0"
		if on {
			parts[i] = "1"
		}
	}
	return strings.Join(parts, ",")
}

// loadLimits reads a limit pair, or the defaults when never saved. A saved pair
// with both boxes empty is "," so it stays empty.
func loadLimits(db *sql.DB, name string, def [2]*float64) [2]*float64 {
	v, _ := GetSetting(db, name)
	if v == "" {
		return def
	}
	return parseLimits(v)
}

func formatLimits(l [2]*float64) string {
	s := func(p *float64) string {
		if p == nil {
			return ""
		}
		return strconv.FormatFloat(*p, 'f', -1, 64)
	}
	return s(l[0]) + "," + s(l[1])
}

// LoadClimate reads the Climate targets and limits card. VPD starts with
// separate lights-off values and Flower phases, the others without.
func LoadClimate(db *sql.DB) (ClimateSettings, error) {
	splits, _ := GetSetting(db, climateSplitLightsKey)
	phases, _ := GetSetting(db, climateFlowerPhasesKey)
	tempLo, tempHi := fToC(climateDefaultFlatTemp[0]), fToC(climateDefaultFlatTemp[1])
	rhLo, rhHi := climateDefaultFlatRH[0], climateDefaultFlatRH[1]
	cs := ClimateSettings{
		SplitLights:  parseChoices(splits, [3]bool{true, false, false}),
		FlowerPhases: parseChoices(phases, [3]bool{true, false, false}),
		TempByStage:  settingBool(db, climateTempByStageKey, true),
		RHByStage:    settingBool(db, climateRHByStageKey, true),
		TempLimits:   loadLimits(db, climateTempLimitsKey, [2]*float64{&tempLo, &tempHi}),
		RHLimits:     loadLimits(db, climateRHLimitsKey, [2]*float64{&rhLo, &rhHi}),
		PhaseWeeks:   [2]int{4, 8},
		Targets:      []ClimateTarget{},
	}
	if v, _ := GetSetting(db, climatePhaseWeeksKey); v != "" {
		a, b, _ := strings.Cut(v, ",")
		w1, e1 := strconv.Atoi(a)
		w2, e2 := strconv.Atoi(b)
		if e1 == nil && e2 == nil && w1 >= 1 && w2 > w1 {
			cs.PhaseWeeks = [2]int{w1, w2}
		}
	}

	rows, err := db.Query(`SELECT ps.status, ct.phase, ct.measure, ct.lights, ct.low, ct.target, ct.high
		FROM climate_target ct JOIN plant_status ps ON ps.id = ct.status_id`)
	if err != nil {
		return cs, err
	}
	defer rows.Close()
	stageOf := map[string]string{}
	for k, v := range climate.StageStatus {
		stageOf[v] = k
	}
	for rows.Next() {
		var t ClimateTarget
		var status string
		var lo, tg, hi sql.NullFloat64
		if err := rows.Scan(&status, &t.Phase, &t.Measure, &t.Lights, &lo, &tg, &hi); err != nil {
			return cs, err
		}
		t.Stage = stageOf[status]
		if t.Stage == "" || t.Stage == climate.Drying && t.Measure == climate.VPD {
			continue
		}
		t.Low, t.Target, t.High = nullPtr(lo), nullPtr(tg), nullPtr(hi)
		cs.Targets = append(cs.Targets, t)
	}
	return cs, rows.Err()
}

func nullPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

// validateClimate checks a card before saving; it returns an api_ message key.
func validateClimate(cs ClimateSettings) string {
	if cs.PhaseWeeks[0] < 1 || cs.PhaseWeeks[1] <= cs.PhaseWeeks[0] || cs.PhaseWeeks[1] > 30 {
		return "api_climate_bad_weeks"
	}
	inRange := func(m string, p *float64) bool {
		if p == nil {
			return true
		}
		b := climateBounds[m]
		return !math.IsNaN(*p) && *p >= b[0] && *p <= b[1]
	}
	for m, l := range map[string][2]*float64{climate.Temp: cs.TempLimits, climate.RH: cs.RHLimits} {
		if !inRange(m, l[0]) || !inRange(m, l[1]) {
			return "api_climate_out_of_range"
		}
		if l[0] != nil && l[1] != nil && *l[0] >= *l[1] {
			return "api_climate_order"
		}
	}
	known := map[climateKey]bool{}
	for _, k := range climateRows() {
		known[k] = true
	}
	seen := map[climateKey]bool{}
	for _, t := range cs.Targets {
		k := climateKey{t.Stage, t.Phase, t.Measure, t.Lights}
		if !known[k] || seen[k] {
			return "api_invalid_payload"
		}
		seen[k] = true
		if !inRange(t.Measure, t.Low) || !inRange(t.Measure, t.Target) || !inRange(t.Measure, t.High) {
			return "api_climate_out_of_range"
		}
		// A Target sits strictly between its limits, because a reading at a
		// limit starts an alert.
		v := []*float64{t.Low, t.Target, t.High}
		for i := 0; i < 2; i++ {
			for j := i + 1; j < 3; j++ {
				if v[i] != nil && v[j] != nil && *v[i] >= *v[j] {
					return "api_climate_order"
				}
			}
		}
	}
	return ""
}

// SaveClimate stores the card. Rows not in cs.Targets are left as they are.
func SaveClimate(db *sql.DB, cs ClimateSettings) error {
	ids, err := statusIDs(db)
	if err != nil {
		return err
	}
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	for name, value := range map[string]string{
		climateSplitLightsKey:  formatChoices(cs.SplitLights),
		climateFlowerPhasesKey: formatChoices(cs.FlowerPhases),
		climateTempByStageKey:  b(cs.TempByStage),
		climateRHByStageKey:    b(cs.RHByStage),
		climatePhaseWeeksKey:   fmt.Sprintf("%d,%d", cs.PhaseWeeks[0], cs.PhaseWeeks[1]),
		climateTempLimitsKey:   formatLimits(cs.TempLimits),
		climateRHLimitsKey:     formatLimits(cs.RHLimits),
	} {
		if err := UpdateSetting(db, nil, name, value); err != nil {
			return err
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range cs.Targets {
		sid, ok := ids[t.Stage]
		if !ok {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO climate_target (status_id, phase, measure, lights, low, target, high)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (status_id, phase, measure, lights) DO UPDATE SET low = excluded.low, target = excluded.target, high = excluded.high`,
			sid, t.Phase, t.Measure, t.Lights, t.Low, t.Target, t.High); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClimateZonesHandler returns the zones as the Settings page has them, with
// fresh readings, so Zone status can redraw after a Grow setup save.
// Route: GET /settings/climate/zones
func ClimateZonesHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"zones": ClimatePageData(c).Zones})
}

// SaveClimateHandler saves the Climate targets card.
// Route: POST /settings/climate
func SaveClimateHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SaveClimateHandler")
	var cs ClimateSettings
	if err := c.ShouldBindJSON(&cs); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if msg := validateClimate(cs); msg != "" {
		apiBadRequest(c, msg)
		return
	}
	if err := SaveClimate(DBFromContext(c), cs); err != nil {
		fieldLogger.WithError(err).Error("Failed to save climate targets and limits")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	apiOK(c, "api_settings_saved")
}

// byStage says whether a measure has Low / Target / High per stage: VPD
// always, temperature and humidity when Targets and limits by stage is on.
func (cs ClimateSettings) byStage(measure string) bool {
	return measure == climate.VPD || measure == climate.Temp && cs.TempByStage || measure == climate.RH && cs.RHByStage
}

// rangeNow returns the Low, Target and High in use for a measure at a stage:
// its row, or the one pair of limits with by stage off.
func (cs ClimateSettings) rangeNow(measure, stage, phase string, lightsOn bool) climate.Range {
	if !cs.byStage(measure) {
		l := cs.TempLimits
		if measure == climate.RH {
			l = cs.RHLimits
		}
		return climate.Range{Low: l[0], High: l[1]}
	}
	i := measureIndex(measure)
	lights := "on"
	if stage == climate.Drying || !lightsOn && cs.SplitLights[i] {
		lights = "off"
	}
	if stage != climate.Flower || !cs.FlowerPhases[i] {
		phase = ""
	}
	for _, t := range cs.Targets {
		if t.Stage == stage && t.Phase == phase && t.Measure == measure && t.Lights == lights {
			return t.Range
		}
	}
	return climate.Range{}
}

// anyLimit says whether a Low or High limit is set in a row in use.
func (cs ClimateSettings) anyLimit() bool {
	for _, m := range []struct {
		measure string
		flat    [2]*float64
	}{{climate.Temp, cs.TempLimits}, {climate.RH, cs.RHLimits}} {
		if !cs.byStage(m.measure) && (m.flat[0] != nil || m.flat[1] != nil) {
			return true
		}
	}
	for _, t := range cs.Targets {
		i := measureIndex(t.Measure)
		if !cs.byStage(t.Measure) || t.Low == nil && t.High == nil {
			continue
		}
		if t.Lights == "off" && t.Stage != climate.Drying && !cs.SplitLights[i] {
			continue
		}
		if t.Stage == climate.Flower && (t.Phase != "") != cs.FlowerPhases[i] {
			continue
		}
		return true
	}
	return false
}

// ZoneSetup is a zone's climate configuration (Edit zone).
type ZoneSetup struct {
	ID             int              `json:"id"`
	Name           string           `json:"name"`
	Schedule       climate.Schedule `json:"light_schedule"` // nil = no schedule
	LightSensorID  *int             `json:"light_sensor_id"`
	LightOnAbove   *float64         `json:"light_on_above"`
	StagePlantID   *int             `json:"stage_plant_id"`
	TempSensorID   *int             `json:"vpd_temp_sensor_id"`
	HumSensorID    *int             `json:"vpd_humidity_sensor_id"`
	VPDSource      string           `json:"vpd_source"` // "sensor", "calc" or ""
	VPDSensorID    *int             `json:"vpd_sensor_id"`
	LeafTempOffset *float64         `json:"leaf_temp_offset"`
}

func sqlIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

// LoadZoneSetups reads every zone's climate configuration, by zone id.
func LoadZoneSetups(db *sql.DB) (map[int]ZoneSetup, error) {
	rows, err := db.Query(`SELECT id, name, light_schedule, light_sensor_id, light_on_above, stage_plant_id,
		vpd_temp_sensor_id, vpd_humidity_sensor_id, COALESCE(vpd_source, ''), vpd_sensor_id, leaf_temp_offset FROM zones`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]ZoneSetup{}
	for rows.Next() {
		var z ZoneSetup
		var sched sql.NullString
		var light, plant, temp, hum, vpd sql.NullInt64
		var above, leaf sql.NullFloat64
		if err := rows.Scan(&z.ID, &z.Name, &sched, &light, &above, &plant, &temp, &hum, &z.VPDSource, &vpd, &leaf); err != nil {
			return nil, err
		}
		if sched.Valid && sched.String != "" {
			if err := json.Unmarshal([]byte(sched.String), &z.Schedule); err != nil {
				logger.Log.WithError(err).WithField("zone", z.ID).Warn("Unreadable light schedule")
			}
		}
		z.LightSensorID, z.LightOnAbove, z.StagePlantID = sqlIntPtr(light), nullPtr(above), sqlIntPtr(plant)
		z.TempSensorID, z.HumSensorID, z.VPDSensorID = sqlIntPtr(temp), sqlIntPtr(hum), sqlIntPtr(vpd)
		z.LeafTempOffset = nullPtr(leaf)
		out[z.ID] = z
	}
	return out, rows.Err()
}

// zonePlants returns the plants in each zone with their current status.
func zonePlants(db *sql.DB) (map[int][]climate.Plant, error) {
	order := "strftime('%s', l.date) DESC, l.id DESC"
	if model.IsPostgres() {
		order = "l.date DESC, l.id DESC"
	}
	rows, err := db.Query(fmt.Sprintf(`
		SELECT p.id, p.name, p.zone_id, ps.status, ps.status_order, psl.date
		FROM plant p
		JOIN plant_status_log psl ON psl.id = (SELECT l.id FROM plant_status_log l WHERE l.plant_id = p.id ORDER BY %s LIMIT 1)
		JOIN plant_status ps ON ps.id = psl.status_id
		WHERE p.zone_id IS NOT NULL`, order))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int][]climate.Plant{}
	for rows.Next() {
		var p climate.Plant
		var zone int
		if err := rows.Scan(&p.ID, &p.Name, &zone, &p.Status, &p.StatusOrder, &p.Since); err != nil {
			return nil, err
		}
		p.Since = utils.AsLocal(p.Since)
		out[zone] = append(out[zone], p)
	}
	return out, rows.Err()
}

// ZoneClimate is what a zone's climate looks like now.
type ZoneClimate struct {
	ZoneID    int               `json:"zone_id"`
	Stage     climate.ZoneStage `json:"stage"`
	Phase     string            `json:"phase,omitempty"`
	Scheduled bool              `json:"scheduled"`
	LightsOn  bool              `json:"lights_on"`
	// Sensor ids whose readings are compared with targets, by measure.
	Sensors map[string]int `json:"sensors"`
	// The Light check sensor's view: nil without a sensor or reading.
	LightSeen *bool    `json:"light_seen,omitempty"`
	LightRead *float64 `json:"light_read,omitempty"`
}

// derivedVPDSensors maps zone id to its calculated VPD sensor.
func derivedVPDSensors(sensors []ManagedSensor) map[int]int {
	out := map[int]int{}
	for _, s := range sensors {
		if s.Source == "derived" && s.Type == "VPD" {
			if z, err := strconv.Atoi(s.Device); err == nil {
				out[z] = s.ID
			}
		}
	}
	return out
}

// ZoneClimates works out each zone's stage, lights and climate sensors now.
func ZoneClimates(db *sql.DB, cs ClimateSettings, sensors []ManagedSensor, now time.Time, loc *time.Location) (map[int]ZoneClimate, map[int]ZoneSetup, error) {
	setups, err := LoadZoneSetups(db)
	if err != nil {
		return nil, nil, err
	}
	plants, err := zonePlants(db)
	if err != nil {
		return nil, nil, err
	}
	inZone := map[int]int{} // sensor id -> zone id
	byID := map[int]ManagedSensor{}
	for _, s := range sensors {
		inZone[s.ID] = s.ZoneID
		byID[s.ID] = s
	}
	derived := derivedVPDSensors(sensors)
	out := map[int]ZoneClimate{}
	for id, z := range setups {
		zc := ZoneClimate{ZoneID: id, Sensors: map[string]int{}}
		chosen := 0
		if z.StagePlantID != nil {
			chosen = *z.StagePlantID
		}
		zc.Stage = climate.Stage(plants[id], chosen, now, loc)
		if zc.Stage.Stage == climate.Flower {
			zc.Phase = climate.Phase(zc.Stage.Week, cs.PhaseWeeks[0], cs.PhaseWeeks[1])
		}
		zc.LightsOn, zc.Scheduled = climate.LightsOn(z.Schedule, zc.Stage.Stage, now, loc)
		pick := func(measure string, sid *int) {
			if sid != nil && inZone[*sid] == id {
				zc.Sensors[measure] = *sid
			}
		}
		pick(climate.Temp, z.TempSensorID)
		pick(climate.RH, z.HumSensorID)
		switch z.VPDSource {
		case "sensor":
			pick(climate.VPD, z.VPDSensorID)
		case "calc":
			if d, ok := derived[id]; ok {
				zc.Sensors[climate.VPD] = d
			}
		}
		if z.LightSensorID != nil && inZone[*z.LightSensorID] == id {
			if s := byID[*z.LightSensorID]; s.Value != nil && !s.Stale {
				above := 0.0
				if z.LightOnAbove != nil {
					above = *z.LightOnAbove
				}
				seen := *s.Value > above
				zc.LightSeen, zc.LightRead = &seen, s.Value
			}
		}
		out[id] = zc
	}
	return out, setups, nil
}

// ClimateCheck is how one sensor's readings are colored: the Low, Target and
// High in use now, in the sensor's own unit, and its state now.
type ClimateCheck struct {
	State climate.State `json:"state"`
	Range climate.Range `json:"range"`
	Near  float64       `json:"near"` // how close to Target is near on a side with no limit
}

func cToF(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p*9/5 + 32
	return &v
}

// check works out a sensor's ClimateCheck. Temperatures are compared in °C
// and returned in the sensor's own unit.
func (cs ClimateSettings) check(zc ZoneClimate, measure string, s ManagedSensor) ClimateCheck {
	var ck ClimateCheck
	if zc.Stage.Stage == "" {
		return ck
	}
	ck.Range = cs.rangeNow(measure, zc.Stage.Stage, zc.Phase, zc.LightsOn)
	ck.Near = climate.Near[measure]
	if s.Value != nil && !s.Stale {
		v := *s.Value
		if measure == climate.Temp && !utils.IsCelsius(s.Unit) {
			v = utils.FahrenheitToCelsius(v)
		}
		ck.State = climate.Band(measure, v, ck.Range)
	}
	if measure == climate.Temp && !utils.IsCelsius(s.Unit) {
		ck.Range = climate.Range{Low: cToF(ck.Range.Low), Target: cToF(ck.Range.Target), High: cToF(ck.Range.High)}
		ck.Near = ck.Near * 9 / 5
	}
	return ck
}

// ClimateChecks covers every zone's chosen climate sensors, by sensor id.
func ClimateChecks(db *sql.DB, sensors []ManagedSensor, loc *time.Location) map[int]ClimateCheck {
	out := map[int]ClimateCheck{}
	cs, err := LoadClimate(db)
	if err != nil {
		logger.Log.WithError(err).Warn("Climate targets unavailable")
		return out
	}
	zones, _, err := ZoneClimates(db, cs, sensors, time.Now(), loc)
	if err != nil {
		logger.Log.WithError(err).Warn("Zone climate unavailable")
		return out
	}
	byID := map[int]ManagedSensor{}
	for _, s := range sensors {
		byID[s.ID] = s
	}
	for _, zc := range zones {
		for measure, sid := range zc.Sensors {
			out[sid] = cs.check(zc, measure, byID[sid])
		}
	}
	return out
}

// climateSensorOption is a sensor offered in Edit zone, with its latest reading.
type climateSensorOption struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Unit   string   `json:"unit"`
	Type   string   `json:"type"`
	Source string   `json:"source"`
	Value  *float64 `json:"value"`
	Stale  bool     `json:"stale"`
}

// climateReading is a zone's latest climate reading; temperature in °C.
type climateReading struct {
	Name  string   `json:"name"`
	Value *float64 `json:"value"`
	Stale bool     `json:"stale"`
}

// climatePageZone is one zone on Settings > Grow setup and Alerts' Zone status.
type climatePageZone struct {
	ZoneSetup
	Now      ZoneClimate               `json:"now"`
	Options  []climateSensorOption     `json:"sensor_options"`
	Readings map[string]climateReading `json:"readings"`
	Light    *zoneLightStatus          `json:"light,omitempty"`
	Alerts   []zoneAlert               `json:"alerts"`
	// The chosen sensors' oldest latest reading, for Zone status.
	LastAt    int64  `json:"last_at"` // epoch ms; 0 = none yet
	LastClock string `json:"last_clock,omitempty"`
	Offline   bool   `json:"offline"`
}

// zoneAlert is an open alert as Zone status shows it. Times are already
// formatted for the page.
type zoneAlert struct {
	Kind     string `json:"kind"`
	Measure  string `json:"measure"`
	Side     string `json:"side"`
	Delivery string `json:"delivery"`
	Sent     string `json:"sent"`
	Next     string `json:"next"`
	Since    string `json:"since,omitempty"`   // offline alerts: the last reading before they stopped
	Sensors  string `json:"sensors,omitempty"` // offline alerts: the sensors that stopped
}

// zoneLightStatus is what Edit zone's Light check says when the saved light
// sensor and schedule disagree. Times are already formatted for the page.
type zoneLightStatus struct {
	Mismatch  bool   `json:"mismatch"`
	SeenOnNow bool   `json:"seen_on_now"`
	Since     string `json:"since,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Alert     string `json:"alert,omitempty"` // the open alert's delivery state
	LastFrom  string `json:"last_from,omitempty"`
	LastTo    string `json:"last_to,omitempty"`
	Clock     bool   `json:"clock"`
	Early     bool   `json:"early"`
	Minutes   int    `json:"minutes,omitempty"`
	SeenOn    string `json:"seen_on,omitempty"`
	SeenOff   string `json:"seen_off,omitempty"`
	SchedOn   string `json:"sched_on,omitempty"`
	SchedOff  string `json:"sched_off,omitempty"`
}

// zoneLight works out a zone's Light check message, or nil when all is well
// or there's nothing to compare. It's the same check the alerts use.
func zoneLight(db *sql.DB, z ZoneSetup, zc ZoneClimate, unit string, open []alerts.Event, now time.Time, loc *time.Location) *zoneLightStatus {
	if z.LightSensorID == nil || !zc.Scheduled || zc.Stage.Stage == "" {
		return nil
	}
	readings, err := recentReadings(db, *z.LightSensorID, unit, "", now.Add(-climate.ClockOffsetLookback))
	if err != nil || len(readings) == 0 {
		return nil
	}
	above := 0.0
	if z.LightOnAbove != nil {
		above = *z.LightOnAbove
	}
	clock := func(t time.Time) string { return t.In(loc).Format("3:04 PM") }
	stage := zc.Stage.Stage
	if s := climate.ClockOffset(readings, above, z.Schedule, stage, now, loc); s != nil {
		times := z.Schedule[stage]
		on, _ := time.Parse("15:04", times[0])
		off, _ := time.Parse("15:04", times[1])
		return &zoneLightStatus{Clock: true, Early: s.Early, Minutes: s.Minutes, SeenOn: clock(s.SeenOn), SeenOff: clock(s.SeenOff),
			SchedOn: on.Format("3:04 PM"), SchedOff: off.Format("3:04 PM")}
	}
	m, _ := climate.CheckLights(readings, above, z.Schedule, stage, now, loc, alerts.LightHold)
	if m == nil {
		return nil
	}
	mins := int(now.Sub(m.Since).Minutes())
	st := &zoneLightStatus{Mismatch: true, SeenOnNow: m.SeenOn, Since: clock(m.Since), Duration: fmt.Sprintf("%d h %d min", mins/60, mins%60)}
	// The last stretch the sensor saw the lights in the other state.
	runs := climate.LightRuns(readings, above)
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].On != m.SeenOn {
			st.LastFrom, st.LastTo = clock(runs[i].From), clock(runs[i].To)
			break
		}
	}
	for _, o := range open {
		if o.Kind == alerts.KindLight && o.ZoneID == z.ID {
			st.Alert = o.Delivery
		}
	}
	return st
}

// ClimatePage is the data behind Climate targets and limits, Edit zone and Zone status.
type ClimatePage struct {
	Settings ClimateSettings   `json:"settings"`
	Units    string            `json:"units"`
	Zones    []climatePageZone `json:"zones"`
	Clock    string            `json:"clock"`   // BudMinder time now, e.g. "9:42 PM"
	Minutes  int               `json:"minutes"` // ...as minutes after midnight
	TimeZone string            `json:"time_zone"`
}

// ClimatePageData builds ClimatePage for the Settings page.
func ClimatePageData(c *gin.Context) ClimatePage {
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)
	loc := requestLocation(c)
	page := ClimatePage{Units: store.Units(), Zones: []climatePageZone{}, TimeZone: loc.String()}
	if page.TimeZone == "Local" {
		page.TimeZone = ""
	}
	local := time.Now().In(loc)
	page.Clock = local.Format("3:04 PM")
	page.Minutes = local.Hour()*60 + local.Minute()
	cs, err := LoadClimate(db)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load climate targets and limits")
	}
	page.Settings = cs
	sensors := ListManagedSensors(db, store.PollingInterval())
	zones, setups, err := ZoneClimates(db, cs, sensors, time.Now(), loc)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load zone climate")
		return page
	}
	open, err := loadOpenAlerts(db)
	if err != nil {
		logger.Log.WithError(err).Warn("Failed to read open alerts")
	}
	unitOf := map[int]string{}
	byID := map[int]ManagedSensor{}
	for _, s := range sensors {
		unitOf[s.ID] = s.Unit
		byID[s.ID] = s
	}
	for id, z := range setups {
		pz := climatePageZone{ZoneSetup: z, Now: zones[id], Options: []climateSensorOption{}, Readings: map[string]climateReading{}, Alerts: []zoneAlert{}}
		for _, o := range open {
			if o.ZoneID == id && o.Kind != alerts.KindSensor {
				za := zoneAlert{Kind: o.Kind, Measure: o.Measure, Side: o.Side, Delivery: o.Delivery,
					Sent: o.Notified.In(loc).Format("3:04 PM"), Next: o.Notified.Add(alerts.Remind).In(loc).Format("3:04 PM"),
					Since: clockOrDate(o.Started.Add(-alerts.OfflineAfter), loc)}
				if o.Kind == alerts.KindOffline {
					za.Sensors = o.Detail
				}
				pz.Alerts = append(pz.Alerts, za)
			}
		}
		pz.lastReading(byID, loc)
		if z.LightSensorID != nil {
			pz.Light = zoneLight(db, z, zones[id], unitOf[*z.LightSensorID], open, time.Now(), loc)
		}
		for measure, sid := range pz.Now.Sensors {
			for _, s := range sensors {
				if s.ID != sid {
					continue
				}
				r := climateReading{Name: s.Name, Value: s.Value, Stale: s.Stale}
				if measure == climate.Temp && s.Value != nil && !utils.IsCelsius(s.Unit) {
					v := utils.FahrenheitToCelsius(*s.Value)
					r.Value = &v
				}
				pz.Readings[measure] = r
			}
		}
		for _, s := range sensors {
			if s.ZoneID == id && s.Source != "derived" {
				pz.Options = append(pz.Options, climateSensorOption{ID: s.ID, Name: s.Name, Unit: s.Unit, Type: s.Type, Source: s.Source, Value: s.Value, Stale: s.Stale})
			}
		}
		page.Zones = append(page.Zones, pz)
	}
	sort.Slice(page.Zones, func(i, j int) bool { return strings.ToLower(page.Zones[i].Name) < strings.ToLower(page.Zones[j].Name) })
	return page
}

// clockOrDate formats a time for Zone status, with its date unless it's today.
func clockOrDate(t time.Time, loc *time.Location) string {
	if t.In(loc).Format("2006-01-02") == time.Now().In(loc).Format("2006-01-02") {
		return t.In(loc).Format("3:04 PM")
	}
	return t.In(loc).Format("Jan 2, 3:04 PM")
}

// lastReading fills in how recent the zone's chosen sensors' readings are.
func (pz *climatePageZone) lastReading(byID map[int]ManagedSensor, loc *time.Location) {
	ids := make([]int, 0, 4)
	for _, sid := range pz.Now.Sensors {
		ids = append(ids, sid)
	}
	if pz.LightSensorID != nil {
		ids = append(ids, *pz.LightSensorID)
	}
	for _, sid := range ids {
		s := byID[sid]
		if s.LastAt != 0 && s.Source != "derived" && (pz.LastAt == 0 || s.LastAt < pz.LastAt) {
			pz.LastAt = s.LastAt
		}
	}
	if pz.LastAt != 0 {
		last := time.UnixMilli(pz.LastAt)
		pz.LastClock = clockOrDate(last, loc)
		pz.Offline = time.Since(last) >= alerts.OfflineAfter
	}
}
