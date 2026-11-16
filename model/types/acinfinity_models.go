package types

type ACIResponse struct {
	// Code is 200 when AC Infinity accepted the request; otherwise Msg says
	// why (e.g. an expired or invalid token). Absent (0) in older fixtures.
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data []ACIDeviceData `json:"data"`
}

type ACIDeviceData struct {
	DevCode    string        `json:"devCode"`
	DeviceInfo ACIDeviceInfo `json:"deviceInfo"`
}

type ACIDeviceInfo struct {
	TemperatureF int         `json:"temperatureF"`
	Temperature  int         `json:"temperature"`
	Humidity     int         `json:"humidity"`
	Ports        []ACIPort   `json:"ports"`
	Sensors      []ACISensor `json:"sensors"`
	Unit         int         `json:"unit"` // 0 = °F, 1 = °C (added to allow unit-aware defaults)
}

type ACIPort struct {
	PortName string `json:"portName"`
	Speak    int    `json:"speak"`
	Port     int    `json:"port"`
	CurMode  int    `json:"curMode"`
	Online   int    `json:"online"`
}

type ACISensor struct {
	SensorType int `json:"sensorType"`
	AccessPort int `json:"accessPort"`
	SensorData int `json:"sensorData"`
	SensorUnit int `json:"sensorUnit"` // optional: unit reported per sensor
	// SensorPrecision says how to scale SensorData: divide by 10^(precision-1).
	// Seen on a real controller (2026-11-16): 3 for temperature, humidity and
	// VPD (7370 = 73.70 °F), 1 for CO2 (plain ppm), 2 for the light sensor
	// (0-100%). The Home Assistant AC Infinity project reads it the same way.
	SensorPrecision int `json:"sensorPrecision"`
}

// ACICO2SensorType is AC Infinity's CO2 sensor (plain ppm). Migration 036
// rescaled the history recorded before 1.0.1, which divided it by 100.
const ACICO2SensorType = 11

// ACILightSensorType is the light sensor sold with the CO2 sensor: 0-100%.
// Before 2.8 it was divided by 100 (0-10); migration 051 rescaled the history.
const ACILightSensorType = 12

// Value is the reading in the sensor's own unit. Without a precision (old
// responses, test fixtures), CO2 is plain, the light sensor x10, and the
// rest x100.
func (s ACISensor) Value() float64 {
	p := s.SensorPrecision
	if p < 1 || p > 6 {
		switch s.SensorType {
		case ACICO2SensorType:
			p = 1
		case ACILightSensorType:
			p = 2
		default:
			p = 3
		}
	}
	v := float64(s.SensorData)
	for ; p > 1; p-- {
		v /= 10
	}
	return v
}
