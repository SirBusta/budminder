package alerts

import (
	"time"

	"isley/climate"
)

// Kinds of alert.
const (
	KindStage   = "stage"   // a reading at a Low or High limit
	KindLight   = "light"   // lights disagree with the schedule
	KindClock   = "clock"   // lights steadily about an hour off the schedule
	KindSensor  = "sensor"  // a sensor alert the grower set up (rule.go)
	KindOffline = "offline" // sensors that stopped sending readings
	// KindIntegration: a sensor integration refuses BudMinder's sign-in or
	// token, or doesn't answer. Measure is the source ("acinfinity"), Detail
	// "auth" or "failing".
	KindIntegration = "integration"
)

// Integration problems, in an integration alert's Detail.
const (
	IntegrationAuth    = "auth"
	IntegrationFailing = "failing"
)

// Sides of a limit. Light alerts use what the sensor saw; clock alerts use
// which way the lights are off.
const (
	SideLow   = "low"
	SideHigh  = "high"
	SideOn    = "on"
	SideOff   = "off"
	SideEarly = "early"
	SideLate  = "late"
)

// The one set of times every alert follows.
const (
	Hold         = 15 * time.Minute // a problem must last this long before an alert starts
	Settle       = 30 * time.Minute // readings must be back in range this long before it ends
	Pause        = 10 * time.Minute // climate checks wait this long after a light switch
	Remind       = 6 * time.Hour    // how often an open alert is sent again
	OfflineAfter = 15 * time.Minute // no readings for this long: the sensor is offline
)

// LightHold is how long lights must disagree with the schedule before an alert.
const LightHold = Hold

// minHoldReadings is the fewest readings a hold is judged from.
const minHoldReadings = 3

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
	Readings   []climate.Reading // oldest first; temperature in °C; covers Settle
	Range      climate.Range     // the Low and High limits in use now
	LastSwitch time.Time         // the latest light switch; zero when there is none
	Now        time.Time
	MinWindow  time.Duration // the hold never gets shorter than this (3 polls)
	StaleAfter time.Duration // no reading for this long: hold everything
	Open       []OpenAlert   // open alerts for this zone and measure
}

// Evaluate compares a measure's readings with its limits. An alert starts when
// every reading of the hold reached a limit, and ends when none has for Settle.
// It holds (returns nothing) when the sensor isn't reporting. Readings from
// before the pause after a light switch ended don't start an alert: they were
// judged against other limits, or are the lights' own swing.
func Evaluate(in Input) []Change {
	n := len(in.Readings)
	if n == 0 || in.Now.Sub(in.Readings[n-1].At) > in.StaleAfter {
		return nil
	}
	latest := in.Readings[n-1].Value
	open := map[Key]bool{}
	for _, o := range in.Open {
		open[o.Key] = true
	}
	hold := max(Hold, in.MinWindow)
	var out []Change
	for _, side := range []string{SideLow, SideHigh} {
		k := Key{KindStage, in.Measure, side}
		limit := in.Range.Low
		if side == SideHigh {
			limit = in.Range.High
		}
		switch {
		case limit == nil:
			if open[k] {
				out = append(out, Change{Key: k, Action: Close})
			}
		case open[k]:
			if noneReached(in.Readings, *limit, side, in.Now, max(Settle, in.MinWindow)) {
				out = append(out, Change{Key: k, Action: End, Value: latest, Limit: limit})
			}
		case !in.LastSwitch.IsZero() && in.Now.Add(-hold).Before(in.LastSwitch.Add(Pause)):
		case allReached(in.Readings, *limit, side, in.Now, hold):
			out = append(out, Change{Key: k, Action: Start, Value: latest, Limit: limit})
		}
	}
	return out
}

// reached says v has reached a limit on a side.
func reached(v, limit float64, side string) bool {
	if side == SideLow {
		return v <= limit
	}
	return v >= limit
}

// inWindow returns the readings of the last w, and whether there are enough to
// judge: a sensor that just came back isn't taken for the whole time.
func inWindow(readings []climate.Reading, now time.Time, w time.Duration) ([]climate.Reading, bool) {
	from := now.Add(-w)
	i := len(readings)
	for i > 0 && !readings[i-1].At.Before(from) {
		i--
	}
	recent := readings[i:]
	n := len(recent)
	return recent, n >= 2 && recent[n-1].At.Sub(recent[0].At) >= w/2
}

func allReached(readings []climate.Reading, limit float64, side string, now time.Time, w time.Duration) bool {
	recent, ok := inWindow(readings, now, w)
	if !ok || len(recent) < minHoldReadings {
		return false
	}
	for _, r := range recent {
		if !reached(r.Value, limit, side) {
			return false
		}
	}
	return true
}

func noneReached(readings []climate.Reading, limit float64, side string, now time.Time, w time.Duration) bool {
	recent, ok := inWindow(readings, now, w)
	if !ok {
		return false
	}
	for _, r := range recent {
		if reached(r.Value, limit, side) {
			return false
		}
	}
	return true
}
