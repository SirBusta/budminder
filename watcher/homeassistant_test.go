package watcher

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/homeassistant"
	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

// seedHASensor adds a Home Assistant sensor (as Manage sensors would) with
// the unit it had when it was added.
func seedHASensor(t *testing.T, db *sql.DB, entityID, unit string) int {
	t.Helper()
	id := seedSensor(t, db, homeassistant.Source, homeassistant.Device, entityID)
	_, err := db.Exec(`UPDATE sensors SET unit = $1 WHERE id = $2`, unit, id)
	require.NoError(t, err)
	return id
}

func haWatcher(t *testing.T, db *sql.DB, url, token string) *Watcher {
	t.Helper()
	w := newTestWatcher(t, db)
	w.HAEnabled = func() bool { return true }
	w.HAURL = func() string { return url }
	w.HAToken = func() string { return token }
	return w
}

func latestValue(t *testing.T, db *sql.DB, sensorID int) float64 {
	t.Helper()
	var v float64
	require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE sensor_id = $1 ORDER BY id DESC LIMIT 1`, sensorID).Scan(&v))
	return v
}

func TestPollHomeAssistant_SavesOnlyAddedSensors(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	ha := fakes.FakeHomeAssistant(t, "states")
	temp := seedHASensor(t, db, "sensor.tent_temperature", "°F")
	dark := seedHASensor(t, db, "sensor.tent_light_off", "lx")
	offline := seedHASensor(t, db, "sensor.garage_temperature", "°C")

	haWatcher(t, db, ha.URL, fakes.HomeAssistantToken).PollHomeAssistant(context.Background())

	assert.InDelta(t, 77.4, latestValue(t, db, temp), 0.001)
	assert.Equal(t, 1, countSensorData(t, db, dark), "a reading of 0 is a real reading")
	assert.Zero(t, latestValue(t, db, dark))
	assert.Equal(t, 0, countSensorData(t, db, offline), "unavailable sensors are skipped")

	var total, sensors int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data`).Scan(&total))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensors`).Scan(&sensors))
	assert.Equal(t, 2, total, "nothing saved for Home Assistant sensors that weren't added")
	assert.Equal(t, 3, sensors, "polling never creates sensors")
}

func TestPollHomeAssistant_ConvertsChangedTemperatureUnit(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	ha := fakes.FakeHomeAssistant(t, "states")
	// Added while Home Assistant reported °C; it now reports 77.4 °F.
	id := seedHASensor(t, db, "sensor.tent_temperature", "°C")

	haWatcher(t, db, ha.URL, fakes.HomeAssistantToken).PollHomeAssistant(context.Background())

	assert.InDelta(t, 25.222, latestValue(t, db, id), 0.001)
}

func TestPollHomeAssistant_NoSensorsAddedMeansNoRequest(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	ha := fakes.FakeHomeAssistant(t, "states")

	haWatcher(t, db, ha.URL, fakes.HomeAssistantToken).PollHomeAssistant(context.Background())

	assert.Zero(t, ha.Hits.Load())
}

func TestPollHomeAssistant_FailuresSaveNothing(t *testing.T) {
	t.Parallel()
	ha := fakes.FakeHomeAssistant(t, "states")
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()

	for name, c := range map[string]struct{ url, token string }{
		"wrong token":  {ha.URL, "wrong"},
		"offline":      {gone.URL, fakes.HomeAssistantToken},
		"no address":   {"", fakes.HomeAssistantToken},
		"no token yet": {ha.URL, ""},
	} {
		db := testutil.NewTestDB(t)
		id := seedHASensor(t, db, "sensor.tent_temperature", "°F")
		haWatcher(t, db, c.url, c.token).PollHomeAssistant(context.Background())
		assert.Equal(t, 0, countSensorData(t, db, id), name)
	}
}

func TestRun_PollsHomeAssistantOnlyWhenOn(t *testing.T) {
	t.Parallel()
	body := testutil.MustReadFixture(t, "homeassistant/states.json")

	for _, on := range []bool{true, false} {
		db := testutil.NewTestDB(t)
		id := seedHASensor(t, db, "sensor.tent_humidity", "%")

		synctest.Test(t, func(t *testing.T) {
			h := &stubHTTPDoer{body: body}
			w := haWatcher(t, db, "http://ha.example:8123", "tok")
			w.HAEnabled = func() bool { return on }
			w.HAHTTP = h
			w.PollingInterval = func() time.Duration { return time.Millisecond }

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				w.Run(ctx)
				close(done)
			}()
			synctest.Wait()
			cancel()
			<-done

			if on {
				assert.GreaterOrEqual(t, atomic.LoadInt32(&h.hits), int32(1))
			} else {
				assert.Zero(t, atomic.LoadInt32(&h.hits))
			}
		})
		if on {
			assert.InDelta(t, 55.2, latestValue(t, db, id), 0.001)
		} else {
			assert.Equal(t, 0, countSensorData(t, db, id))
		}
	}
}
