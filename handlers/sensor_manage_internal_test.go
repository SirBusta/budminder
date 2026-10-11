package handlers

import (
	"isley/model/types"
	"testing"
	"time"
)

func TestShowsOnRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		vis          string
		home, hidden bool
		back         string
	}{
		{"zone_plant", true, false, "zone_plant"},
		{"zone", true, false, "zone_plant"},
		{"plant", false, false, "plant"},
		{"hide", false, true, "hide"},
	}
	for _, tc := range cases {
		home, hidden := showsOn(tc.vis)
		if home != tc.home || hidden != tc.hidden {
			t.Errorf("showsOn(%q) = %v,%v", tc.vis, home, hidden)
		}
		if got := visibilityFor(home, hidden); got != tc.back {
			t.Errorf("visibilityFor(%v,%v) = %q, want %q", home, hidden, got, tc.back)
		}
	}
	if got := visibilityFor(true, true); got != "hide" {
		t.Errorf("hidden wins over Home, got %q", got)
	}
}

func TestStaleAfter(t *testing.T) {
	t.Parallel()
	if got := staleAfter(60); got != 5*time.Minute {
		t.Errorf("staleAfter(60) = %v", got)
	}
	if got := staleAfter(600); got != 30*time.Minute {
		t.Errorf("staleAfter(600) = %v", got)
	}
}

func TestWithSensorLink(t *testing.T) {
	t.Parallel()
	if out, changed := withSensorLink([]int{1, 2}, 3, true); !changed || len(out) != 3 || out[2] != 3 {
		t.Errorf("add: %v %v", out, changed)
	}
	if out, changed := withSensorLink([]int{1, 3, 2}, 3, false); !changed || len(out) != 2 || out[0] != 1 || out[1] != 2 {
		t.Errorf("remove: %v %v", out, changed)
	}
	if _, changed := withSensorLink([]int{1, 3}, 3, true); changed {
		t.Error("already linked should not change")
	}
	if _, changed := withSensorLink(nil, 3, false); changed {
		t.Error("not linked should not change")
	}
}

func TestParseSensorIDs(t *testing.T) {
	t.Parallel()
	if ids, ok := parseSensorIDs("3, 1,3,,2"); !ok || len(ids) != 3 || ids[0] != 3 || ids[2] != 2 {
		t.Errorf("parse: %v %v", ids, ok)
	}
	for _, bad := range []string{"", "a", "1,-2", "0"} {
		if _, ok := parseSensorIDs(bad); ok {
			t.Errorf("parseSensorIDs(%q) should fail", bad)
		}
	}
}

func TestThinSeries(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	var pts []seriesPoint
	for i := 0; i < 3600; i++ { // one a second
		v := float64(i % 10)
		pts = append(pts, seriesPoint{t: start.Add(time.Duration(i) * time.Second), avg: v, peak: v})
	}
	avg := thinSeries(pts, start, end, 60, false)
	if len(avg) != 60 {
		t.Fatalf("want 60 points, got %d", len(avg))
	}
	if avg[0][0] != float64(start.UnixMilli()) || avg[0][1] != 4.5 {
		t.Errorf("first slot = %v, want start / 4.5", avg[0])
	}
	peak := thinSeries(pts, start, end, 60, true)
	if peak[5][1] != 9 {
		t.Errorf("port slot = %v, want the highest value 9", peak[5])
	}
	few := []seriesPoint{pts[0], pts[1000], pts[2000]}
	if got := thinSeries(few, start, end, 600, false); len(got) != 3 {
		t.Errorf("few points stay as they are, got %d", len(got))
	}
	if got := thinSeries(nil, start, end, 600, false); got == nil || len(got) != 0 {
		t.Errorf("no points = empty list, got %v", got)
	}
}

func TestACIFoundSensors(t *testing.T) {
	t.Parallel()
	resp := types.ACIResponse{Data: []types.ACIDeviceData{{
		DevCode: "DEV1",
		DeviceInfo: types.ACIDeviceInfo{
			Sensors: []types.ACISensor{{SensorType: 0, AccessPort: 1}, {SensorType: 3, AccessPort: 1}},
			Ports:   []types.ACIPort{{Port: 2, PortName: "Inline fan"}},
		},
	}}}
	got := map[string]foundSensor{}
	for _, f := range aciFoundSensors(resp) {
		got[f.Type] = f
	}
	for _, typ := range []string{"ACI.tempF", "ACI.humidity", "ACI.tempC", "ACI.1.0", "ACI.1.3", "ACIP.2"} {
		if _, ok := got[typ]; !ok {
			t.Errorf("missing %s in %v", typ, got)
		}
	}
	if f := got["ACIP.2"]; f.Name != "Inline fan" || f.Unit != "%" || f.Source != "acinfinity" || f.Device != "DEV1" {
		t.Errorf("port = %+v", f)
	}
	if f := got["ACI.1.3"]; f.Unit != "kPa" {
		t.Errorf("VPD unit = %q", f.Unit)
	}
}

func TestEcowittFoundSensors(t *testing.T) {
	t.Parallel()
	resp := types.ECWAPIResponse{CHSoil: []types.ECWCHSoil{{Channel: "1", Name: "Soil 1"}}}
	got := map[string]foundSensor{}
	for _, f := range ecowittFoundSensors("192.168.1.40", resp) {
		got[f.Type] = f
	}
	for _, typ := range []string{"WH25.InTemp", "WH25.InHumi", "Soil.1"} {
		if f, ok := got[typ]; !ok || f.Device != "192.168.1.40" || f.Source != "ecowitt" {
			t.Errorf("%s = %+v (found %v)", typ, f, ok)
		}
	}
	if got["Soil.1"].Name != "Soil 1" {
		t.Errorf("soil name = %q", got["Soil.1"].Name)
	}
}
