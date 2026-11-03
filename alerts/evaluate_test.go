package alerts

import (
	"strings"
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

// tempInput has a 20 to 30 °C pair of limits and no Target.
func tempInput(vals ...float64) Input {
	return Input{Measure: climate.Temp, Readings: series(vals...), Range: climate.Range{Low: fp(20), High: fp(30)},
		Now: evalNow, MinWindow: 3 * time.Minute, StaleAfter: 5 * time.Minute}
}

func actions(cs []Change) map[Key]string {
	out := map[Key]string{}
	for _, c := range cs {
		out[c.Key] = c.Action
	}
	return out
}

var highKey = Key{KindStage, climate.Temp, SideHigh}

func opened(k Key) []OpenAlert { return []OpenAlert{{Key: k, NotifiedAt: evalNow.Add(-time.Hour)}} }

func TestEvaluate_StartsAfterFifteenMinutesStraight(t *testing.T) {
	t.Parallel()
	if got := actions(Evaluate(tempInput(repeatVal(31, 16)...))); got[highKey] != Start {
		t.Errorf("15 minutes at the limit: %v", got)
	}
	if got := Evaluate(tempInput(repeatVal(30, 16)...)); len(got) != 1 || got[0].Action != Start || *got[0].Limit != 30 {
		t.Errorf("reaching the limit counts: %v", got)
	}
	if got := Evaluate(tempInput(repeatVal(31, 6)...)); len(got) != 0 {
		t.Errorf("a sensor that just came back isn't taken for the whole hold: %v", got)
	}
	vals := repeatVal(31, 16)
	vals[8] = 29
	if got := Evaluate(tempInput(vals...)); len(got) != 0 {
		t.Errorf("one reading in range resets it: %v", got)
	}
	// Readings before the hold don't matter.
	if got := actions(Evaluate(tempInput(append(repeatVal(25, 10), repeatVal(31, 16)...)...))); got[highKey] != Start {
		t.Errorf("older readings: %v", got)
	}
}

func TestEvaluate_LowSide(t *testing.T) {
	t.Parallel()
	got := Evaluate(tempInput(repeatVal(20, 16)...))
	if len(got) != 1 || got[0].Key != (Key{KindStage, climate.Temp, SideLow}) || got[0].Action != Start {
		t.Errorf("reaching Low: %v", got)
	}
}

func TestEvaluate_NoLimitsNoAlert(t *testing.T) {
	t.Parallel()
	in := tempInput(repeatVal(99, 16)...)
	in.Range = climate.Range{Target: fp(25)}
	if got := Evaluate(in); len(got) != 0 {
		t.Errorf("a Target alone sends nothing: %v", got)
	}
}

func TestEvaluate_EndsAfterThirtyMinutesInRange(t *testing.T) {
	t.Parallel()
	in := tempInput(repeatVal(25, 31)...)
	in.Open = opened(highKey)
	if got := actions(Evaluate(in)); got[highKey] != End {
		t.Errorf("30 minutes in range: %v", got)
	}
	vals := repeatVal(25, 31)
	vals[5] = 30
	in = tempInput(vals...)
	in.Open = opened(highKey)
	if got := Evaluate(in); len(got) != 0 {
		t.Errorf("one reading at the limit keeps it open: %v", got)
	}
	in = tempInput(repeatVal(25, 10)...)
	in.Open = opened(highKey)
	if got := Evaluate(in); len(got) != 0 {
		t.Errorf("a sensor that just came back doesn't end it: %v", got)
	}
}

func TestEvaluate_OpenAlertDoesNotStartAgain(t *testing.T) {
	t.Parallel()
	in := tempInput(repeatVal(31, 31)...)
	in.Open = opened(highKey)
	if got := Evaluate(in); len(got) != 0 {
		t.Errorf("still at the limit: %v", got)
	}
}

func TestEvaluate_LimitRemovedClosesQuietly(t *testing.T) {
	t.Parallel()
	in := tempInput(repeatVal(31, 16)...)
	in.Range = climate.Range{Low: fp(20)}
	in.Open = opened(highKey)
	if got := actions(Evaluate(in)); got[highKey] != Close {
		t.Errorf("limit removed: %v", got)
	}
}

func TestEvaluate_PauseAfterALightSwitch(t *testing.T) {
	t.Parallel()
	in := tempInput(repeatVal(31, 40)...)
	in.LastSwitch = evalNow.Add(-20 * time.Minute)
	if got := Evaluate(in); len(got) != 0 {
		t.Errorf("the hold reaches back into the pause: %v", got)
	}
	in.LastSwitch = evalNow.Add(-25 * time.Minute)
	if got := actions(Evaluate(in)); got[highKey] != Start {
		t.Errorf("25 minutes after the switch: %v", got)
	}
}

func TestEvaluate_SlowSensorWidensTheHold(t *testing.T) {
	t.Parallel()
	readings := make([]climate.Reading, 4)
	for i := range readings {
		readings[i] = climate.Reading{At: evalNow.Add(time.Duration(i-3) * 10 * time.Minute), Value: 31}
	}
	in := Input{Measure: climate.Temp, Readings: readings, Range: climate.Range{High: fp(30)}, Now: evalNow,
		MinWindow: 30 * time.Minute, StaleAfter: 30 * time.Minute}
	if got := actions(Evaluate(in)); got[highKey] != Start {
		t.Errorf("three polls of a slow sensor: %v", got)
	}
}

func TestEvaluate_StaleSensorHolds(t *testing.T) {
	t.Parallel()
	in := tempInput(repeatVal(25, 31)...)
	in.Now = evalNow.Add(10 * time.Minute)
	in.Open = opened(highKey)
	if got := Evaluate(in); len(got) != 0 {
		t.Errorf("not reporting isn't back to normal: %v", got)
	}
	if got := Evaluate(Input{Measure: climate.Temp, Now: evalNow}); len(got) != 0 {
		t.Errorf("no readings: %v", got)
	}
}

func testWords() Words {
	text := map[string]string{
		"al_ev_low": "{measure} low in {zone}", "al_ev_high": "{measure} high in {zone}", "al_ev_ok": "Climate back to normal in {zone}",
		"al_line_stage_high": "{measure} is {value}, High {limit}.", "al_line_ok": "Readings in {zone} in range.",
		"al_ev_offline": "Sensors offline in {zone}", "al_ev_offline_all": "Sensors offline", "al_ev_offline_ok": "Back online in {zone}",
		"al_ev_offline_ok_all": "Back online", "al_line_offline": "No readings from {sensors}.", "al_line_offline_ok": "Resumed.",
		"al_measure_temp": "Temperature", "al_since": "Since {time}.", "al_still": "Still active.", "al_widen": "Widen.",
		"al_subject": "BudMinder: {title}", "al_subject_more": "BudMinder: {title} and {n} more", "al_open_budminder": "Open",
	}
	return Words{T: func(k string) string { return text[k] }, Units: "metric", Loc: time.UTC}
}

func TestWords_Offline(t *testing.T) {
	t.Parallel()
	w := testWords()
	e := Event{Kind: KindOffline, Zone: "Tent", Detail: "Temp, Humidity"}
	if got := w.Title(e, false); got != "Sensors offline in Tent" {
		t.Errorf("title: %q", got)
	}
	if got := w.Line(e, false); got != "No readings from Temp, Humidity." {
		t.Errorf("line: %q", got)
	}
	if got := w.Title(e, true); got != "Back online in Tent" {
		t.Errorf("resolved: %q", got)
	}
	e.Zone = ""
	if got := w.Title(e, false); got != "Sensors offline" {
		t.Errorf("no zone: %q", got)
	}
	if got := w.Title(e, true); got != "Back online" {
		t.Errorf("no zone, resolved: %q", got)
	}
	if Severity(KindOffline) != Critical {
		t.Error("offline is the most serious")
	}
}

func TestBuildMessage_ClimateAdvice(t *testing.T) {
	t.Parallel()
	w := testWords()
	v, l := 33.0, 30.0
	stage := Event{Kind: KindStage, Measure: climate.Temp, Side: SideHigh, Zone: "Tent", Value: &v, Limit: &l, Started: evalNow, Pending: NoticeFired}
	offline := Event{Kind: KindOffline, Zone: "Tent", Detail: "Temp", Started: evalNow, Pending: NoticeRepeat}
	m := BuildMessage(w, []Event{stage, offline}, "")
	if !strings.Contains(m.Subject, "Sensors offline in Tent and 1 more") {
		t.Errorf("the most serious names the subject: %q", m.Subject)
	}
	if !strings.Contains(m.Text, "Widen.") || !strings.Contains(m.Text, "Still active.") {
		t.Errorf("body: %q", m.Text)
	}
	stage.Pending = NoticeResolved
	m = BuildMessage(w, []Event{stage}, "")
	if !strings.Contains(m.Text, "Climate back to normal in Tent") || strings.Contains(m.Text, "Widen.") {
		t.Errorf("all clear: %q", m.Text)
	}
}
