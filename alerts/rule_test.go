package alerts

import (
	"testing"
	"time"

	"isley/climate"
)

func ruleReadings(now time.Time, every time.Duration, vals ...float64) []climate.Reading {
	out := make([]climate.Reading, len(vals))
	for i, v := range vals {
		out[i] = climate.Reading{Value: v, At: now.Add(time.Duration(i-len(vals)+1) * every)}
	}
	return out
}

func TestEvaluateRule(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	rule := Rule{ID: 7, SensorID: 3, Side: SideHigh, Limit: 100, Hold: 5 * time.Minute, Enabled: true}
	key := Key{KindSensor, "rule:7", SideHigh}
	check := func(r Rule, open []OpenAlert, readings []climate.Reading) string {
		c := EvaluateRule(RuleInput{Rule: r, Readings: readings, Now: now, Timing: DefaultTiming(),
			MinWindow: 2 * time.Minute, StaleAfter: 10 * time.Minute, Open: open})
		if len(c) == 0 {
			return ""
		}
		return c[0].Action
	}
	min := time.Minute
	fresh := []OpenAlert{{Key: key, NotifiedAt: now.Add(-time.Minute)}}
	due := []OpenAlert{{Key: key, NotifiedAt: now.Add(-3 * time.Hour)}}

	for _, tc := range []struct {
		name     string
		rule     Rule
		open     []OpenAlert
		readings []climate.Reading
		want     string
	}{
		{"held long enough", rule, nil, ruleReadings(now, min, 100, 101, 102, 103, 104, 105), Start},
		{"reaching the limit counts", rule, nil, ruleReadings(now, min, 100, 100, 100, 100, 100, 100), Start},
		{"one reading under", rule, nil, ruleReadings(now, min, 105, 105, 99, 105, 105, 105), ""},
		{"older readings don't matter", rule, nil, ruleReadings(now, min, 1, 1, 105, 105, 105, 105, 105, 105), Start},
		{"just came back", rule, nil, ruleReadings(now, min, 105, 105), ""},
		{"one reading", rule, nil, ruleReadings(now, min, 105), ""},
		{"no readings", rule, nil, nil, ""},
		{"stale holds", rule, fresh, ruleReadings(now.Add(-20*min), min, 1, 1, 1, 1, 1, 1), ""},
		{"still open, not due", rule, fresh, ruleReadings(now, min, 105, 105, 105, 105, 105, 105), ""},
		{"still open, due", rule, due, ruleReadings(now, min, 105, 105, 105, 105, 105, 105), Repeat},
		{"partly clear stays open", rule, fresh, ruleReadings(now, min, 105, 99, 99, 99, 99, 99), ""},
		{"clear for the hold ends", rule, fresh, ruleReadings(now, min, 99, 99, 99, 99, 99, 99), End},
		{"low side", Rule{ID: 7, Side: SideLow, Limit: 10, Hold: 5 * min}, nil, ruleReadings(now, min, 10, 9, 8, 7, 6, 5), Start},
	} {
		if got := check(tc.rule, tc.open, tc.readings); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSensorAlertText(t *testing.T) {
	t.Parallel()
	w := Words{T: func(k string) string {
		return map[string]string{
			"al_ev_sensor_high":   "{sensor} high",
			"al_ev_sensor_ok":     "{sensor} ok",
			"al_line_sensor_high": "{sensor} {value} limit {limit}",
		}[k]
	}}
	v, l := 1234.5678, 1000.0
	e := Event{Kind: KindSensor, Sensor: "CO2", Zone: "Tent", Side: SideHigh, Value: &v, Limit: &l, Detail: "ppm"}
	if got := w.Title(e, false); got != "CO2 (Tent) high" {
		t.Errorf("title: %q", got)
	}
	if got := w.Title(e, true); got != "CO2 (Tent) ok" {
		t.Errorf("resolved title: %q", got)
	}
	if got := w.Line(e, false); got != "CO2 1234.57 ppm limit 1000 ppm" {
		t.Errorf("line: %q", got)
	}
	e.Zone, e.Detail = "", "%"
	if got := w.Title(e, false); got != "CO2 high" {
		t.Errorf("no zone: %q", got)
	}
	if got := w.Line(e, false); got != "CO2 1234.57% limit 1000%" {
		t.Errorf("percent: %q", got)
	}
}
