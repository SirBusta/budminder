package vivosun

import "strings"

// Source is the sensors.source value for Vivosun sensors.
const Source = "vivosun"

// Offer is one reading a scan offers to add.
type Offer struct {
	Key    string // Vivosun's name in the reading rows, e.g. "pTemp"
	Type   string // sensors.type: "VS." + Key, plus ".out" for air outside the device
	Suffix string // added to the device name: "temp", "room humidity", ...
	Unit   string
	// Note is a translation key for the gray line under the reading in the
	// scan ("" = none).
	Note string
	// Home: start with Home on (inside air). Room/outside air and the
	// electronics and coil temperatures start with it off.
	Home bool
}

// outSuffix marks a sensor type whose air is outside the device.
const outSuffix = ".out"

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
// also water temperature.
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
	{key: "outTemp", suffix: "outside temp", out: true},
	{key: "outHumi", suffix: "outside humidity", out: true},
	{key: "outVpd", suffix: "outside VPD", out: true},
	{key: "waterT", suffix: "water temp"},
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

// Offers lists the readings a row has that BudMinder can add, in display
// order. Wi-Fi signal, fan, compressor and other states are left out.
func Offers(values map[string]float64) []Offer {
	specs := otherOffers
	if IsCure(values) {
		specs = cureOffers
	}
	var out []Offer
	for _, s := range specs {
		if _, ok := values[s.key]; !ok {
			continue
		}
		t := "VS." + s.key
		if s.out {
			t += outSuffix
		}
		out = append(out, Offer{Key: s.key, Type: t, Suffix: s.suffix, Unit: units(s.key), Note: s.note, Home: s.home})
	}
	return out
}
