package homeassistant

import "strings"

// Source and Device identify Home Assistant sensors in BudMinder's sensors
// table; the sensor's type is its entity ID (e.g. sensor.tent_temperature).
const (
	Source = "homeassistant"
	Device = "homeassistant"
)

// growClasses are the Home Assistant device classes a grow cares about.
var growClasses = map[string]bool{
	"temperature": true, "humidity": true, "carbon_dioxide": true,
	"moisture": true, "illuminance": true, "conductivity": true,
}

// growUnits catch sensors without a device class (template sensors, VPD).
var growUnits = map[string]bool{
	"°c": true, "°f": true, "kpa": true, "ppm": true, "lx": true,
	"ms/cm": true, "µs/cm": true, "us/cm": true,
}

// IsSensor reports whether e is a numeric sensor entity BudMinder can read
// right now (sensor.* with a number for its state).
func IsSensor(e Entity) bool {
	if !strings.HasPrefix(e.EntityID, "sensor.") {
		return false
	}
	_, ok := e.Value()
	return ok
}

// IsGrowRelevant reports whether a sensor is one a grow usually wants
// (temperature, humidity, CO2, soil moisture, light, EC, VPD). Batteries,
// power, signal strength and the like are not; Manage sensors hides them
// unless "Show all" is on.
func IsGrowRelevant(e Entity) bool {
	dc := strings.ToLower(e.Attributes.DeviceClass)
	if dc == "battery" {
		return false
	}
	return growClasses[dc] || growUnits[strings.ToLower(strings.TrimSpace(e.Attributes.Unit))]
}

// ConvertTemperature converts v from one temperature unit to another
// ("°C" / "°F"), for a sensor whose unit in Home Assistant changed after it
// was added. Other units, or the same unit, return v unchanged.
func ConvertTemperature(v float64, from, to string) float64 {
	f, t := tempScale(from), tempScale(to)
	switch {
	case f == 'C' && t == 'F':
		return v*9/5 + 32
	case f == 'F' && t == 'C':
		return (v - 32) * 5 / 9
	}
	return v
}

func tempScale(unit string) byte {
	switch strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(unit, "°"))) {
	case "C":
		return 'C'
	case "F":
		return 'F'
	}
	return 0
}
