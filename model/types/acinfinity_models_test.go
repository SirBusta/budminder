package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Values from a real controller's devInfoListAll (owner's, 2026-11-16), and
// the fallbacks for responses without sensorPrecision.
func TestACISensor_Value(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    ACISensor
		want float64
	}{
		{"temperature, precision 3", ACISensor{SensorType: 0, SensorData: 7370, SensorPrecision: 3}, 73.70},
		{"VPD, precision 3", ACISensor{SensorType: 3, SensorData: 122, SensorPrecision: 3}, 1.22},
		{"CO2, precision 1", ACISensor{SensorType: ACICO2SensorType, SensorData: 588, SensorPrecision: 1}, 588},
		{"light, precision 2", ACISensor{SensorType: ACILightSensorType, SensorData: 760, SensorPrecision: 2}, 76},
		{"no precision: x100", ACISensor{SensorType: 2, SensorData: 5100}, 51},
		{"no precision: CO2 plain", ACISensor{SensorType: ACICO2SensorType, SensorData: 770}, 770},
		{"no precision: light x10", ACISensor{SensorType: ACILightSensorType, SensorData: 1000}, 100},
		{"nonsense precision falls back", ACISensor{SensorType: 0, SensorData: 7370, SensorPrecision: 40}, 73.70},
	}
	for _, c := range cases {
		assert.InDelta(t, c.want, c.s.Value(), 1e-9, c.name)
	}
}
