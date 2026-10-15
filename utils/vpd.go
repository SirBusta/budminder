package utils

import (
	"math"
	"strings"
)

// IsCelsius reports whether a sensor's temperature unit is Celsius, in any
// spelling or case: "°C", "C", "c", "celsius", "Celsius", "degC", "℃".
// Anything else (including "°F", "fahrenheit" and no unit) counts as
// Fahrenheit, BudMinder's default.
func IsCelsius(unit string) bool {
	u := strings.ToLower(strings.TrimSpace(unit))
	return u == "℃" || strings.Contains(u, "c")
}

// FahrenheitToCelsius converts a temperature from Fahrenheit to Celsius.
func FahrenheitToCelsius(f float64) float64 {
	return (f - 32.0) * 5.0 / 9.0
}

// saturationVaporPressure returns the saturation vapour pressure in kPa for
// a given temperature in °C using the Tetens approximation.
func saturationVaporPressure(tempC float64) float64 {
	return 0.6107 * math.Pow(10, 7.5*tempC/(237.3+tempC))
}

// VPD computes the Vapor Pressure Deficit in kPa.
//
// Parameters:
//   - airTempC   – air temperature in °C
//   - rhPct      – relative humidity in percent (0–100)
//   - leafOffsetC – leaf temperature offset relative to air temp, in °C
//     (typically negative, i.e. the leaf is cooler than the air)
//
// Formula:
//
//	VPD = es(airTempC + leafOffsetC) - es(airTempC) * rh/100
func VPD(airTempC, rhPct, leafOffsetC float64) float64 {
	esLeaf := saturationVaporPressure(airTempC + leafOffsetC)
	esAir := saturationVaporPressure(airTempC)
	return esLeaf - esAir*rhPct/100.0
}
