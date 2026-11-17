package handlers

// Tools > VPD chart (built 2026-11-17 from mockup data/mockups/tools/vpd/v3.html).
// A leaf VPD chart with fixed color ranges per stage, drawn by
// web/static/js/tools-vpd.js. The ranges never follow Grow setup's Targets
// and the chart sends no alerts; this file only supplies each zone's stage,
// current air temperature and humidity, and leaf temperature offset.

import (
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/climate"
	"isley/logger"
	"isley/utils"
)

// vpdDefaultOffsetF is the leaf temperature offset (°F) used for a zone that
// has none saved: Edit Zone keeps one only when VPD is calculated, and offers
// −2 °F there too.
const vpdDefaultOffsetF = -2.0

// vpdZone is one zone in the VPD chart's Zone menu.
type vpdZone struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Stage   string   `json:"stage"`           // climate stage key; "" with no plants
	Week    int      `json:"week,omitempty"`  // week of the stage
	Phase   string   `json:"phase,omitempty"` // Flower phase, when VPD splits Flower
	TempC   *float64 `json:"temp_c"`          // latest air temperature, °C; nil = none recent
	RH      *float64 `json:"rh"`              // latest humidity, %
	OffsetF *float64 `json:"offset_f"`        // the zone's own leaf temperature offset, °F
}

// vpdTextKeys are the translations the page script needs besides vp_*.
var vpdTextKeys = []string{"seedling_label", "veg_label", "flower_label", "drying_label",
	"cl_stretch", "cl_bloom", "cl_ripen", "cl_week_n", "edit_zone"}

// VPDChartPageData builds the VPD chart's page data.
func VPDChartPageData(c *gin.Context, translations map[string]string) gin.H {
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)
	t := map[string]string{}
	for k, v := range translations {
		if strings.HasPrefix(k, "vp_") {
			t[k] = v
		}
	}
	for _, k := range vpdTextKeys {
		t[k] = translations[k]
	}
	data := gin.H{"units": utils.NormalizeUnits(store.Units()), "default_offset_f": vpdDefaultOffsetF, "t": t}
	zones := []vpdZone{}
	data["zones"] = zones

	cs, err := LoadClimate(db)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load climate settings for the VPD chart")
	}
	sensors := ListManagedSensors(db, store.PollingInterval())
	now, setups, err := ZoneClimates(db, cs, sensors, time.Now(), requestLocation(c))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load zones for the VPD chart")
		return data
	}
	byID := map[int]ManagedSensor{}
	for _, s := range sensors {
		byID[s.ID] = s
	}
	reading := func(zc ZoneClimate, measure string) *float64 {
		sid, ok := zc.Sensors[measure]
		if !ok {
			return nil
		}
		s := byID[sid]
		if s.Value == nil || s.Stale {
			return nil
		}
		v := *s.Value
		if measure == climate.Temp && !utils.IsCelsius(s.Unit) {
			v = utils.FahrenheitToCelsius(v)
		}
		return &v
	}
	for id, z := range setups {
		zc := now[id]
		vz := vpdZone{ID: id, Name: z.Name, Stage: zc.Stage.Stage, Week: zc.Stage.Week, OffsetF: z.LeafTempOffset}
		if zc.Stage.Stage == climate.Flower && cs.FlowerPhases[measureIndex(climate.VPD)] {
			vz.Phase = zc.Phase
		}
		vz.TempC, vz.RH = reading(zc, climate.Temp), reading(zc, climate.RH)
		zones = append(zones, vz)
	}
	sort.Slice(zones, func(i, j int) bool { return strings.ToLower(zones[i].Name) < strings.ToLower(zones[j].Name) })
	data["zones"] = zones
	return data
}
