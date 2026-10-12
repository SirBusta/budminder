package utils

import (
	"testing"
	"time"
)

func TestNormalizeUnits(t *testing.T) {
	t.Parallel()
	cases := map[string]string{"": UnitsImperial, "imperial": UnitsImperial, "metric": UnitsMetric, " Metric ": UnitsMetric, "bogus": UnitsImperial}
	for in, want := range cases {
		if got := NormalizeUnits(in); got != want {
			t.Errorf("NormalizeUnits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShowHeight(t *testing.T) {
	t.Parallel()
	cases := []struct{ text, units, want string }{
		{"90-150cm", UnitsImperial, "35-59 in"},
		{"90-150cm", "", "35-59 in"},
		{"200-250cm", UnitsImperial, "6.6-8.2 ft"},
		{"1.5 – 2 m", UnitsImperial, "4.9-6.6 ft"},
		{"150 cm", UnitsImperial, "59 in"},
		{"90-150cm", UnitsMetric, "90-150cm"},
		{"3-5 ft", UnitsMetric, "91-152 cm"},
		{"36 in", UnitsMetric, "91 cm"},
		{"36 in", UnitsImperial, "36 in"},
		{"Medium", UnitsImperial, "Medium"},
		{"90-150 furlongs", UnitsImperial, "90-150 furlongs"},
		{"", UnitsImperial, ""},
	}
	for _, tc := range cases {
		if got := ShowHeight(tc.text, tc.units); got != tc.want {
			t.Errorf("ShowHeight(%q, %q) = %q, want %q", tc.text, tc.units, got, tc.want)
		}
	}
}

func TestShowYield(t *testing.T) {
	t.Parallel()
	cases := []struct{ text, units, want string }{
		{"450-550 g/m²", UnitsImperial, "1.5-1.8 oz/ft²"},
		{"375-450 g/m2", UnitsImperial, "1.2-1.5 oz/ft²"},
		{"450,5 g/m2", UnitsImperial, "1.5 oz/ft²"},
		{"450-550 g/m²", UnitsMetric, "450-550 g/m²"},
		{"1.5 oz/ft²", UnitsMetric, "458 g/m²"},
		{"80-140 g/plant", UnitsImperial, "2.8-4.9 oz/plant"},
		{"80 to 140 g/plant", UnitsImperial, "2.8-4.9 oz/plant"},
		{"500-1000 g/plant", UnitsImperial, "1.1-2.2 lb/plant"},
		{"80-140 g/plant", UnitsMetric, "80-140 g/plant"},
		{"3-5 oz/plant", UnitsMetric, "85-142 g/plant"},
		{"2-3 lb/plant", UnitsMetric, "0.9-1.4 kg/plant"},
		{"3-5 oz/plant", UnitsImperial, "3-5 oz/plant"},
		{"High", UnitsImperial, "High"},
		{"", UnitsMetric, ""},
	}
	for _, tc := range cases {
		if got := ShowYield(tc.text, tc.units); got != tc.want {
			t.Errorf("ShowYield(%q, %q) = %q, want %q", tc.text, tc.units, got, tc.want)
		}
	}
}

func TestLongDate(t *testing.T) {
	t.Parallel()
	if got := LongDate("2025-03-04", nil); got != "March 4, 2025" {
		t.Errorf("LongDate default = %q", got)
	}
	de := map[string]string{"month_3": "März", "date_long": "{day}. {month} {year}"}
	if got := LongDate("2025-03-14", de); got != "14. März 2025" {
		t.Errorf("LongDate translated = %q", got)
	}
	if got := LongDate("sometime", nil); got != "sometime" {
		t.Errorf("LongDate unreadable = %q", got)
	}
}

func TestShortDate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	if got := shortDateAt("2026-12-02", nil, now); got != "Dec 2" {
		t.Errorf("this year = %q", got)
	}
	if got := shortDateAt("2025-04-20T10:00:00", nil, now); got != "Apr 20, 2025" {
		t.Errorf("other year = %q", got)
	}
	de := map[string]string{"month_short_3": "März", "date_short": "{day}. {month}", "date_short_year": "{day}. {month} {year}"}
	if got := shortDateAt("2026-03-14", de, now); got != "14. März" {
		t.Errorf("translated = %q", got)
	}
	if got := shortDateAt("soon", nil, now); got != "soon" {
		t.Errorf("unreadable = %q", got)
	}
}
