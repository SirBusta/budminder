package alerts

// What Recent alerts shows for a limit or sensor alert: the reading when it
// started, the highest (or lowest) reading while it was active, and each limit
// in use, with what changed it. The engine records them (handlers/alert_engine.go).

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"isley/climate"
)

// LimitUsed is one limit an alert was judged against. Lights is "on" or "off"
// for a zone with a light schedule, "" otherwise; Stage and Phase are the
// zone's stage and Flower phase at the time. Sensor alerts keep only V and At.
type LimitUsed struct {
	V      float64   `json:"v"`
	At     time.Time `json:"at"`
	Lights string    `json:"lights,omitempty"`
	Stage  string    `json:"stage,omitempty"`
	Phase  string    `json:"phase,omitempty"`
}

// maxLimitsKept caps the list for an alert that runs for days.
const maxLimitsKept = 50

// AddLimit returns the list with l added when its value differs from the last
// one, and whether it changed.
func AddLimit(list []LimitUsed, l LimitUsed) ([]LimitUsed, bool) {
	if n := len(list); n > 0 && list[n-1].V == l.V {
		return list, false
	}
	if len(list) >= maxLimitsKept {
		return list, false
	}
	return append(list, l), true
}

// EncodeLimits is the alert_event.limits text for a list ("" for none).
func EncodeLimits(list []LimitUsed) string {
	if len(list) == 0 {
		return ""
	}
	b, err := json.Marshal(list)
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodeLimits reads alert_event.limits; text it can't read counts as none.
func DecodeLimits(s string) []LimitUsed {
	if s == "" {
		return nil
	}
	var out []LimitUsed
	if json.Unmarshal([]byte(s), &out) != nil {
		return nil
	}
	return out
}

// Peak is the highest reading (high side) or lowest (low side) at or after
// from, or nil when there is none.
func Peak(readings []climate.Reading, side string, from time.Time) *float64 {
	var out *float64
	for _, r := range readings {
		if r.At.Before(from) {
			continue
		}
		if out == nil || (side == SideLow && r.Value < *out) || (side != SideLow && r.Value > *out) {
			v := r.Value
			out = &v
		}
	}
	return out
}

// Further says v is further past the limit than peak (nil peak: yes).
func Further(v float64, peak *float64, side string) bool {
	if peak == nil {
		return true
	}
	if side == SideLow {
		return v < *peak
	}
	return v > *peak
}

// HasChart says a Recent alerts row of this kind opens the Sensors page at the
// alert. Clock notices span days and integration alerts belong to no sensor.
func HasChart(kind string) bool { return kind != KindClock && kind != KindIntegration }

// HasReadings says Recent alerts shows a reading and limit for this kind.
func HasReadings(kind string) bool { return kind == KindStage || kind == KindSensor }

// RowValue is the reading when the alert started, formatted, or "" when there
// is none (an alert from before readings were saved, or a kind without one).
func (w Words) RowValue(e Event) string {
	if !HasReadings(e.Kind) || e.StartValue == nil {
		return ""
	}
	return w.rowAmount(e, *e.StartValue)
}

func (w Words) rowAmount(e Event, v float64) string {
	if e.Kind == KindSensor {
		return sensorAmount(&v, e.Detail)
	}
	return w.Amount(e.Measure, &v)
}

// peakShown says the highest or lowest reading is far enough past the
// starting reading to be worth a mention: 1 °F or 0.5 °C, 1%, 0.05 kPa, or 1%
// of a sensor alert's limit.
func (w Words) peakShown(e Event) bool {
	if e.StartValue == nil || e.Peak == nil {
		return false
	}
	d := math.Abs(*e.Peak - *e.StartValue)
	const eps = 1e-9
	if e.Kind == KindSensor {
		near := 0.01
		if e.Limit != nil && *e.Limit != 0 {
			near = math.Abs(*e.Limit) * 0.01
		}
		return d >= near-eps
	}
	switch e.Measure {
	case climate.Temp:
		if w.Units == "metric" {
			return d >= 0.5-eps
		}
		return d*9/5 >= 1-eps
	case climate.RH:
		return d >= 1-eps
	case climate.VPD:
		return d >= 0.05-eps
	}
	return false
}

// RowLine is the line under a limit or sensor alert's title in Recent alerts:
// the highest or lowest reading when it went further than the start, then the
// limit or limits. Other kinds return "".
func (w Words) RowLine(e Event, active bool) string {
	if !HasReadings(e.Kind) {
		return ""
	}
	var parts []string
	if w.peakShown(e) {
		key := "al_row_highest"
		if e.Side == SideLow {
			key = "al_row_lowest"
		}
		if active {
			key += "_so_far"
		}
		parts = append(parts, w.fill(key, map[string]string{"value": w.rowAmount(e, *e.Peak)}))
	}
	if limits := w.limitsText(e); limits != "" {
		key := "al_row_limit_high"
		switch {
		case e.Kind == KindSensor:
			key = "al_row_limit_rule"
		case e.Side == SideLow:
			key = "al_row_limit_low"
		}
		parts = append(parts, w.fill(key, map[string]string{"limits": limits}))
	}
	return strings.Join(parts, " ")
}

// limitsText lists the limits in use, with what changed them: lights on and
// off, a new stage or Flower phase, or an edit. A mix shows the range.
func (w Words) limitsText(e Event) string {
	list := e.Limits
	if len(list) == 0 {
		if e.Limit == nil {
			return ""
		}
		list = []LimitUsed{{V: *e.Limit}}
	}
	amt := func(v float64) string { return w.rowAmount(e, v) }
	if len(list) == 1 {
		return amt(list[0].V)
	}
	why := ""
	for i := 1; i < len(list); i++ {
		r := changeReason(list[i-1], list[i])
		if why != "" && why != r {
			why = "mixed"
			break
		}
		why = r
	}
	var parts []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			parts = append(parts, s)
		}
	}
	switch {
	case why == "lights":
		for _, l := range list {
			if l.Lights == "" { // the schedule was removed during the alert
				add(amt(l.V))
				continue
			}
			add(w.fill("al_row_lights_"+l.Lights, map[string]string{"limit": amt(l.V)}))
		}
		return strings.Join(parts, ", ")
	case why == "stage":
		for _, l := range list {
			add(w.fill("al_row_in_stage", map[string]string{"limit": amt(l.V), "stage": w.StageName(l)}))
		}
		return strings.Join(parts, ", ")
	case why == "edit" && len(list) == 2:
		return w.fill("al_row_changed_to", map[string]string{"from": amt(list[0].V), "to": amt(list[1].V)})
	}
	lo, hi := list[0].V, list[0].V
	for _, l := range list {
		lo, hi = math.Min(lo, l.V), math.Max(hi, l.V)
	}
	return w.fill("al_row_changed_range", map[string]string{"low": amt(lo), "high": amt(hi)})
}

// changeReason says what changed the limit from a to b: "stage" (a new stage
// or Flower phase, which wins over lights), "lights" or "edit".
func changeReason(a, b LimitUsed) string {
	switch {
	case a.Stage != b.Stage || a.Phase != b.Phase:
		return "stage"
	case a.Lights != b.Lights:
		return "lights"
	}
	return "edit"
}

// StageName is the translated stage, or Flower phase, a limit was used in.
func (w Words) StageName(l LimitUsed) string {
	if l.Phase != "" {
		return w.T("cl_" + l.Phase)
	}
	if l.Stage == "" {
		return ""
	}
	return w.T(l.Stage + "_label")
}

// LimitReasons gives each limit in the list the reason the Sensors page names
// beside it: "lights_on" or "lights_off", "stage" (see StageName), "edit"
// (changed to this value) or "" (nothing to say). The first limit takes the
// reason of the change that ended it, except an edit (it is the value before
// the edit); the others take the reason of the change that started them. A
// lights change to a limit with no lights recorded (the schedule was removed)
// gets "".
func LimitReasons(list []LimitUsed) []string {
	out := make([]string, len(list))
	if len(list) < 2 {
		return out
	}
	for i, l := range list {
		var r string
		if i == 0 {
			if r = changeReason(l, list[1]); r == "edit" {
				r = ""
			}
		} else {
			r = changeReason(list[i-1], l)
		}
		if r == "lights" {
			r = ""
			if l.Lights != "" {
				r = "lights_" + l.Lights
			}
		}
		out[i] = r
	}
	return out
}
