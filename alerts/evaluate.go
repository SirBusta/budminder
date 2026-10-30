package alerts

import (
	"time"

	"isley/climate"
)

// Kinds of alert.
const (
	KindSafety = "safety" // a temperature or humidity safety limit
	KindStage  = "stage"  // a stage's Low or High
	KindDrift  = "drift"  // amber on one side of Target for a long time
	KindLight  = "light"  // lights disagree with the schedule
	KindClock  = "clock"  // lights steadily about an hour off the schedule
	KindSensor = "sensor" // a sensor alert the grower set up (rule.go)
)

// Severities.
const (
	Critical = "critical"
	Warning  = "warning"
	Info     = "info"
)

// Sides of a limit or Target. Light alerts use what the sensor saw; clock
// alerts use which way the lights are off.
const (
	SideLow   = "low"
	SideHigh  = "high"
	SideOn    = "on"
	SideOff   = "off"
	SideEarly = "early"
	SideLate  = "late"
)

// Severity returns a kind's severity.
func Severity(kind string) string {
	switch kind {
	case KindSafety:
		return Critical
	case KindStage, KindLight, KindSensor:
		return Warning
	}
	return Info
}

// Timing holds the Climate alerts card's times.
type Timing struct {
	SafetyReadings int           // readings in a row that trigger a safety limit
	Window         time.Duration // the Low / High median window
	Drift          time.Duration // how long amber lasts before a drifting notice
	Pause          time.Duration // Low, High and drift checks wait this long after a light switch
	RepeatCritical time.Duration // 0 = never
	RepeatWarning  time.Duration // 0 = never
}

// DefaultTiming is what the card starts with.
func DefaultTiming() Timing {
	return Timing{SafetyReadings: 2, Window: 10 * time.Minute, Drift: 2 * time.Hour, Pause: 10 * time.Minute,
		RepeatCritical: 30 * time.Minute, RepeatWarning: 2 * time.Hour}
}

// Repeat is how often a kind repeats while it lasts; 0 means it never does.
func (t Timing) Repeat(kind string) time.Duration {
	switch Severity(kind) {
	case Critical:
		return t.RepeatCritical
	case Warning:
		return t.RepeatWarning
	}
	return 0
}

// LightHold is how long lights must disagree with the schedule before an alert.
const LightHold = 15 * time.Minute

// minMedianReadings is the fewest readings a median is taken from.
const minMedianReadings = 3

// driftBucket is the slice of a drift period that gets one median, so a
// moment near Target doesn't restart the clock.
const driftBucket = 10 * time.Minute

// driftCoverage is the share of buckets that must have readings.
const driftCoverage = 0.8

// margin is how far past a limit a reading must come back before an alert
// ends, so one hovering at the limit doesn't flap (temperature in Â°C).
var margin = map[string]float64{climate.Temp: 0.5, climate.RH: 2, climate.VPD: 0.05}

// Key identifies an alert that can be open.
type Key struct {
	Kind, Measure, Side string
}

// OpenAlert is an alert that hasn't ended.
type OpenAlert struct {
	Key
	NotifiedAt time.Time
}

// Actions.
const (
	Start  = "start"
	Repeat = "repeat"
	End    = "end"   // back to normal: tell the grower
	Close  = "close" // ended without a notice (no longer applies, or a notice kind)
)

// Change is something Evaluate wants done.
type Change struct {
	Key
	Action string
	Value  float64
	Limit  *float64
}

// Input is one measure of one zone.
type Input struct {
	Measure    string
	Readings   []climate.Reading // oldest first; temperature in Â°C; at least Window's worth
	Limits     [2]*float64       // safety limits; both nil for VPD
	Range      climate.Range     // the stage's range now; empty when not by stage
	LastSwitch time.Time         // the latest light switch; zero when there is none
	Now        time.Time
	Timing     Timing
	MinWindow  time.Duration // the window never gets shorter than this (3 polls)
	StaleAfter time.Duration // no reading for this long: hold everything
	CheckDrift bool          // Readings cover Timing.Drift; start drift notices
	Open       []OpenAlert   // open alerts for this zone and measure
}

// Evaluate compares a measure's readings with its limits and range. It holds
// (returns nothing) when the sensor isn't reporting.
func Evaluate(in Input) []Change {
	if n := len(in.Readings); n == 0 || in.Now.Sub(in.Readings[n-1].At) > in.StaleAfter {
		return nil
	}
	open := map[Key]OpenAlert{}
	for _, o := range in.Open {
		open[o.Key] = o
	}
	var out []Change
	out = append(out, evalSafety(in, open)...)
	out = append(out, evalStage(in, open)...)
	out = append(out, evalDrift(in, open)...)
	return repeats(in, out)
}

// repeats adds a repeat for each open alert that's due and isn't ending.
func repeats(in Input, out []Change) []Change {
	done := map[Key]bool{}
	for _, c := range out {
		done[c.Key] = true
	}
	latest := in.Readings[len(in.Readings)-1].Value
	for _, o := range in.Open {
		every := in.Timing.Repeat(o.Kind)
		if done[o.Key] || every <= 0 || in.Now.Sub(o.NotifiedAt) < every {
			continue
		}
		out = append(out, Change{Key: o.Key, Action: Repeat, Value: latest, Limit: limitFor(in, o.Key)})
	}
	return out
}

func limitFor(in Input, k Key) *float64 {
	switch k.Kind {
	case KindSafety:
		if k.Side == SideLow {
			return in.Limits[0]
		}
		return in.Limits[1]
	case KindStage:
		if k.Side == SideLow {
			return in.Range.Low
		}
		return in.Range.High
	case KindDrift:
		return in.Range.Target
	}
	return nil
}

// reached says v has reached a limit on a side; cleared says it's come back past
// the limit by the margin.
func reached(v, limit float64, side string) bool {
	if side == SideLow {
		return v <= limit
	}
	return v >= limit
}

func cleared(v, limit float64, side, measure string) bool {
	if side == SideLow {
		return v > limit+margin[measure]-1e-9
	}
	return v < limit-margin[measure]+1e-9
}

func evalSafety(in Input, open map[Key]OpenAlert) []Change {
	n := in.Timing.SafetyReadings
	if n < 1 {
		n = 1
	}
	var out []Change
	for i, side := range []string{SideLow, SideHigh} {
		k := Key{KindSafety, in.Measure, side}
		_, isOpen := open[k]
		limit := in.Limits[i]
		if limit == nil {
			if isOpen {
				out = append(out, Change{Key: k, Action: Close})
			}
			continue
		}
		if len(in.Readings) < n {
			continue
		}
		last := in.Readings[len(in.Readings)-n:]
		allReached, allClear := true, true
		for _, r := range last {
			allReached = allReached && reached(r.Value, *limit, side)
			allClear = allClear && cleared(r.Value, *limit, side, in.Measure)
		}
		v := last[n-1].Value
		switch {
		case !isOpen && allReached:
			out = append(out, Change{Key: k, Action: Start, Value: v, Limit: limit})
		case isOpen && allClear:
			out = append(out, Change{Key: k, Action: End, Value: v, Limit: limit})
		}
	}
	return out
}

// window returns the readings of the median window, or none during the pause
// after a light switch. Readings from before the pause ended were judged
// against other targets (or are the lights' own swing), so they don't count.
func window(in Input) (vals []float64, paused bool) {
	w := in.Timing.Window
	if w < in.MinWindow {
		w = in.MinWindow
	}
	from := in.Now.Add(-w)
	if !in.LastSwitch.IsZero() {
		end := in.LastSwitch.Add(in.Timing.Pause)
		if in.Now.Before(end) {
			return nil, true
		}
		if end.After(from) {
			from = end
		}
	}
	for _, r := range in.Readings {
		if !r.At.Before(from) {
			vals = append(vals, r.Value)
		}
	}
	return vals, false
}

func evalStage(in Input, open map[Key]OpenAlert) []Change {
	var out []Change
	limits := map[string]*float64{SideLow: in.Range.Low, SideHigh: in.Range.High}
	vals, paused := window(in)
	for _, side := range []string{SideLow, SideHigh} {
		k := Key{KindStage, in.Measure, side}
		_, isOpen := open[k]
		limit := limits[side]
		if limit == nil {
			if isOpen {
				out = append(out, Change{Key: k, Action: Close})
			}
			continue
		}
		if paused || len(vals) < minMedianReadings {
			continue
		}
		m := climate.Median(vals)
		_, safety := open[Key{KindSafety, in.Measure, side}]
		switch {
		case !isOpen && !safety && reached(m, *limit, side):
			out = append(out, Change{Key: k, Action: Start, Value: m, Limit: limit})
		case isOpen && cleared(m, *limit, side, in.Measure):
			out = append(out, Change{Key: k, Action: End, Value: m, Limit: limit})
		}
	}
	return out
}

func evalDrift(in Input, open map[Key]OpenAlert) []Change {
	var out []Change
	target := in.Range.Target
	if target == nil {
		for _, side := range []string{SideLow, SideHigh} {
			if k := (Key{KindDrift, in.Measure, side}); hasOpen(open, k) {
				out = append(out, Change{Key: k, Action: Close})
			}
		}
		return out
	}
	// An open notice ends when readings are near Target or past it.
	vals, paused := window(in)
	if !paused && len(vals) >= minMedianReadings {
		m := climate.Median(vals)
		for _, side := range []string{SideLow, SideHigh} {
			k := Key{KindDrift, in.Measure, side}
			if hasOpen(open, k) && (climate.Band(m, in.Range) == climate.OK || sideOf(m, *target) != side) {
				out = append(out, Change{Key: k, Action: Close, Value: m, Limit: target})
			}
		}
	}
	if !in.CheckDrift {
		return out
	}
	side, v, ok := drifting(in)
	if !ok {
		return out
	}
	for _, s := range []Key{{KindSafety, in.Measure, SideLow}, {KindSafety, in.Measure, SideHigh},
		{KindStage, in.Measure, SideLow}, {KindStage, in.Measure, SideHigh},
		{KindDrift, in.Measure, SideLow}, {KindDrift, in.Measure, SideHigh}} {
		if hasOpen(open, s) {
			return out
		}
	}
	return append(out, Change{Key: Key{KindDrift, in.Measure, side}, Action: Start, Value: v, Limit: target})
}

func hasOpen(open map[Key]OpenAlert, k Key) bool {
	_, ok := open[k]
	return ok
}

func sideOf(v, target float64) string {
	if v < target {
		return SideLow
	}
	return SideHigh
}

// drifting says whether the whole drift period's buckets are amber (or worse)
// on one side of Target, and which side.
func drifting(in Input) (side string, last float64, ok bool) {
	d := in.Timing.Drift
	if d <= 0 || in.Range.Target == nil {
		return "", 0, false
	}
	from := in.Now.Add(-d)
	if !in.LastSwitch.IsZero() && in.LastSwitch.Add(in.Timing.Pause).After(from) {
		return "", 0, false
	}
	n := int(d / driftBucket)
	if n < 1 {
		return "", 0, false
	}
	buckets := make([][]float64, n)
	for _, r := range in.Readings {
		if r.At.Before(from) || r.At.After(in.Now) {
			continue
		}
		i := int(r.At.Sub(from) / driftBucket)
		if i >= n {
			i = n - 1
		}
		buckets[i] = append(buckets[i], r.Value)
	}
	have := 0
	for _, b := range buckets {
		if len(b) == 0 {
			continue
		}
		have++
		m := climate.Median(b)
		st := climate.Band(m, in.Range)
		if st != climate.Toward && st != climate.Reached {
			return "", 0, false
		}
		s := sideOf(m, *in.Range.Target)
		if side != "" && s != side {
			return "", 0, false
		}
		side, last = s, m
	}
	if float64(have) < driftCoverage*float64(n) {
		return "", 0, false
	}
	return side, last, true
}
