package homeassistant_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/homeassistant"
	"isley/tests/testutil/fakes"
)

func TestNormalizeURL(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"http://192.168.1.20:8123":           "http://192.168.1.20:8123",
		" http://homeassistant.local:8123/ ": "http://homeassistant.local:8123",
		"https://example.com/ha/":            "https://example.com/ha",
	} {
		got, err := homeassistant.NormalizeURL(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got)
	}
	for _, bad := range []string{"", "192.168.1.20:8123", "ftp://host", "http://", "http://host/?x=1", "http://user:pw@host"} {
		_, err := homeassistant.NormalizeURL(bad)
		assert.ErrorIs(t, err, homeassistant.ErrBadURL, bad)
	}
}

func TestNew_NeedsToken(t *testing.T) {
	t.Parallel()
	_, err := homeassistant.New("http://host:8123", "  ")
	assert.ErrorIs(t, err, homeassistant.ErrNoToken)
}

func TestClient_CheckAndStates(t *testing.T) {
	t.Parallel()
	ha := fakes.FakeHomeAssistant(t, "states")
	c, err := homeassistant.New(ha.URL, fakes.HomeAssistantToken)
	require.NoError(t, err)

	require.NoError(t, c.Check(context.Background()))
	states, err := c.States(context.Background())
	require.NoError(t, err)
	assert.Len(t, states, 14)
	assert.Equal(t, "sensor.tent_temperature", states[0].EntityID)
	assert.Equal(t, "°F", states[0].Attributes.Unit)
	assert.Equal(t, "Tent temperature", states[0].Name())
}

func TestClient_WrongToken(t *testing.T) {
	t.Parallel()
	ha := fakes.FakeHomeAssistant(t, "states")
	c, err := homeassistant.New(ha.URL, "not-the-token")
	require.NoError(t, err)
	assert.ErrorIs(t, c.Check(context.Background()), homeassistant.ErrUnauthorized)
	_, err = c.States(context.Background())
	assert.ErrorIs(t, err, homeassistant.ErrUnauthorized)
}

func TestClient_Unreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close() // nothing listens there now
	c, err := homeassistant.New(addr, "tok")
	require.NoError(t, err)
	err = c.Check(context.Background())
	assert.ErrorIs(t, err, homeassistant.ErrUnreachable)
	assert.NotContains(t, err.Error(), "tok", "errors never carry the token")
}

func TestClient_NotHomeAssistant(t *testing.T) {
	t.Parallel()
	// A web server that isn't Home Assistant: HTML on /api/, 404 elsewhere.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/" {
			_, _ = w.Write([]byte("<html>router login</html>"))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := homeassistant.New(srv.URL, "tok")
	require.NoError(t, err)
	assert.ErrorIs(t, c.Check(context.Background()), homeassistant.ErrNotHomeAssistant)
	_, err = c.States(context.Background())
	assert.ErrorIs(t, err, homeassistant.ErrNotHomeAssistant)
}

func TestClient_DoesNotFollowRedirects(t *testing.T) {
	t.Parallel()
	var followed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Store(true) }))
	t.Cleanup(target.Close)
	srv := httptest.NewServer(http.RedirectHandler(target.URL+"/api/", http.StatusFound))
	t.Cleanup(srv.Close)
	c, err := homeassistant.New(srv.URL, "tok")
	require.NoError(t, err)
	assert.ErrorIs(t, c.Check(context.Background()), homeassistant.ErrNotHomeAssistant)
	assert.False(t, followed.Load(), "the token must not be sent on to a redirect target")
}

func TestClient_TooLarge(t *testing.T) {
	t.Parallel()
	big := "[" + strings.Repeat(" ", homeassistant.MaxResponseBytes) + "]"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(big)) }))
	t.Cleanup(srv.Close)
	c, err := homeassistant.New(srv.URL, "tok")
	require.NoError(t, err)
	_, err = c.States(context.Background())
	assert.True(t, errors.Is(err, homeassistant.ErrTooLarge), "got %v", err)
}

func TestSensorFilters(t *testing.T) {
	t.Parallel()
	ha := fakes.FakeHomeAssistant(t, "states")
	c, err := homeassistant.New(ha.URL, fakes.HomeAssistantToken)
	require.NoError(t, err)
	states, err := c.States(context.Background())
	require.NoError(t, err)

	var sensors, grow []string
	for _, e := range states {
		if homeassistant.IsSensor(e) {
			sensors = append(sensors, e.EntityID)
			if homeassistant.IsGrowRelevant(e) {
				grow = append(grow, e.EntityID)
			}
		}
	}
	assert.ElementsMatch(t, []string{
		"sensor.tent_temperature", "sensor.tent_humidity", "sensor.tent_co2", "sensor.pot_1_soil_moisture",
		"sensor.tent_vpd", "sensor.tent_light", "sensor.tent_light_off", "sensor.tent_probe_battery", "sensor.dehumidifier_power",
	}, sensors, "numeric sensor.* only: not unavailable, text states, switches, sun or input_number")
	assert.ElementsMatch(t, []string{
		"sensor.tent_temperature", "sensor.tent_humidity", "sensor.tent_co2", "sensor.pot_1_soil_moisture",
		"sensor.tent_vpd", "sensor.tent_light", "sensor.tent_light_off",
	}, grow, "battery and power are hidden unless Show all is on")
}

func TestConvertTemperature(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 77.0, homeassistant.ConvertTemperature(25, "°C", "°F"), 0.001)
	assert.InDelta(t, 25.0, homeassistant.ConvertTemperature(77, "°F", "°C"), 0.001)
	assert.InDelta(t, 25.0, homeassistant.ConvertTemperature(25, "°C", "°C"), 0.001)
	assert.InDelta(t, 55.0, homeassistant.ConvertTemperature(55, "%", "°F"), 0.001)
}
