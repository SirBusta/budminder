package climate

import (
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

func TestBand(t *testing.T) {
	t.Parallel()
	r := Range{Low: f(1.20), Target: f(1.35), High: f(1.50)}
	cases := []struct {
		v    float64
		want State
	}{
		{1.35, OK},
		{1.30, OK},      // 0.05 below, half of 0.15 is 0.075
		{1.275, OK},     // exactly half way counts as near
		{1.25, Toward},  // more than half way to Low
		{1.20, Reached}, // reaching Low counts
		{1.10, Reached},
		{1.42, OK},
		{1.45, Toward},
		{1.50, Reached}, // reaching High counts
	}
	for _, c := range cases {
		if got := Band(c.v, r); got != c.want {
			t.Errorf("Band(%v) = %q, want %q", c.v, got, c.want)
		}
	}
	if got := Band(1, Range{}); got != None {
		t.Errorf("empty range = %q", got)
	}
	if got := Band(0.5, Range{Low: f(1), High: f(2)}); got != Reached {
		t.Errorf("no target below low = %q", got)
	}
	if got := Band(1.5, Range{Low: f(1), High: f(2)}); got != OK {
		t.Errorf("no target inside = %q", got)
	}
}

func TestLimits(t *testing.T) {
	t.Parallel()
	if Limits(25, f(25), nil) != Reached || Limits(30, nil, f(30)) != Reached {
		t.Error("reaching a limit should be red")
	}
	if Limits(26, f(25), f(30)) != None || Limits(5, nil, nil) != None {
		t.Error("inside limits shows no color")
	}
	if Worse(OK, Reached) != Reached || Worse(Toward, None) != Toward {
		t.Error("Worse")
	}
}

var chicago, _ = time.LoadLocation("America/Chicago")

func day(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func TestStage(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, chicago)
	flower := Plant{ID: 1, Status: "Flower", StatusOrder: 5, Since: day("2026-09-20")} // day 40: week 6
	flower2 := Plant{ID: 2, Status: "Flower", StatusOrder: 5, Since: day("2026-10-20")}
	veg := Plant{ID: 3, Status: "Veg", StatusOrder: 4, Since: day("2026-10-01")}
	dry := Plant{ID: 4, Status: "Drying", StatusOrder: 6, Since: day("2026-10-25")}
	germ := Plant{ID: 5, Status: "Germinating", StatusOrder: 1, Since: day("2026-10-28")}
	dead := Plant{ID: 6, Status: "Dead", StatusOrder: 9, Since: day("2026-10-28")}

	zs := Stage([]Plant{flower2, flower}, 0, now, chicago)
	if zs.Stage != Flower || zs.Week != 6 || zs.Plant.ID != 1 || zs.Mixed {
		t.Errorf("all flower: %+v", zs)
	}

	// Drying is skipped while others grow; Dead never counts.
	zs = Stage([]Plant{dry, veg, flower, dead}, 0, now, chicago)
	if zs.Stage != Flower || !zs.Mixed || len(zs.Plants) != 2 {
		t.Errorf("mixed: %+v", zs)
	}
	zs = Stage([]Plant{dry, veg, flower}, 3, now, chicago)
	if zs.Stage != Veg || zs.Left {
		t.Errorf("chosen veg plant: %+v", zs)
	}
	// The chosen plant left: back to the furthest along, with a note.
	zs = Stage([]Plant{veg, flower}, 99, now, chicago)
	if zs.Stage != Flower || !zs.Left {
		t.Errorf("chosen plant left: %+v", zs)
	}
	// Only drying plants left: the zone follows them.
	zs = Stage([]Plant{dry}, 0, now, chicago)
	if zs.Stage != Drying || zs.Week != 1 {
		t.Errorf("drying only: %+v", zs)
	}
	if zs = Stage([]Plant{germ}, 0, now, chicago); zs.Stage != Seedling {
		t.Errorf("germinating = seedling: %+v", zs)
	}
	if zs = Stage(nil, 0, now, chicago); zs.Stage != "" || zs.Plant != nil {
		t.Errorf("empty: %+v", zs)
	}
}

func TestPhase(t *testing.T) {
	t.Parallel()
	for w, want := range map[int]string{1: Stretch, 4: Stretch, 5: Bloom, 8: Bloom, 9: Ripen, 15: Ripen} {
		if got := Phase(w, 4, 8); got != want {
			t.Errorf("Phase(%d) = %s, want %s", w, got, want)
		}
	}
}

func TestLightsOn(t *testing.T) {
	t.Parallel()
	s := Schedule{Flower: {"06:00", "18:00"}, Veg: {"18:00", "12:00"}, Seedling: {"06:00", "06:00"}}
	at := func(h, m int) time.Time { return time.Date(2026, 6, 1, h, m, 0, 0, chicago) }
	cases := []struct {
		stage string
		t     time.Time
		on    bool
	}{
		{Flower, at(5, 59), false}, {Flower, at(6, 0), true}, {Flower, at(17, 59), true}, {Flower, at(18, 0), false},
		{Veg, at(23, 0), true}, {Veg, at(3, 0), true}, {Veg, at(12, 0), false}, {Veg, at(15, 0), false},
		{Seedling, at(2, 0), true},
	}
	for _, c := range cases {
		on, sched := LightsOn(s, c.stage, c.t, chicago)
		if on != c.on || !sched {
			t.Errorf("%s at %s: on=%v sched=%v", c.stage, c.t.Format("15:04"), on, sched)
		}
	}
	if on, sched := LightsOn(nil, Flower, at(2, 0), chicago); !on || sched {
		t.Error("no schedule: lights-on targets")
	}
	if on, _ := LightsOn(s, Drying, at(12, 0), chicago); on {
		t.Error("drying is lights off")
	}
	// Wall clock follows DST: 6:00 in Chicago is 11:00 UTC in summer, 12:00 UTC in winter.
	if on, _ := LightsOn(s, Flower, time.Date(2026, 7, 1, 11, 0, 0, 0, time.UTC), chicago); !on {
		t.Error("summer 6:00 CDT should be on")
	}
	if on, _ := LightsOn(s, Flower, time.Date(2026, 12, 1, 11, 30, 0, 0, time.UTC), chicago); on {
		t.Error("winter 5:30 CST should be off")
	}
}

func TestClock(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "6:00", "24:00", "12:60", "ab:cd", "12-00"} {
		if _, ok := ParseClock(bad); ok {
			t.Errorf("ParseClock(%q) accepted", bad)
		}
	}
	if Hours("06:00", "18:00") != 12 || Hours("18:00", "12:00") != 18 || Hours("06:00", "06:00") != 24 {
		t.Error("Hours")
	}
}
