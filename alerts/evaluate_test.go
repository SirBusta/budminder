package alerts

import (
	"testing"
	"time"

	"isley/climate"
)

func fp(v float64) *float64 { return &v }

var evalNow = time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)

// series makes one reading a minute, the last at evalNow, from the values.
func series(vals ...float64) []climate.Reading {
	out := make([]climate.Reading, len(vals))
	for i, v := range vals {
		out[i] = climate.Reading{At: evalNow.Add(time.Duration(i-len(vals)+1) * time.Minute), Value: v}
	}
	return out
}

func repeatVal(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func vpdInput(vals ...float64) Input {
	return Input{Measure: climate.VPD, Readings: series(vals...), Now: evalNow, Timing: DefaultTiming(),
		MinWindow: 3 * time.Minute, StaleAfter: 5 * time.Minute,
		Range: climate.Range{Low: fp(1.2), Target: fp(1.35), High: fp(1.5)}}
}

func tempInput(vals ...float64) Input {
	return Input{Measure: climate.Temp, Readings: series(vals...), Now: evalNow, Timing: DefaultTiming(),
		MinWindow: 3 * time.Minute, StaleAfter: 5 * time.Minute, Limits: [2]*float64{fp(10), fp(32)}}
}

func actions(cs []Change) map[Key]string {
	out := map[Key]string{}
	for _, c := range cs {
		out[c.Key] = c.Action
	}
	return out
}

func opened(k Key, ago time.Duration) []OpenAlert {
	return []OpenAlert{{Key: k, NotifiedAt: evalNow.Add(-ago)}}
}

func TestSafety_FiresOnSecondReadingInARow(t *testing.T) {
	t.Parallel()
	k := Key{KindSafety, climate.Temp, SideHigh}
	if a := actions(Evaluate(tempInput(25, 25, 33))); a[k] != "" {
		t.Errorf("one reading fired: %v", a)
	}
	got := Evaluate(tempInput(25, 33, 33))
	if a := actions(got); a[k] != Start {
		t.Fatalf("two readings didn't fire: %v", a)
	}
	if got[0].Value != 33 || *got[0].Limit != 32 {
		t.Errorf("value/limit = %v/%v", got[0].Value, *got[0].Limit)
	}
	if a := actions(Evaluate(tempInput(25, 32, 32))); a[k] != Start {
		t.Errorf("reaching the limit exactly counts: %v", a)
	}
	low := Key{KindSafety, climate.Temp, SideLow}
	if a := actions(Evaluate(tempInput(25, 10, 9))); a[low] != Start {
		t.Errorf("low limit: %v", a)
	}
}

func TestSafety_BackToNormalNeedsTheMargin(t *testing.T) {
	t.Parallel()
	k := Key{KindSafety, climate.Temp, SideHigh}
	in := tempInput(33, 31.8, 31.8)
	in.Open = opened(k, time.Minute)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("inside the margin ended it: %v", a)
	}
	in = tempInput(33, 31.5, 31)
	in.Open = opened(k, time.Minute)
	if a := actions(Evaluate(in)); a[k] != End {
		t.Errorf("clear readings didn't end it: %v", a)
	}
	in = tempInput(33, 31, 33)
	in.Open = opened(k, time.Minute)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("one clear reading in a row of two ended it: %v", a)
	}
}

func TestSafety_RepeatsWhileItLasts(t *testing.T) {
	t.Parallel()
	k := Key{KindSafety, climate.Temp, SideHigh}
	in := tempInput(33, 33, 33)
	in.Open = opened(k, 29*time.Minute)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("repeated early: %v", a)
	}
	in.Open = opened(k, 30*time.Minute)
	if a := actions(Evaluate(in)); a[k] != Repeat {
		t.Errorf("didn't repeat: %v", a)
	}
	in.Timing.RepeatCritical = 0
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("repeat set to never: %v", a)
	}
}

func TestSafety_LimitRemovedClosesQuietly(t *testing.T) {
	t.Parallel()
	k := Key{KindSafety, climate.Temp, SideHigh}
	in := tempInput(33, 33, 33)
	in.Limits[1] = nil
	in.Open = opened(k, time.Minute)
	if a := actions(Evaluate(in)); a[k] != Close {
		t.Errorf("got %v", a)
	}
}

func TestStage_FiveMinuteSpikeSendsNothing(t *testing.T) {
	t.Parallel()
	vals := append(repeatVal(1.35, 6), repeatVal(1.7, 5)...)
	if got := Evaluate(vpdInput(vals...)); len(got) != 0 {
		t.Errorf("spike fired: %v", got)
	}
}

func TestStage_MedianOfTheWindow(t *testing.T) {
	t.Parallel()
	k := Key{KindStage, climate.VPD, SideHigh}
	vals := append(repeatVal(1.35, 4), repeatVal(1.5, 7)...)
	got := Evaluate(vpdInput(vals...))
	if a := actions(got); a[k] != Start {
		t.Fatalf("most readings at High didn't fire: %v", a)
	}
	if got[0].Value != 1.5 || *got[0].Limit != 1.5 {
		t.Errorf("value/limit = %v/%v", got[0].Value, *got[0].Limit)
	}
	low := Key{KindStage, climate.VPD, SideLow}
	if a := actions(Evaluate(vpdInput(repeatVal(1.1, 11)...))); a[low] != Start {
		t.Errorf("low: %v", a)
	}
}

func TestStage_TooFewReadings(t *testing.T) {
	t.Parallel()
	if got := Evaluate(vpdInput(1.7, 1.7)); len(got) != 0 {
		t.Errorf("two readings fired: %v", got)
	}
}

func TestStage_SlowSensorWidensTheWindow(t *testing.T) {
	t.Parallel()
	// A reading every 8 minutes: only 2 fall in 10 minutes, but 3 in 3 polls.
	in := vpdInput()
	for i := 3; i >= 0; i-- {
		in.Readings = append(in.Readings, climate.Reading{At: evalNow.Add(-time.Duration(i*8) * time.Minute), Value: 1.7})
	}
	in.StaleAfter = 24 * time.Minute
	k := Key{KindStage, climate.VPD, SideHigh}
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("10-minute window with 2 readings fired: %v", a)
	}
	in.MinWindow = 24 * time.Minute
	if a := actions(Evaluate(in)); a[k] != Start {
		t.Errorf("3-poll window didn't fire: %v", a)
	}
}

func TestStage_BackToNormalNeedsTheMargin(t *testing.T) {
	t.Parallel()
	k := Key{KindStage, climate.VPD, SideHigh}
	in := vpdInput(repeatVal(1.47, 11)...)
	in.Open = opened(k, time.Minute)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("inside the margin ended it: %v", a)
	}
	in = vpdInput(repeatVal(1.45, 11)...)
	in.Open = opened(k, time.Minute)
	if a := actions(Evaluate(in)); a[k] != End {
		t.Errorf("clear median didn't end it: %v", a)
	}
}

func TestStage_PauseAfterALightSwitch(t *testing.T) {
	t.Parallel()
	k := Key{KindStage, climate.VPD, SideHigh}
	vals := repeatVal(1.7, 30)

	in := vpdInput(vals...)
	in.LastSwitch = evalNow.Add(-5 * time.Minute)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("fired during the pause: %v", a)
	}
	in.Open = opened(k, time.Minute)
	in.Readings = series(repeatVal(1.0, 30)...)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("ended during the pause: %v", a)
	}

	// Pause over, but too few readings since it ended.
	in = vpdInput(vals...)
	in.LastSwitch = evalNow.Add(-11 * time.Minute)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("fired on readings from the pause: %v", a)
	}

	in.LastSwitch = evalNow.Add(-13 * time.Minute)
	if a := actions(Evaluate(in)); a[k] != Start {
		t.Errorf("didn't fire 13 minutes after the switch: %v", a)
	}
}

func TestSafety_KeepsRunningDuringThePause(t *testing.T) {
	t.Parallel()
	in := tempInput(33, 33, 33)
	in.LastSwitch = evalNow.Add(-time.Minute)
	if a := actions(Evaluate(in)); a[Key{KindSafety, climate.Temp, SideHigh}] != Start {
		t.Errorf("safety paused: %v", a)
	}
}

func TestStage_NotWhileASafetyAlertIsOpen(t *testing.T) {
	t.Parallel()
	in := tempInput(repeatVal(33, 11)...)
	in.Range = climate.Range{Low: fp(18), Target: fp(24), High: fp(30)}
	s := Key{KindSafety, climate.Temp, SideHigh}
	in.Open = opened(s, time.Minute)
	if a := actions(Evaluate(in)); a[Key{KindStage, climate.Temp, SideHigh}] != "" {
		t.Errorf("doubled up: %v", a)
	}
}

func TestEvaluate_StaleSensorHolds(t *testing.T) {
	t.Parallel()
	k := Key{KindSafety, climate.Temp, SideHigh}
	in := tempInput(25, 25, 25)
	in.Now = evalNow.Add(6 * time.Minute)
	in.Open = opened(k, 2*time.Hour)
	if got := Evaluate(in); len(got) != 0 {
		t.Errorf("stale sensor did something: %v", got)
	}
	in.Readings = nil
	if got := Evaluate(in); len(got) != 0 {
		t.Errorf("no readings did something: %v", got)
	}
}

func TestDrift_OneNoticeAfterTwoHours(t *testing.T) {
	t.Parallel()
	k := Key{KindDrift, climate.VPD, SideHigh}
	in := vpdInput(repeatVal(1.45, 121)...) // amber above Target
	in.CheckDrift = true
	got := Evaluate(in)
	if a := actions(got); a[k] != Start {
		t.Fatalf("didn't fire: %v", a)
	}
	if *got[0].Limit != 1.35 {
		t.Errorf("limit should be Target: %v", *got[0].Limit)
	}

	in.Open = opened(k, 24*time.Hour)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("a notice never repeats: %v", a)
	}

	in.Open = nil
	in.CheckDrift = false
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("started on a check without history: %v", a)
	}
}

func TestDrift_NotWhenABucketIsNearTargetOrOnTheOtherSide(t *testing.T) {
	t.Parallel()
	near := append(repeatVal(1.45, 60), repeatVal(1.36, 11)...)
	near = append(near, repeatVal(1.45, 50)...)
	in := vpdInput(near...)
	in.CheckDrift = true
	if a := actions(Evaluate(in)); a[Key{KindDrift, climate.VPD, SideHigh}] != "" {
		t.Errorf("near Target for 10 minutes: %v", a)
	}

	sides := append(repeatVal(1.25, 60), repeatVal(1.45, 61)...)
	in = vpdInput(sides...)
	in.CheckDrift = true
	for k := range actions(Evaluate(in)) {
		if k.Kind == KindDrift {
			t.Errorf("both sides of Target: %v", k)
		}
	}

	// A single stray reading doesn't break it: one bucket's median stays amber.
	stray := repeatVal(1.45, 121)
	stray[50] = 1.35
	in = vpdInput(stray...)
	in.CheckDrift = true
	if a := actions(Evaluate(in)); a[Key{KindDrift, climate.VPD, SideHigh}] != Start {
		t.Errorf("a stray reading stopped it: %v", a)
	}
}

func TestDrift_NeedsHistoryAndAStableLightPeriod(t *testing.T) {
	t.Parallel()
	k := Key{KindDrift, climate.VPD, SideHigh}
	in := vpdInput(repeatVal(1.45, 60)...)
	in.CheckDrift = true
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("only an hour of history: %v", a)
	}
	in = vpdInput(repeatVal(1.45, 121)...)
	in.CheckDrift = true
	in.LastSwitch = evalNow.Add(-time.Hour)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("light switch an hour ago: %v", a)
	}
}

func TestDrift_NotWhileAnotherAlertIsOpen(t *testing.T) {
	t.Parallel()
	in := vpdInput(repeatVal(1.45, 121)...)
	in.CheckDrift = true
	in.Open = opened(Key{KindStage, climate.VPD, SideHigh}, time.Minute)
	if a := actions(Evaluate(in)); a[Key{KindDrift, climate.VPD, SideHigh}] != "" {
		t.Errorf("doubled up: %v", a)
	}
}

func TestDrift_ClosesWithoutANoticeNearTarget(t *testing.T) {
	t.Parallel()
	k := Key{KindDrift, climate.VPD, SideHigh}
	in := vpdInput(repeatVal(1.36, 11)...)
	in.Open = opened(k, time.Hour)
	if a := actions(Evaluate(in)); a[k] != Close {
		t.Errorf("got %v", a)
	}
	in = vpdInput(repeatVal(1.25, 11)...) // amber on the other side
	in.Open = opened(k, time.Hour)
	if a := actions(Evaluate(in)); a[k] != Close {
		t.Errorf("other side: %v", a)
	}
	in = vpdInput(repeatVal(1.45, 11)...)
	in.Open = opened(k, time.Hour)
	if a := actions(Evaluate(in)); a[k] != "" {
		t.Errorf("still amber: %v", a)
	}
}
