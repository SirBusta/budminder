package utils

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Measurement systems for the "units" setting.
const (
	UnitsImperial = "imperial"
	UnitsMetric   = "metric"
)

// NormalizeUnits maps any stored value to a known system; imperial is the
// default for an empty or unknown value.
func NormalizeUnits(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), UnitsMetric) {
		return UnitsMetric
	}
	return UnitsImperial
}

// amountRe reads free text such as "90-150cm", "1.5 – 2 m", "450-550 g/m²"
// or "80 to 140 g/plant": one number or a range, then a unit.
var amountRe = regexp.MustCompile(`^\s*(\d+(?:[.,]\d+)?)\s*(?:(?:-|–|—|to)\s*(\d+(?:[.,]\d+)?))?\s*(.+?)\s*$`)

type amount struct {
	lo, hi float64 // hi == lo when the text is a single number
	single bool
	unit   string // lower-cased, spaces removed
}

func parseAmount(s string) (amount, bool) {
	m := amountRe.FindStringSubmatch(s)
	if m == nil {
		return amount{}, false
	}
	num := func(v string) (float64, bool) {
		f, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
		return f, err == nil
	}
	lo, ok := num(m[1])
	if !ok {
		return amount{}, false
	}
	a := amount{lo: lo, hi: lo, single: true}
	if m[2] != "" {
		if a.hi, ok = num(m[2]); !ok {
			return amount{}, false
		}
		a.single = false
	}
	u := strings.ToLower(strings.Join(strings.Fields(m[3]), ""))
	u = strings.ReplaceAll(u, "²", "2")
	u = strings.ReplaceAll(u, "^2", "2")
	a.unit = u
	return a, true
}

// formatAmount prints a value or range with the given decimals, dropping a
// trailing ".0" so whole numbers stay whole.
func formatAmount(lo, hi float64, single bool, decimals int, unit string) string {
	f := func(v float64) string {
		p := math.Pow(10, float64(decimals))
		return strconv.FormatFloat(math.Round(v*p)/p, 'f', -1, 64)
	}
	if single {
		return f(lo) + " " + unit
	}
	return f(lo) + "-" + f(hi) + " " + unit
}

// ShowHeight shows a strain's free-text height in the chosen system,
// e.g. "90-150cm" -> "35-59 in" (imperial). Text that can't be read, or
// that is already in the chosen system, is returned unchanged.
func ShowHeight(text, units string) string {
	a, ok := parseAmount(text)
	if !ok {
		return text
	}
	var cm float64
	metric := true
	switch a.unit {
	case "cm":
		cm = 1
	case "m":
		cm = 100
	case "mm":
		cm = 0.1
	case "in", "inch", "inches", `"`:
		cm, metric = 2.54, false
	case "ft", "feet", "foot", "'":
		cm, metric = 30.48, false
	default:
		return text
	}
	lo, hi := a.lo*cm, a.hi*cm
	if NormalizeUnits(units) == UnitsMetric {
		if metric {
			return text
		}
		return formatAmount(lo, hi, a.single, 0, "cm")
	}
	if !metric {
		return text
	}
	// Inches up to 6 ft, feet above that.
	if hi/2.54 < 72 {
		return formatAmount(lo/2.54, hi/2.54, a.single, 0, "in")
	}
	return formatAmount(lo/30.48, hi/30.48, a.single, 1, "ft")
}

const (
	gramsPerOunce    = 28.349523125
	gramsPerPound    = 453.59237
	squareFeetPerSqM = 10.7639104
	gm2PerOzFt2      = gramsPerOunce * squareFeetPerSqM // 1 oz/ft² in g/m²
)

// ShowYield shows a strain's free-text yield in the chosen system:
// per area (g/m² <-> oz/ft²) or per plant (g/plant <-> oz or lb/plant).
// Text that can't be read, or that is already in the chosen system, is
// returned unchanged.
func ShowYield(text, units string) string {
	a, ok := parseAmount(text)
	if !ok {
		return text
	}
	imperial := NormalizeUnits(units) == UnitsImperial
	switch a.unit {
	case "g/m2", "gm2", "g/sqm":
		if !imperial {
			return text
		}
		return formatAmount(a.lo/gm2PerOzFt2, a.hi/gm2PerOzFt2, a.single, 1, "oz/ft²")
	case "oz/ft2", "oz/sqft":
		if imperial {
			return text
		}
		return formatAmount(a.lo*gm2PerOzFt2, a.hi*gm2PerOzFt2, a.single, 0, "g/m²")
	}
	var grams float64
	metric := true
	switch a.unit {
	case "g/plant", "g", "gr/plant", "grams/plant":
		grams = 1
	case "kg/plant", "kg":
		grams = 1000
	case "oz/plant", "oz":
		grams, metric = gramsPerOunce, false
	case "lb/plant", "lbs/plant", "lb", "lbs":
		grams, metric = gramsPerPound, false
	default:
		return text
	}
	lo, hi := a.lo*grams, a.hi*grams
	if imperial {
		if !metric {
			return text
		}
		// Ounces up to 2 lb, pounds above that.
		if hi < 2*gramsPerPound {
			return formatAmount(lo/gramsPerOunce, hi/gramsPerOunce, a.single, 1, "oz/plant")
		}
		return formatAmount(lo/gramsPerPound, hi/gramsPerPound, a.single, 1, "lb/plant")
	}
	if metric {
		return text
	}
	if hi >= 1000 {
		return formatAmount(lo/1000, hi/1000, a.single, 1, "kg/plant")
	}
	return formatAmount(lo, hi, a.single, 0, "g/plant")
}

// MonthYear shows a "2006-01-02" date as month and year, e.g. "March 2025",
// taking the month name from translations ("month_1".."month_12") when
// present. Text that isn't such a date is returned unchanged.
func MonthYear(date string, lcl map[string]string) string {
	t, err := time.Parse(LayoutDate, date)
	if err != nil {
		return date
	}
	name := lcl["month_"+strconv.Itoa(int(t.Month()))]
	if name == "" {
		name = t.Month().String()
	}
	return name + " " + strconv.Itoa(t.Year())
}
