package handlers

// climateKind must agree with SensorKinds.kind in web/static/js/sensor-kinds.js,
// which fills Edit zone's climate sensor pickers.

import "testing"

func TestClimateKind_MatchesEditZonePickers(t *testing.T) {
	t.Parallel()
	cases := []struct{ typ, name, unit, want string }{
		{"ACI.1.0", "Inside Temp", "°F", "temp"},
		{"ACI.1.2", "Inside Humidity", "%", "hum"},
		{"ACI.1.3", "Inside VPD", "kPa", "vpd"},
		{"ACI.2.11", "CO2", "ppm", ""},
		{"ACIP.1.1", "Port 1 Light", "", ""},
		{"VS.light.lv", "Light", "%", ""}, // equipment level, not humidity
		{"VS.pTemp", "Probe", "°C", "temp"},
		{"VS.pTemp.out", "Room temp", "°C", "temp"},
		{"VS.waterLv", "Water level", "%", ""},
		{"sensor.tent_battery", "Tent battery", "%", ""},
		{"sensor.lux", "Lux", "lx", ""},
		{"sensor.soil", "Soil moisture", "%", ""},
		{"sensor.ec", "Feed EC", "mS/cm", ""},
		{"sensor.x", "Water temperature", "celsius", "temp"},
		{"sensor.dew", "Dew point", "℉", "temp"},
		{"sensor.rh", "Room", "%", "hum"},
		{"tempf", "Outdoor temp", "", "temp"},
		{"humidity", "Outdoor", "", "hum"},
		{"sensor.other", "Door", "", ""},
	}
	for _, c := range cases {
		if got := climateKind(c.typ, c.name, c.unit); got != c.want {
			t.Errorf("climateKind(%q, %q, %q) = %q, want %q", c.typ, c.name, c.unit, got, c.want)
		}
	}
}
