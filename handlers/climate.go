package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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

// Climate settings keys (Settings > Grow setup > Climate targets).
const (
	climateSplitLightsKey  = "climate_split_lights"
	climateFlowerPhasesKey = "climate_flower_phases"
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

// ClimateSettings is the whole Climate targets card.
type ClimateSettings struct {
	SplitLights  bool            `json:"split_lights"`
	FlowerPhases bool            `json:"flower_phases"`
	PhaseWeeks   [2]int          `json:"phase_weeks"` // last week of Stretch, last week of Full bloom
	TempByStage  bool            `json:"temp_by_stage"`
	RHByStage    bool            `json:"rh_by_stage"`
	TempLimits   [2]*float64     `json:"temp_limits"` // °C; nil = skip
	RHLimits     [2]*float64     `json:"rh_limits"`
	Targets      []ClimateTarget `json:"targets"`
}

// climateKey identifies a target row.
type climateKey struct{ stage, phase, measure, lights string }

// climateRows lists every row the card has: each measure for Seedling, Veg,
// Flower (whole and each phase) with lights on and off, and Drying (lights off).
func climateRows() []climateKey {
	var out []climateKey
	for _, m := range []string{climate.VPD, climate.Temp, climate.RH} {
		for _, sp := range [][2]string{{climate.Seedling, ""}, {climate.Veg, ""}, {climate.Flower, ""},
			{climate.Flower, climate.Stretch}, {climate.Flower, climate.Bloom}, {climate.Flower, climate.Ripen}} {
			for _, l := range []string{"on", "off"} {
				out = append(out, climateKey{sp[0], sp[1], m, l})
			}
		}
		out = append(out, climateKey{climate.Drying, "", m, "off"})
	}
	return out
}

// Allowed values per measure (temperature in °C).
var climateBounds = map[string][2]float64{climate.VPD: {0, 5}, climate.Temp: {-20, 60}, climate.RH: {0, 100}}

func fToC(f float64) float64 { return math.Round((f-32)*5/9*10000) / 10000 }

// Example values from the Climate targets design: [lights on, lights off]
// as [low, target, high]; temperature in °F, converted to °C when stored.
var climateDefaults = map[string]map[string][2][3]float64{
	climate.VPD: {
		"seedling": {{0.4, 0.6, 0.8}, {0.3, 0.5, 0.7}}, "veg": {{0.8, 1.0, 1.2}, {0.6, 0.8, 1.0}},
		"flower": {{1.0, 1.25, 1.5}, {0.8, 1.05, 1.3}}, "stretch": {{1.0, 1.2, 1.4}, {0.8, 1.0, 1.2}},
		"bloom": {{1.2, 1.35, 1.5}, {1.0, 1.15, 1.3}}, "ripen": {{1.3, 1.45, 1.6}, {1.1, 1.25, 1.4}},
		"drying": {{0.7, 0.8, 0.9}, {0.7, 0.8, 0.9}}},
	climate.Temp: {
		"seedling": {{72, 77, 82}, {66, 70, 74}}, "veg": {{72, 78, 84}, {64, 70, 75}},
		"flower": {{68, 75, 82}, {60, 66, 72}}, "stretch": {{70, 77, 82}, {62, 68, 72}},
		"bloom": {{68, 75, 80}, {60, 66, 70}}, "ripen": {{65, 72, 78}, {58, 64, 68}},
		"drying": {{58, 60, 64}, {58, 60, 64}}},
	climate.RH: {
		"seedling": {{60, 68, 75}, {60, 68, 75}}, "veg": {{50, 60, 70}, {50, 60, 70}},
		"flower": {{40, 50, 60}, {40, 50, 58}}, "stretch": {{45, 52, 60}, {45, 52, 60}},
		"bloom": {{40, 47, 55}, {40, 47, 55}}, "ripen": {{35, 42, 50}, {35, 42, 48}},
		"drying": {{55, 60, 65}, {55, 60, 65}}},
}

// The VPD ranges migration 019 seeded; an install that still has them gets
// the fuller design defaults instead of a copy.
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
// after migration 043, and after restoring a backup made before it. VPD keeps
// each stage's old Low and High (Target halfway) unless they were never
// changed from the 019 defaults.
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
		key := k.stage
		if k.phase != "" {
			key = k.phase
		}
		d := climateDefaults[k.measure][key][0]
		if k.lights == "off" {
			d = climateDefaults[k.measure][key][1]
		}
		lo, t, hi := d[0], d[1], d[2]
		if k.measure == climate.Temp {
			lo, t, hi = fToC(lo), fToC(t), fToC(hi)
		}
		if k.measure == climate.VPD {
			name := climate.StageStatus[k.stage]
			if r, ok := old[name]; ok && r[0] != nil && r[1] != nil {
				if def, had := vpd019[name]; !had || *r[0] != def[0] || *r[1] != def[1] {
					lo, hi = *r[0], *r[1]
					t = math.Round((lo+hi)/2*1000) / 1000
				}
			}
		}
		if _, err := tx.Exec(`INSERT INTO climate_target (status_id, phase, measure, lights, low, target, high)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, sid, k.phase, k.measure, k.lights, lo, t, hi); err != nil {
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

func formatLimits(l [2]*float64) string {
	s := func(p *float64) string {
		if p == nil {
			return ""
		}
		return strconv.FormatFloat(*p, 'f', -1, 64)
	}
	return s(l[0]) + "," + s(l[1])
}

// LoadClimate reads the Climate targets card.
func LoadClimate(db *sql.DB) (ClimateSettings, error) {
	cs := ClimateSettings{
		SplitLights:  settingBool(db, climateSplitLightsKey, true),
		FlowerPhases: settingBool(db, climateFlowerPhasesKey, false),
		TempByStage:  settingBool(db, climateTempByStageKey, false),
		RHByStage:    settingBool(db, climateRHByStageKey, false),
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
	v, _ := GetSetting(db, climateTempLimitsKey)
	cs.TempLimits = parseLimits(v)
	v, _ = GetSetting(db, climateRHLimitsKey)
	cs.RHLimits = parseLimits(v)

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
		if t.Stage == "" {
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
		if l[0] != nil && l[1] != nil && *l[0] > *l[1] {
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
		v := []*float64{t.Low, t.Target, t.High}
		for i := 0; i < 2; i++ {
			for j := i + 1; j < 3; j++ {
				if v[i] != nil && v[j] != nil && *v[i] > *v[j] {
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
		climateSplitLightsKey:  b(cs.SplitLights),
		climateFlowerPhasesKey: b(cs.FlowerPhases),
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
		fieldLogger.WithError(err).Error("Failed to save climate targets")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	apiOK(c, "api_settings_saved")
}

// byStage says whether a measure has Low / Target / High per stage: VPD
// always, temperature and humidity when Targets by stage is on.
func (cs ClimateSettings) byStage(measure string) bool {
	return measure == climate.VPD || measure == climate.Temp && cs.TempByStage || measure == climate.RH && cs.RHByStage
}

// target returns the range in use for a measure at a stage.
func (cs ClimateSettings) target(measure, stage, phase string, lightsOn bool) climate.Range {
	lights := "on"
	if stage == climate.Drying || !lightsOn && cs.SplitLights {
		lights = "off"
	}
	if stage != climate.Flower || !cs.FlowerPhases {
		phase = ""
	}
	for _, t := range cs.Targets {
		if t.Stage == stage && t.Phase == phase && t.Measure == measure && t.Lights == lights {
			return t.Range
		}
	}
	return climate.Range{}
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

// ClimateCheck is how one sensor's readings are colored: the stage range and
// safety limits in use now, in the sensor's own unit, and its state now.
type ClimateCheck struct {
	State  climate.State `json:"state"`
	Range  climate.Range `json:"range"`  // empty for limits only
	Limits [2]*float64   `json:"limits"` // temperature and humidity safety limits
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
	byStage := cs.byStage(measure)
	if byStage {
		ck.Range = cs.target(measure, zc.Stage.Stage, zc.Phase, zc.LightsOn)
	}
	switch measure {
	case climate.Temp:
		ck.Limits = cs.TempLimits
	case climate.RH:
		ck.Limits = cs.RHLimits
	}
	if s.Value != nil && !s.Stale {
		v := *s.Value
		if measure == climate.Temp && !utils.IsCelsius(s.Unit) {
			v = utils.FahrenheitToCelsius(v)
		}
		ck.State = climate.Limits(v, ck.Limits[0], ck.Limits[1])
		if byStage {
			ck.State = climate.Worse(ck.State, climate.Band(v, ck.Range))
		}
	}
	if measure == climate.Temp && !utils.IsCelsius(s.Unit) {
		ck.Range = climate.Range{Low: cToF(ck.Range.Low), Target: cToF(ck.Range.Target), High: cToF(ck.Range.High)}
		ck.Limits = [2]*float64{cToF(ck.Limits[0]), cToF(ck.Limits[1])}
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

// climatePageZone is one zone on Settings > Grow setup.
type climatePageZone struct {
	ZoneSetup
	Now      ZoneClimate               `json:"now"`
	Options  []climateSensorOption     `json:"sensor_options"`
	Readings map[string]climateReading `json:"readings"`
	Light    *zoneLightStatus          `json:"light,omitempty"`
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

// ClimatePage is the data behind Climate targets and Edit zone.
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
		logger.Log.WithError(err).Error("Failed to load climate targets")
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
	for _, s := range sensors {
		unitOf[s.ID] = s.Unit
	}
	for id, z := range setups {
		pz := climatePageZone{ZoneSetup: z, Now: zones[id], Options: []climateSensorOption{}, Readings: map[string]climateReading{}}
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
