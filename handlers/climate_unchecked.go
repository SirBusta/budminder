package handlers

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"
	"time"

	"isley/logger"
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
	if isPortType(typ) {
		return ""
	}
	u := strings.ToLower(unit)
	ty := typ + " " + name
	switch {
	case climateKindBattery.MatchString(ty):
		return ""
	case typ == "VS.waterLv" || climateKindLevel.MatchString(name):
		return ""
	case u == "lx" || climateKindLight.MatchString(ty):
		return ""
	case strings.Contains(u, "°") || climateKindTempU.MatchString(u) || climateKindTemp.MatchString(ty) && !strings.Contains(u, "%"):
		return "temp"
	case u == "kpa" || climateKindVPD.MatchString(ty):
		return "vpd"
	case u == "ppm" || climateKindCO2.MatchString(ty):
		return ""
	case climateKindEC.MatchString(u + " " + typ):
		return ""
	case climateKindSoil.MatchString(ty):
		return ""
	case u == "%" || climateKindHum.MatchString(ty):
		return "hum"
	}
	return ""
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
