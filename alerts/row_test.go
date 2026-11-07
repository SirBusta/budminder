package alerts

import (
	"testing"
	"time"

	"isley/climate"
)

func rowWords(units string) Words {
	text := map[string]string{
		"al_row_highest": "Highest: {value}.", "al_row_highest_so_far": "Highest so far: {value}.",
		"al_row_lowest": "Lowest: {value}.", "al_row_lowest_so_far": "Lowest so far: {value}.",
		"al_row_limit_high": "High limit: {limits}.", "al_row_limit_low": "Low limit: {limits}.", "al_row_limit_rule": "Your limit: {limits}.",
		"al_row_lights_on": "{limit} with lights on", "al_row_lights_off": "{limit} with lights off",
		"al_row_in_stage": "{limit} in {stage}", "al_row_changed_to": "{from}, changed to {to}",
		"al_row_changed_range": "{low}–{high}, changed during the alert",
		"veg_label": "Veg", "flower_label": "Flower", "cl_stretch": "Stretch",
	}
	return Words{T: func(k string) string { return text[k] }, Units: units, Loc: time.UTC}
}

func f64(v float64) *float64 { return &v }

// °F readings are stored in °C.
func fToC(f float64) float64 { return (f - 32) * 5 / 9 }

func TestRowLine(t *testing.T) {
	t.Parallel()
	w := rowWords("imperial")
	at := time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC)
	temp := func(start, peak float64, limits ...LimitUsed) Event {
		return Event{Kind: KindStage, Measure: climate.Temp, Side: SideHigh, StartValue: f64(fToC(start)), Peak: f64(fToC(peak)), Limits: limits}
	}
	lim := func(f float64) LimitUsed { return LimitUsed{V: fToC(f), At: at} }
	cases := []struct {
		name   string
		e      Event
		active bool
		want   string
	}{
		{"highest so far", temp(87.2, 89.3, lim(86)), true, "Highest so far: 89 °F. High limit: 86 °F."},
		{"ended", temp(87.2, 89.3, lim(86)), false, "Highest: 89 °F. High limit: 86 °F."},
		{"under 1 °F past the start: no highest", temp(86.3, 87.2, lim(86)), false, "High limit: 86 °F."},
		{"lights", temp(79.1, 81.2,
			LimitUsed{V: fToC(78), Lights: "on", Stage: "flower"}, LimitUsed{V: fToC(74), Lights: "off", Stage: "flower"},
			LimitUsed{V: fToC(78), Lights: "on", Stage: "flower"}), false,
			"Highest: 81 °F. High limit: 78 °F with lights on, 74 °F with lights off."},
		{"stage", temp(87.1, 87.6,
			LimitUsed{V: fToC(86), Stage: "veg"}, LimitUsed{V: fToC(82), Stage: "flower", Phase: "stretch"}), false,
			"High limit: 86 °F in Veg, 82 °F in Stretch."},
		{"edit", temp(87.3, 88.6, lim(86), lim(90)), false, "Highest: 89 °F. High limit: 86 °F, changed to 90 °F."},
		{"mixed", temp(87.3, 88.6,
			LimitUsed{V: fToC(86), Lights: "on", Stage: "veg"}, LimitUsed{V: fToC(84), Lights: "off", Stage: "veg"},
			LimitUsed{V: fToC(90), Lights: "off", Stage: "veg"}), false,
			"Highest: 89 °F. High limit: 84 °F–90 °F, changed during the alert."},
		{"from before readings were saved", Event{Kind: KindStage, Measure: climate.Temp, Side: SideHigh, Limit: f64(fToC(86))}, false, "High limit: 86 °F."},
		{"low VPD", Event{Kind: KindStage, Measure: climate.VPD, Side: SideLow, StartValue: f64(0.89), Peak: f64(0.83), Limits: []LimitUsed{{V: 0.9}}}, false,
			"Lowest: 0.83 kPa. Low limit: 0.90 kPa."},
		{"sensor alert", Event{Kind: KindSensor, Side: SideHigh, Detail: "ppm", StartValue: f64(1519), Peak: f64(1611), Limit: f64(1500), Limits: []LimitUsed{{V: 1500}}}, false,
			"Highest: 1611 ppm. Your limit: 1500 ppm."},
		{"sensor alert under 1% past", Event{Kind: KindSensor, Side: SideHigh, Detail: "ppm", StartValue: f64(1519), Peak: f64(1530), Limit: f64(1500), Limits: []LimitUsed{{V: 1500}}}, false,
			"Your limit: 1500 ppm."},
		{"offline: no line", Event{Kind: KindOffline, Zone: "Tent"}, false, ""},
	}
	for _, tc := range cases {
		if got := w.RowLine(tc.e, tc.active); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRowLine_Metric(t *testing.T) {
	t.Parallel()
	w := rowWords("metric")
	e := Event{Kind: KindStage, Measure: climate.Temp, Side: SideHigh, StartValue: f64(30.2), Peak: f64(30.6), Limits: []LimitUsed{{V: 30}}}
	if got := w.RowLine(e, false); got != "High limit: 30.0 °C." {
		t.Errorf("0.4 °C past the start: got %q", got)
	}
	e.Peak = f64(30.7)
	if got := w.RowLine(e, false); got != "Highest: 30.7 °C. High limit: 30.0 °C." {
		t.Errorf("0.5 °C past the start: got %q", got)
	}
}

func TestRowValue(t *testing.T) {
	t.Parallel()
	w := rowWords("imperial")
	if got := w.RowValue(Event{Kind: KindStage, Measure: climate.RH, StartValue: f64(71.3)}); got != "71%" {
		t.Errorf("humidity: %q", got)
	}
	if got := w.RowValue(Event{Kind: KindStage, Measure: climate.Temp}); got != "" {
		t.Errorf("not saved: %q", got)
	}
	if got := w.RowValue(Event{Kind: KindLight, StartValue: f64(1)}); got != "" {
		t.Errorf("light: %q", got)
	}
}

func TestLimitsList(t *testing.T) {
	t.Parallel()
	list, added := AddLimit(nil, LimitUsed{V: 30})
	if !added || len(list) != 1 {
		t.Fatalf("first: %v %v", list, added)
	}
	if _, added = AddLimit(list, LimitUsed{V: 30, Lights: "off"}); added {
		t.Error("same value added again")
	}
	list, _ = AddLimit(list, LimitUsed{V: 28})
	back := DecodeLimits(EncodeLimits(list))
	if len(back) != 2 || back[1].V != 28 {
		t.Errorf("round trip: %v", back)
	}
	if DecodeLimits("not json") != nil || EncodeLimits(nil) != "" {
		t.Error("empty or broken text")
	}
}

func TestLimitReasons(t *testing.T) {
	t.Parallel()
	l := func(v float64, lights, stage, phase string) LimitUsed {
		return LimitUsed{V: v, Lights: lights, Stage: stage, Phase: phase}
	}
	cases := []struct {
		name string
		list []LimitUsed
		want []string
	}{
		{"none", nil, []string{}},
		{"one", []LimitUsed{l(30, "on", "veg", "")}, []string{""}},
		{"lights off", []LimitUsed{l(30, "on", "veg", ""), l(28, "off", "veg", "")}, []string{"lights_on", "lights_off"}},
		{"edit", []LimitUsed{l(30, "on", "veg", ""), l(32, "on", "veg", "")}, []string{"", "edit"}},
		{"stage wins over lights", []LimitUsed{l(30, "on", "veg", ""), l(29, "off", "flower", "stretch")}, []string{"stage", "stage"}},
		{"phase", []LimitUsed{l(30, "", "flower", "stretch"), l(29, "", "flower", "bloom")}, []string{"stage", "stage"}},
		{"schedule removed", []LimitUsed{l(30, "on", "veg", ""), l(28, "", "veg", "")}, []string{"lights_on", ""}},
		{"edit then lights", []LimitUsed{l(30, "on", "veg", ""), l(32, "on", "veg", ""), l(29, "off", "veg", "")}, []string{"", "edit", "lights_off"}},
	}
	for _, c := range cases {
		got := LimitReasons(c.list)
		if len(got) != len(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
	w := rowWords("imperial")
	if got := w.StageName(l(1, "", "flower", "stretch")); got != "Stretch" {
		t.Errorf("phase name: %q", got)
	}
	if got := w.StageName(l(1, "", "veg", "")); got != "Veg" {
		t.Errorf("stage name: %q", got)
	}
}

func TestPeak(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	rs := []climate.Reading{{Value: 90, At: at}, {Value: 85, At: at.Add(time.Minute)}, {Value: 87, At: at.Add(2 * time.Minute)}}
	if p := Peak(rs, SideHigh, at.Add(time.Minute)); p == nil || *p != 87 {
		t.Errorf("high from the second reading: %v", p)
	}
	if p := Peak(rs, SideLow, at); p == nil || *p != 85 {
		t.Errorf("low: %v", p)
	}
	if Peak(rs, SideHigh, at.Add(time.Hour)) != nil {
		t.Error("nothing after from")
	}
}
