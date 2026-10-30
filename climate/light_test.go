package climate

import (
	"testing"
	"time"
)

func TestLastSwitch(t *testing.T) {
	t.Parallel()
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skip("no zone data")
	}
	s := Schedule{Flower: {"06:00", "18:00"}, Veg: {"18:00", "06:00"}, Seedling: {"06:00", "06:00"}}
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, loc) }

	got, ok := LastSwitch(s, Flower, at(2026, 6, 10, 9, 0), loc)
	if !ok || !got.Equal(at(2026, 6, 10, 6, 0)) {
		t.Errorf("morning: %v %v", got, ok)
	}
	got, ok = LastSwitch(s, Flower, at(2026, 6, 10, 5, 0), loc)
	if !ok || !got.Equal(at(2026, 6, 9, 18, 0)) {
		t.Errorf("before lights on: yesterday's off, got %v %v", got, ok)
	}
	got, ok = LastSwitch(s, Flower, at(2026, 6, 10, 6, 0), loc)
	if !ok || !got.Equal(at(2026, 6, 10, 6, 0)) {
		t.Errorf("right at the switch: %v %v", got, ok)
	}
	// Overnight schedule: on at 18:00, off at 06:00.
	got, ok = LastSwitch(s, Veg, at(2026, 6, 10, 2, 0), loc)
	if !ok || !got.Equal(at(2026, 6, 9, 18, 0)) {
		t.Errorf("overnight: %v %v", got, ok)
	}
	// Daylight saving ended Nov 1, 2026: the wall-clock time still counts.
	got, ok = LastSwitch(s, Flower, at(2026, 11, 1, 7, 0), loc)
	if !ok || !got.Equal(at(2026, 11, 1, 6, 0)) {
		t.Errorf("DST day: %v %v", got, ok)
	}
	if got.UTC().Hour() != 12 {
		t.Errorf("6 AM after DST ended is 12:00 UTC, got %v", got.UTC())
	}

	for name, stage := range map[string]string{"equal times": Seedling, "drying": Drying, "no schedule": "other"} {
		if _, ok := LastSwitch(s, stage, at(2026, 6, 10, 9, 0), loc); ok {
			t.Errorf("%s has no switches", name)
		}
	}
}

func TestMedian(t *testing.T) {
	t.Parallel()
	vals := []float64{3, 1, 2}
	if m := Median(vals); m != 2 {
		t.Errorf("odd: %v", m)
	}
	if vals[0] != 3 {
		t.Error("Median changed its input")
	}
	if m := Median([]float64{1, 2, 3, 4}); m != 2.5 {
		t.Errorf("even: %v", m)
	}
	if m := Median(nil); m != 0 {
		t.Errorf("empty: %v", m)
	}
}

func TestLightRuns(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	var rs []Reading
	for i, v := range []float64{0, 0, 100, 100, 100, 0} {
		rs = append(rs, Reading{At: base.Add(time.Duration(i) * time.Minute), Value: v})
	}
	runs := LightRuns(rs, 0)
	if len(runs) != 3 || runs[0].On || !runs[1].On || runs[2].On {
		t.Fatalf("runs: %+v", runs)
	}
	if !runs[1].From.Equal(base.Add(2*time.Minute)) || !runs[1].To.Equal(base.Add(4*time.Minute)) {
		t.Errorf("middle run: %+v", runs[1])
	}
}

// lightReadings makes one reading a minute over [from, now], on by the func.
func lightReadings(from, now time.Time, on func(time.Time) bool) []Reading {
	var out []Reading
	for at := from; !at.After(now); at = at.Add(time.Minute) {
		v := 0.0
		if on(at) {
			v = 100
		}
		out = append(out, Reading{At: at, Value: v})
	}
	return out
}

func TestCheckLights(t *testing.T) {
	t.Parallel()
	s := Schedule{Flower: {"06:00", "18:00"}}
	now := time.Date(2026, 6, 10, 8, 14, 0, 0, time.UTC)
	hold := 15 * time.Minute
	off := func(time.Time) bool { return false }
	on := func(time.Time) bool { return true }

	m, agree := CheckLights(lightReadings(now.Add(-hold), now, off), 0, s, Flower, now, time.UTC, hold)
	if m == nil || agree || m.SeenOn || !m.Since.Equal(time.Date(2026, 6, 10, 6, 0, 0, 0, time.UTC)) {
		t.Errorf("lights off in the day: %+v %v", m, agree)
	}
	m, agree = CheckLights(lightReadings(now.Add(-hold), now, on), 0, s, Flower, now, time.UTC, hold)
	if m != nil || !agree {
		t.Errorf("as scheduled: %+v %v", m, agree)
	}

	// One agreeing reading in the window: not a mismatch yet.
	early := now.Add(-10 * time.Minute)
	m, _ = CheckLights(lightReadings(now.Add(-hold), now, func(at time.Time) bool { return at.Before(early) }), 0, s, Flower, now, time.UTC, hold)
	if m != nil {
		t.Errorf("only 10 minutes of disagreement: %+v", m)
	}

	// Just after a scheduled switch: wait, however it looks.
	soon := time.Date(2026, 6, 10, 6, 10, 0, 0, time.UTC)
	m, agree = CheckLights(lightReadings(soon.Add(-hold), soon, off), 0, s, Flower, soon, time.UTC, hold)
	if m != nil || agree {
		t.Errorf("10 minutes after the switch: %+v %v", m, agree)
	}

	// A sensor that only just came back doesn't count as 15 minutes.
	m, _ = CheckLights(lightReadings(now.Add(-3*time.Minute), now, off), 0, s, Flower, now, time.UTC, hold)
	if m != nil {
		t.Errorf("3 minutes of readings: %+v", m)
	}

	// No schedule, or too few readings: nothing either way.
	m, agree = CheckLights(lightReadings(now.Add(-hold), now, off), 0, Schedule{}, Flower, now, time.UTC, hold)
	if m != nil || agree {
		t.Errorf("no schedule: %+v %v", m, agree)
	}
	m, agree = CheckLights(nil, 0, s, Flower, now, time.UTC, hold)
	if m != nil || agree {
		t.Errorf("no readings: %+v %v", m, agree)
	}

	// Drying: lights should be off all the time.
	m, _ = CheckLights(lightReadings(now.Add(-hold), now, on), 0, s, Drying, now, time.UTC, hold)
	if m == nil || !m.SeenOn {
		t.Errorf("light on while drying: %+v", m)
	}
}

// scheduledBy returns "on" for a 06:00-18:00 schedule moved by shift.
func scheduledBy(shift time.Duration) func(time.Time) bool {
	return func(at time.Time) bool {
		at = at.Add(-shift)
		return at.Hour() >= 6 && at.Hour() < 18
	}
}

func TestClockOffset(t *testing.T) {
	t.Parallel()
	s := Schedule{Flower: {"06:00", "18:00"}}
	now := time.Date(2026, 6, 10, 20, 30, 0, 0, time.UTC)
	from := now.Add(-ClockOffsetLookback)

	sh := ClockOffset(lightReadings(from, now, scheduledBy(-time.Hour)), 0, s, Flower, now, time.UTC)
	if sh == nil || !sh.Early || sh.Minutes != 60 {
		t.Fatalf("an hour early: %+v", sh)
	}
	if sh.SeenOn.Hour() != 5 || sh.SeenOff.Hour() != 17 || sh.SeenOff.Day() != 10 {
		t.Errorf("seen times: %v %v", sh.SeenOn, sh.SeenOff)
	}

	sh = ClockOffset(lightReadings(from, now, scheduledBy(time.Hour)), 0, s, Flower, now, time.UTC)
	if sh == nil || sh.Early {
		t.Errorf("an hour late: %+v", sh)
	}

	if sh := ClockOffset(lightReadings(from, now, scheduledBy(0)), 0, s, Flower, now, time.UTC); sh != nil {
		t.Errorf("on time: %+v", sh)
	}
	if sh := ClockOffset(lightReadings(from, now, scheduledBy(20*time.Minute)), 0, s, Flower, now, time.UTC); sh != nil {
		t.Errorf("20 minutes off is no clock problem: %+v", sh)
	}

	// Only 2 days of history: not enough.
	if sh := ClockOffset(lightReadings(now.Add(-48*time.Hour), now, scheduledBy(-time.Hour)), 0, s, Flower, now, time.UTC); sh != nil {
		t.Errorf("2 days: %+v", sh)
	}

	// One normal day among the three: not steady.
	shifted := scheduledBy(-time.Hour)
	normal := scheduledBy(0)
	mix := func(at time.Time) bool {
		if at.Day() == 9 {
			return normal(at)
		}
		return shifted(at)
	}
	if sh := ClockOffset(lightReadings(from, now, mix), 0, s, Flower, now, time.UTC); sh != nil {
		t.Errorf("a normal day in the middle: %+v", sh)
	}
}
