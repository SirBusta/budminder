package alerts

import (
	"strconv"
	"time"

	"isley/climate"
)

// RuleLimitMax is how far from zero a sensor alert's value can be.
var RuleLimitMax = 1000000.0

// MaxRules is how many sensor alerts an install can have.
const MaxRules = 100

// Rule is one sensor alert: a sensor at or past a value. Sensors can measure
// anything, so the limit is in the sensor's own unit.
type Rule struct {
	ID       int
	SensorID int
	Side     string // SideLow or SideHigh
	Limit    float64
	Enabled  bool
}

// RuleMeasure is what a rule's alert_event rows keep in measure, so each rule
// can have its own open alert. Their zone_id is 0: a rule follows its sensor,
// which can move between zones or have none.
func RuleMeasure(id int) string { return "rule:" + strconv.Itoa(id) }

// RuleInput is one rule's check.
type RuleInput struct {
	Rule       Rule
	Readings   []climate.Reading // oldest first, in the sensor's own unit; covers Settle
	Now        time.Time
	MinWindow  time.Duration // the hold never gets shorter than this (2 polls)
	StaleAfter time.Duration // no reading for this long: hold everything
	Open       []OpenAlert   // the rule's open alerts
}

// EvaluateRule compares a sensor's readings with a rule, the same way Evaluate
// does for a limit. Nothing changes while the sensor isn't reporting or there
// are too few readings to judge.
func EvaluateRule(in RuleInput) []Change {
	n := len(in.Readings)
	if n == 0 || in.Now.Sub(in.Readings[n-1].At) > in.StaleAfter {
		return nil
	}
	latest := in.Readings[n-1].Value
	k := Key{KindSensor, RuleMeasure(in.Rule.ID), in.Rule.Side}
	isOpen := false
	for _, o := range in.Open {
		isOpen = isOpen || o.Key == k
	}
	limit := in.Rule.Limit
	switch {
	case isOpen && noneReached(in.Readings, limit, in.Rule.Side, in.Now, max(Settle, in.MinWindow)):
		return []Change{{Key: k, Action: End, Value: latest, Limit: &limit}}
	case !isOpen && allReached(in.Readings, limit, in.Rule.Side, in.Now, max(Hold, in.MinWindow)):
		return []Change{{Key: k, Action: Start, Value: latest, Limit: &limit}}
	}
	return nil
}
