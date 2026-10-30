package alerts

import (
	"strconv"
	"time"

	"isley/climate"
)

// Allowed values for a sensor alert.
var (
	RuleHoldRange = [2]int{1, 1440} // minutes
	RuleLimitMax  = 1000000.0       // either way from zero
)

// MaxRules is how many sensor alerts an install can have.
const MaxRules = 100

// Rule is one sensor alert: a sensor at or past a value. Sensors can measure
// anything, so the limit is in the sensor's own unit, and instead of a margin
// an alert ends when the value has been clear for as long as it had to hold.
type Rule struct {
	ID       int
	SensorID int
	Side     string // SideLow or SideHigh
	Limit    float64
	Hold     time.Duration
	Enabled  bool
}

// RuleMeasure is what a rule's alert_event rows keep in measure, so each rule
// can have its own open alert. Their zone_id is 0: a rule follows its sensor,
// which can move between zones or have none.
func RuleMeasure(id int) string { return "rule:" + strconv.Itoa(id) }

// RuleInput is one rule's check.
type RuleInput struct {
	Rule       Rule
	Readings   []climate.Reading // oldest first, in the sensor's own unit
	Now        time.Time
	Timing     Timing
	MinWindow  time.Duration // the hold never gets shorter than this (2 polls)
	StaleAfter time.Duration // no reading for this long: hold everything
	Open       []OpenAlert   // the rule's open alerts
}

// EvaluateRule compares a sensor's readings with a rule. Nothing changes while
// the sensor isn't reporting or there are too few readings to judge.
func EvaluateRule(in RuleInput) []Change {
	n := len(in.Readings)
	if n == 0 || in.Now.Sub(in.Readings[n-1].At) > in.StaleAfter {
		return nil
	}
	latest := in.Readings[n-1].Value
	k := Key{KindSensor, RuleMeasure(in.Rule.ID), in.Rule.Side}
	var open *OpenAlert
	for i := range in.Open {
		if in.Open[i].Key == k {
			open = &in.Open[i]
		}
	}
	limit := in.Rule.Limit
	w := max(in.Rule.Hold, in.MinWindow)
	from := in.Now.Add(-w)
	var recent []climate.Reading
	for _, r := range in.Readings {
		if !r.At.Before(from) {
			recent = append(recent, r)
		}
	}
	// A sensor that just came back isn't taken for the whole hold.
	judged := len(recent) >= 2 && recent[len(recent)-1].At.Sub(recent[0].At) >= w/2
	allReached, noneReached := judged, judged
	for _, r := range recent {
		if reached(r.Value, limit, in.Rule.Side) {
			noneReached = false
		} else {
			allReached = false
		}
	}
	switch {
	case open == nil && allReached:
		return []Change{{Key: k, Action: Start, Value: latest, Limit: &limit}}
	case open != nil && noneReached:
		return []Change{{Key: k, Action: End, Value: latest, Limit: &limit}}
	case open != nil:
		if every := in.Timing.Repeat(KindSensor); every > 0 && in.Now.Sub(open.NotifiedAt) >= every {
			return []Change{{Key: k, Action: Repeat, Value: latest, Limit: &limit}}
		}
	}
	return nil
}
