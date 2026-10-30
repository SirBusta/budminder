// Package climate holds the rules behind Settings > Grow setup's Climate
// targets and each zone's light schedule: which stage a zone is in, whether
// its lights are on, and how a reading compares with its Low / Target / High.
// It does no database work; handlers/climate.go loads the data.
package climate

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Measures with targets.
const (
	VPD  = "vpd"
	Temp = "temp" // stored in °C
	RH   = "rh"
)

// Stage keys used for targets and light schedules.
const (
	Seedling = "seedling"
	Veg      = "veg"
	Flower   = "flower"
	Drying   = "drying"
)

// Flower phases (phase "" is the whole stage).
const (
	Stretch = "stretch"
	Bloom   = "bloom"
	Ripen   = "ripen"
)

// StageStatus maps a stage key to the plant_status row its targets hang on.
var StageStatus = map[string]string{Seedling: "Seedling", Veg: "Veg", Flower: "Flower", Drying: "Drying"}

// StageOf maps a plant status to the stage whose targets apply. Germinating
// and Planted count as Seedling, Curing as Drying; Success, Dead and unknown
// statuses have none.
func StageOf(status string) string {
	switch status {
	case "Germinating", "Planted", "Seedling":
		return Seedling
	case "Veg":
		return Veg
	case "Flower":
		return Flower
	case "Drying", "Curing":
		return Drying
	}
	return ""
}

// State of a reading against its targets.
type State string

const (
	None    State = ""        // no color
	OK      State = "ok"      // near Target
	Toward  State = "toward"  // between Target and Low or High
	Reached State = "reached" // reached Low or High
)

// Range is one Low / Target / High set; nil means not set.
type Range struct {
	Low    *float64 `json:"low"`
	Target *float64 `json:"target"`
	High   *float64 `json:"high"`
}

// Band compares v with r. Reaching Low or High counts (inclusive). Near
// Target is within half the distance from Target to that side's limit.
// Without a Target only Low and High are checked.
func Band(v float64, r Range) State {
	if r.Low == nil && r.High == nil && r.Target == nil {
		return None
	}
	if r.Low != nil && v <= *r.Low || r.High != nil && v >= *r.High {
		return Reached
	}
	if r.Target == nil {
		return OK
	}
	t := *r.Target
	limit := r.High
	if v < t {
		limit = r.Low
	}
	if limit == nil {
		return OK
	}
	d := v - t
	if d < 0 {
		d = -d
	}
	gap := *limit - t
	if gap < 0 {
		gap = -gap
	}
	if d <= gap/2+1e-9 { // float noise: exactly half way counts as near
		return OK
	}
	return Toward
}

// Limits checks a safety limit pair: only reaching one shows (red).
func Limits(v float64, low, high *float64) State {
	if low != nil && v <= *low || high != nil && v >= *high {
		return Reached
	}
	return None
}

// Worse returns the more serious of two states.
func Worse(a, b State) State {
	rank := map[State]int{None: 0, OK: 1, Toward: 2, Reached: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// Plant is one plant in a zone, with its current status.
type Plant struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	StatusOrder int       `json:"-"`
	Since       time.Time `json:"-"` // when the current status started (wall clock)
	Week        int       `json:"week,omitempty"`
}

// ZoneStage is the stage a zone's targets follow.
type ZoneStage struct {
	Stage  string  `json:"stage"`            // stage key; "" when there are no plants with a stage
	Week   int     `json:"week,omitempty"`   // week in the stage, 1-based
	Plant  *Plant  `json:"plant,omitempty"`  // the plant followed
	Mixed  bool    `json:"mixed"`            // plants are in different stages
	Left   bool    `json:"left"`             // the chosen plant left the zone
	Plants []Plant `json:"plants,omitempty"` // plants that count, furthest along first
}

// WeekIn returns the 1-based week of a stage that started at since, today
// being now in loc. since is a wall-clock date.
func WeekIn(since, now time.Time, loc *time.Location) int {
	if loc != nil {
		now = now.In(loc)
	}
	y, m, d := since.Date()
	a := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	y, m, d = now.Date()
	b := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	days := int(b.Sub(a).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return days/7 + 1
}

// Stage works out which stage a zone follows. Plants with no stage (Success,
// Dead) don't count; Drying and Curing plants count only when no plant is
// still growing. The furthest along wins (highest status order, then the
// longest in that stage), unless stages differ and chosen is still here.
func Stage(plants []Plant, chosen int, now time.Time, loc *time.Location) ZoneStage {
	var growing, drying []Plant
	for _, p := range plants {
		switch StageOf(p.Status) {
		case "":
		case Drying:
			p.Week = WeekIn(p.Since, now, loc)
			drying = append(drying, p)
		default:
			p.Week = WeekIn(p.Since, now, loc)
			growing = append(growing, p)
		}
	}
	pool := growing
	if len(pool) == 0 {
		pool = drying
	}
	var zs ZoneStage
	if len(pool) == 0 {
		zs.Left = chosen != 0
		return zs
	}
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].StatusOrder != pool[j].StatusOrder {
			return pool[i].StatusOrder > pool[j].StatusOrder
		}
		return pool[i].Since.Before(pool[j].Since)
	})
	for _, p := range pool[1:] {
		if StageOf(p.Status) != StageOf(pool[0].Status) {
			zs.Mixed = true
		}
	}
	follow := pool[0]
	if chosen != 0 {
		found := false
		for _, p := range pool {
			if p.ID == chosen {
				found = true
				if zs.Mixed {
					follow = p
				}
			}
		}
		// A chosen plant that moved on to Drying while others still grow is
		// as good as gone: the zone follows the growing plants.
		zs.Left = !found
	}
	zs.Plants = pool
	zs.Stage = StageOf(follow.Status)
	zs.Week = follow.Week
	zs.Plant = &follow
	return zs
}

// Phase returns the Flower phase for a week, given the last weeks of Stretch
// and Full bloom.
func Phase(week, stretchEnd, bloomEnd int) string {
	switch {
	case week <= stretchEnd:
		return Stretch
	case week <= bloomEnd:
		return Bloom
	}
	return Ripen
}

// Schedule is a zone's light schedule: lights-on and lights-off times
// ("HH:MM", wall clock in the Settings time zone) per stage key.
type Schedule map[string][2]string

// ParseClock reads "HH:MM" as minutes after midnight.
func ParseClock(s string) (int, bool) {
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	return hh*60 + mm, true
}

// LightsOn says whether a zone's lights should be on now. Drying is always
// lights off. Without a schedule for the stage the lights count as on and
// scheduled is false. Equal on and off times mean on all day.
func LightsOn(s Schedule, stage string, now time.Time, loc *time.Location) (on, scheduled bool) {
	if stage == Drying {
		return false, true
	}
	t, ok := s[stage]
	if !ok {
		return true, false
	}
	start, ok1 := ParseClock(t[0])
	end, ok2 := ParseClock(t[1])
	if !ok1 || !ok2 {
		return true, false
	}
	if loc != nil {
		now = now.In(loc)
	}
	m := now.Hour()*60 + now.Minute()
	switch {
	case start == end:
		return true, true
	case start < end:
		return m >= start && m < end, true
	}
	return m >= start || m < end, true
}

// Hours is the length of a lights-on period in hours (24 when the times are equal).
func Hours(on, off string) float64 {
	a, ok1 := ParseClock(on)
	b, ok2 := ParseClock(off)
	if !ok1 || !ok2 {
		return 0
	}
	d := (b - a + 1440) % 1440
	if d == 0 {
		d = 1440
	}
	return float64(d) / 60
}
