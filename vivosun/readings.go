package vivosun

import (
	"math"
	"strings"
)

// Source is the sensors.source value for Vivosun sensors.
const Source = "vivosun"

// Offer is one reading a scan offers to add.
type Offer struct {
	Key    string // Vivosun's name in the reading rows, e.g. "pTemp"
	Type   string // sensors.type: "VS." + Key, plus ".out" for air outside the device
	Suffix string // added to the device name: "temp", "room humidity", ...
	Unit   string
	// Value is the reading now, in Unit (see Value).
	Value float64
	// Note is a translation key for the gray line under the reading in the
	// scan ("" = none).
	Note string
	// Home: start with Home on (inside air). Room/outside air, water, the
	// electronics and coil temperatures and equipment start with it off.
	Home bool
	// More: equipment (lights, fans, outlets...), listed only with "Show
	// equipment".
	More bool
}

// outSuffix marks a sensor type whose air is outside the device.
const outSuffix = ".out"

// UnitOnOff is the unit of an equipment state that is only on or off (0/1).
const UnitOnOff = "on/off"

// KeyOf returns the reading-row name a sensor type reads ("VS.bTemp.out" ->
// "bTemp"), or "" for a type that isn't Vivosun's.
func KeyOf(sensorType string) string {
	if !strings.HasPrefix(sensorType, "VS.") {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(sensorType, "VS."), outSuffix)
}

type offerSpec struct {
	key, suffix, unit, note string
	home, out               bool
}

func units(key string) string {
	switch {
	case strings.HasSuffix(key, "Humi"):
		return "%"
	case strings.HasSuffix(key, "Vpd"):
		return "kPa"
	case key == "pCo2":
		return "ppm"
	case key == "waterLv":
		return "%"
	}
	return "°C" // Vivosun sends temperatures in °C whatever the app shows
}

// The VCure C80 (tested 2026-11-07 with the owner's warm-water test): p* is
// the air inside the box, as its screen and the app show; b* is the room
// air outside it. coreTemp is its circuit board and evapT its cooling coil.
var cureOffers = []offerSpec{
	{key: "pTemp", suffix: "temp", note: "sm_vs_note_in", home: true},
	{key: "pHumi", suffix: "humidity", note: "sm_vs_note_in", home: true},
	{key: "pVpd", suffix: "VPD", note: "sm_vs_note_in", home: true},
	{key: "bTemp", suffix: "room temp", note: "sm_vs_note_room", out: true},
	{key: "bHumi", suffix: "room humidity", note: "sm_vs_note_room", out: true},
	{key: "bVpd", suffix: "room VPD", note: "sm_vs_note_room", out: true},
	{key: "coreTemp", suffix: "electronics temp", note: "sm_vs_note_core"},
	{key: "evapT", suffix: "cooling coil temp", note: "sm_vs_note_evap"},
}

// Other models, from what they send (untested here): controllers have
// in*/out*, the VGrow and A22 have b* (built in) and p* (probe), the VGrow
// also CO2, water temperature and water level, the VSeed root temperature.
var otherOffers = []offerSpec{
	{key: "inTemp", suffix: "inside temp", home: true},
	{key: "inHumi", suffix: "inside humidity", home: true},
	{key: "inVpd", suffix: "inside VPD", home: true},
	{key: "bTemp", suffix: "temp", home: true},
	{key: "bHumi", suffix: "humidity", home: true},
	{key: "bVpd", suffix: "VPD", home: true},
	{key: "pTemp", suffix: "probe temp", home: true},
	{key: "pHumi", suffix: "probe humidity", home: true},
	{key: "pVpd", suffix: "probe VPD", home: true},
	{key: "pCo2", suffix: "CO₂", home: true},
	{key: "outTemp", suffix: "outside temp", out: true},
	{key: "outHumi", suffix: "outside humidity", out: true},
	{key: "outVpd", suffix: "outside VPD", out: true},
	{key: "rTemp", suffix: "root temp", note: "sm_vs_note_root", home: true},
	{key: "waterT", suffix: "water temp"},
	{key: "waterLv", suffix: "water level"},
}

// The VSeed's p* sensor is the air under its humidity dome.
var seedOffers = []offerSpec{
	{key: "pTemp", suffix: "dome temp", note: "sm_vs_note_dome", home: true},
	{key: "pHumi", suffix: "dome humidity", note: "sm_vs_note_dome", home: true},
	{key: "pVpd", suffix: "dome VPD", note: "sm_vs_note_dome", home: true},
	{key: "rTemp", suffix: "root temp", note: "sm_vs_note_root", home: true},
}

// Equipment: levels 0-100 (a light's spectrum is a color setting) and the
// VCure's compressor, which is 0 or 1. Vivosun sends these as they are, not
// times 100 like the readings.
var cureEquipment = []offerSpec{
	{key: "cure.comp", suffix: "compressor", unit: UnitOnOff, note: "sm_vs_note_comp"},
	{key: "cure.fanCir", suffix: "circulation fan", unit: "%"},
	{key: "cure.fanVap", suffix: "cooling coil fan", unit: "%"},
	{key: "cure.hmdf", suffix: "humidifier", unit: "%"},
	{key: "cure.ptc", suffix: "heater", unit: "%"},
}

var otherEquipment = []offerSpec{
	{key: "light.lv", suffix: "light", unit: "%"},
	{key: "light.spec", suffix: "light spectrum", unit: "%", note: "sm_vs_note_spec"},
	{key: "cFan.lv", suffix: "circulation fan", unit: "%"},
	{key: "dFan.lv", suffix: "duct fan", unit: "%"},
	{key: "usb1.lv", suffix: "USB 1", unit: "%"},
	{key: "usb2.lv", suffix: "USB 2", unit: "%"},
	{key: "plug1.lv", suffix: "outlet 1", unit: "%"},
	{key: "plug2.lv", suffix: "outlet 2", unit: "%"},
}

var equipmentKeys = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range append(append([]offerSpec{}, cureEquipment...), otherEquipment...) {
		m[s.key] = true
	}
	return m
}()

// IsEquipmentType tells whether a sensor type is a Vivosun device's
// equipment level or state (charted like an AC Infinity port).
func IsEquipmentType(sensorType string) bool {
	return equipmentKeys[KeyOf(sensorType)]
}

// IsCure tells a curing box (the VCure) by what its rows carry.
func IsCure(values map[string]float64) bool {
	if _, ok := values["evapT"]; ok {
		return true
	}
	for k := range values {
		if strings.HasPrefix(k, "cure.") {
			return true
		}
	}
	return false
}

// isSeed tells the VSeed seedling station by its model token.
func isSeed(model string) bool { return strings.HasPrefix(model, "VSSB") }

// co2Plain: a CO2 row value (already divided by 100) under this, times 100,
// was sent as plain ppm. Vivosun's scale for CO2 is unknown: air never has
// under 200 ppm, so x100 values start at 20000 and plain ones stay below.
const co2Plain = 20000

// Value turns a row value (Row divides everything by 100) into the reading
// in its unit. Equipment comes as plain 0-100 (or 0/1), CO2 maybe as plain
// ppm, and a humidifier's water level is x1000 (the Home Assistant
// project's rule; the VGrow's is x100 like the rest).
func Value(model, key string, v float64) float64 {
	switch {
	case equipmentKeys[key]:
		return math.Round(v * 100)
	case key == "pCo2":
		if v*100 < co2Plain {
			return math.Round(v * 100)
		}
		return math.Round(v)
	case key == "waterLv" && strings.HasPrefix(model, "VSHUM"):
		return v / 10
	}
	return v
}

// Offers lists the readings a row has that BudMinder can add, in display
// order: air, water and roots, electronics and coil, then equipment. model
// is the device's model token (Device.Model, "" when unknown). Wi-Fi signal,
// set points and other states are left out.
func Offers(model string, values map[string]float64) []Offer {
	specs, equip := otherOffers, otherEquipment
	switch {
	case IsCure(values):
		specs, equip = cureOffers, cureEquipment
	case isSeed(model):
		specs = seedOffers
	}
	var out []Offer
	add := func(s offerSpec, more bool) {
		v, ok := values[s.key]
		if !ok {
			return
		}
		t := "VS." + s.key
		if s.out {
			t += outSuffix
		}
		unit := s.unit
		if unit == "" {
			unit = units(s.key)
		}
		out = append(out, Offer{Key: s.key, Type: t, Suffix: s.suffix, Unit: unit, Value: Value(model, s.key, v),
			Note: s.note, Home: s.home, More: more})
	}
	for _, s := range specs {
		add(s, false)
	}
	for _, s := range equip {
		add(s, true)
	}
	return out
}
