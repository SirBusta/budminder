package handlers

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"
	"time"

	"isley/logger"
	"isley/utils"
)

// A zone whose climate limits are never checked: it has plants and
// temperature, humidity or VPD sensors, but none is chosen as a climate
// sensor in Edit zone. Settings (Grow setup, Alerts) and the Today page warn
// about it in amber. The page scripts work this out again from the same
// facts (climatePageZone.Choosable, its chosen sensors and the limits in use),
// so edits on the Climate card show at once.

var (
	climateKindBattery = regexp.MustCompile(`(?i)battery`)
	climateKindLevel   = regexp.MustCompile(`(?i)water level`)
	climateKindLight   = regexp.MustCompile(`(?i)illuminance|\blux\b`)
	climateKindTempU   = regexp.MustCompile(`^(celsius|fahrenheit|degc|degf|℃|℉)$`)
	climateKindTemp    = regexp.MustCompile(`(?i)temp`)
	climateKindVPD     = regexp.MustCompile(`(?i)vpd`)
	climateKindCO2     = regexp.MustCompile(`(?i)co2|co₂`)
	climateKindEC      = regexp.MustCompile(`(?i)ms/cm|\.ec$`)
	climateKindSoil    = regexp.MustCompile(`(?i)soil|moist`)
	climateKindHum     = regexp.MustCompile(`(?i)hum`)
)

// climateKind says whether a sensor reads temperature ("temp"), humidity
// ("hum") or VPD ("vpd"), or "" for anything else. It follows SensorKinds.kind
// in web/static/js/sensor-kinds.js, which Edit zone uses to fill its pickers.
func climateKind(typ, name, unit string) string {
	switch k := sensorKind(typ, name, unit); k {
	case "temp", "hum", "vpd":
		return k
	}
	return ""
}

// sensorKind is SensorKinds.kind (web/static/js/sensor-kinds.js): port, other,
// level, light, temp, vpd, co2, ec, soil or hum. Keep the two in step.
func sensorKind(typ, name, unit string) string {
	if isPortType(typ) {
		return "port"
	}
	u := strings.ToLower(unit)
	ty := typ + " " + name
	switch {
	case climateKindBattery.MatchString(ty):
		return "other"
	case typ == "VS.waterLv" || climateKindLevel.MatchString(name):
		return "level"
	case u == "lx" || climateKindLight.MatchString(ty):
		return "light"
	case strings.Contains(u, "°") || climateKindTempU.MatchString(u) || climateKindTemp.MatchString(ty) && !strings.Contains(u, "%"):
		return "temp"
	case u == "kpa" || climateKindVPD.MatchString(ty):
		return "vpd"
	case u == "ppm" || climateKindCO2.MatchString(ty):
		return "co2"
	case climateKindEC.MatchString(u + " " + typ):
		return "ec"
	case climateKindSoil.MatchString(ty):
		return "soil"
	case u == "%" || climateKindHum.MatchString(ty):
		return "hum"
	}
	return "other"
}

// sensorDecimals is SensorKinds.decimals: how many decimals a reading shows.
func sensorDecimals(kind string) int {
	switch kind {
	case "vpd", "ec":
		return 2
	case "co2", "level", "light", "port":
		return 0
	}
	return 1
}

// sensorDisplay is SensorKinds.display: the unit a sensor's readings show in
// and the conversion from the stored value (°C <-> °F follows Settings >
// Measurement units; stored values never change).
func sensorDisplay(kind, unit, units string) (string, func(float64) float64) {
	same := func(v float64) float64 { return v }
	if kind != "temp" {
		return unit, same
	}
	imperial := utils.NormalizeUnits(units) != utils.UnitsMetric
	lu := strings.ToLower(unit)
	if imperial && strings.Contains(lu, "c") {
		return "°F", func(v float64) float64 { return v*9/5 + 32 }
	}
	if !imperial && strings.Contains(lu, "f") {
		return "°C", func(v float64) float64 { return (v - 32) * 5 / 9 }
	}
	return unit, same
}

// climateChoosable says whether a zone has a sensor Edit zone offers as a
// climate sensor (calculated VPD sensors aren't offered).
func climateChoosable(zoneID int, sensors []ManagedSensor) bool {
	for _, s := range sensors {
		if s.ZoneID == zoneID && s.Source != "derived" && climateKind(s.Type, s.Name, s.Unit) != "" {
			return true
		}
	}
	return false
}

// UncheckedZone is a zone that gets no climate alerts because no climate
// sensor is chosen in Edit zone.
type UncheckedZone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ClimateUnchecked lists the zones, sorted by name, whose climate limits are
// never checked: plants in the zone, sensors to choose from, none chosen,
// and at least one Low or High limit in use (without one no zone is checked).
func ClimateUnchecked(db *sql.DB, sensors []ManagedSensor, loc *time.Location) []UncheckedZone {
	out := []UncheckedZone{}
	cs, err := LoadClimate(db)
	if err != nil {
		logger.Log.WithError(err).Warn("Climate targets unavailable")
		return out
	}
	if !cs.anyLimit() {
		return out
	}
	zones, setups, err := ZoneClimates(db, cs, sensors, time.Now(), loc)
	if err != nil {
		logger.Log.WithError(err).Warn("Zone climate unavailable")
		return out
	}
	for id, zc := range zones {
		if zc.Stage.Stage == "" || len(zc.Sensors) > 0 || !climateChoosable(id, sensors) {
			continue
		}
		out = append(out, UncheckedZone{ID: id, Name: setups[id].Name})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
