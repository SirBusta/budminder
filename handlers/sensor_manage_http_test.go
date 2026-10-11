package handlers_test

// The Environment / Manage sensors backend (handlers/sensor_manage.go):
// chart data, editing a sensor, adding scanned sensors, scan previews.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/model/types"
	"isley/tests/testutil"
)

const sensorManagePassword = "sensor-manage-pass"

type sensorEnv struct {
	db     *sql.DB
	server *testutil.TestServer
	client *testutil.Client
	token  string
}

func newSensorEnv(t *testing.T) *sensorEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, sensorManagePassword)
	c, token := server.LoginAndFetchCSRF(t, sensorManagePassword, "/settings")
	return &sensorEnv{db: db, server: server, client: c, token: token}
}

func (e *sensorEnv) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	_, err := e.db.Exec(q, args...)
	require.NoError(t, err)
}

type seriesResp struct {
	Series map[string][][2]float64 `json:"series"`
	Raw    bool                    `json:"raw"`
}

func (e *sensorEnv) series(t *testing.T, query string) (int, seriesResp) {
	t.Helper()
	resp := e.client.Get("/sensors/series?" + query)
	defer testutil.DrainAndClose(resp)
	var out seriesResp
	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	}
	return resp.StatusCode, out
}

func utcDB(tm time.Time) string { return tm.UTC().Format("2006-01-02 15:04:05") }

func TestSensorSeries_RawReadings(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	sid := testutil.SeedSensor(t, e.db, "acinfinity", "DEV1", "ACI.1.0")
	base := time.Now().UTC().Truncate(time.Minute).Add(-90 * time.Minute)
	for i, v := range []float64{70.5, 71, 72.25} {
		e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`,
			sid, v, utcDB(base.Add(time.Duration(i)*10*time.Minute)))
	}
	// Outside the range: ignored.
	e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 99, $2)`, sid, utcDB(base.Add(-3*time.Hour)))

	q := fmt.Sprintf("ids=%d&from=%d&to=%d", sid, base.Add(-time.Minute).UnixMilli(), base.Add(time.Hour).UnixMilli())
	code, out := e.series(t, q)
	require.Equal(t, http.StatusOK, code)
	assert.True(t, out.Raw)
	pts := out.Series[strconv.Itoa(sid)]
	require.Len(t, pts, 3)
	assert.Equal(t, float64(base.UnixMilli()), pts[0][0], "times are epoch ms")
	assert.Equal(t, 72.25, pts[2][1])
}

func TestSensorSeries_LongRangeUsesHourlyRollups(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	temp := testutil.SeedSensor(t, e.db, "acinfinity", "DEV1", "ACI.1.0")
	port := testutil.SeedSensor(t, e.db, "acinfinity", "DEV1", "ACIP.2")
	base := time.Now().UTC().Truncate(time.Hour).Add(-48 * time.Hour)
	for i := 0; i < 3; i++ {
		bucket := utcDB(base.Add(time.Duration(i) * time.Hour))
		e.exec(t, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES ($1, $2, 60, 80, 70, 60)`, temp, bucket)
		e.exec(t, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES ($1, $2, 0, 100, 25, 60)`, port, bucket)
	}
	q := fmt.Sprintf("ids=%d,%d&from=%d&to=%d", temp, port, base.Add(-time.Hour).UnixMilli(), time.Now().UnixMilli())
	code, out := e.series(t, q)
	require.Equal(t, http.StatusOK, code)
	assert.False(t, out.Raw, "over 24 hours comes from the rollups")
	require.Len(t, out.Series[strconv.Itoa(temp)], 3)
	assert.Equal(t, 70.0, out.Series[strconv.Itoa(temp)][0][1], "readings use the hourly average")
	assert.Equal(t, 100.0, out.Series[strconv.Itoa(port)][0][1], "ports use the hourly highest")
}

func TestSensorSeries_ThinsLongSeries(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	sid := testutil.SeedSensor(t, e.db, "acinfinity", "DEV1", "ACI.1.2")
	base := time.Now().UTC().Truncate(time.Hour).Add(-3 * time.Hour)
	tx, err := e.db.Begin()
	require.NoError(t, err)
	for i := 0; i < 1500; i++ {
		_, err := tx.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`,
			sid, float64(i%7), utcDB(base.Add(time.Duration(i)*5*time.Second)))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	q := fmt.Sprintf("ids=%d&from=%d&to=%d", sid, base.UnixMilli(), base.Add(2*time.Hour+5*time.Minute).UnixMilli())
	code, out := e.series(t, q)
	require.Equal(t, http.StatusOK, code)
	n := len(out.Series[strconv.Itoa(sid)])
	assert.LessOrEqual(t, n, 600)
	assert.Greater(t, n, 100)
}

func TestSensorSeries_RejectsBadInput(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	now := time.Now().UnixMilli()
	for _, q := range []string{
		"",
		fmt.Sprintf("ids=abc&from=%d&to=%d", now-1000, now),
		fmt.Sprintf("ids=1&from=%d&to=%d", now, now-1000),
		"ids=1&from=x&to=y",
		fmt.Sprintf("ids=1&from=%d&to=%d", now-500*24*3600*1000, now),
	} {
		code, _ := e.series(t, q)
		assert.Equalf(t, http.StatusBadRequest, code, "query %q", q)
	}
	anon := e.server.NewClient(t)
	resp := anon.Get(fmt.Sprintf("/sensors/series?ids=1&from=%d&to=%d", now-1000, now))
	defer testutil.DrainAndClose(resp)
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, resp.StatusCode)
}

// livingPlant seeds a plant in zoneID whose latest status is name.
func livingPlant(t *testing.T, e *sensorEnv, name string, zoneID int, status string) int {
	t.Helper()
	strainID := testutil.SeedStrain(t, e.db, testutil.SeedBreeder(t, e.db, name+" breeder"), name+" strain")
	id := testutil.SeedPlant(t, e.db, name, strainID, zoneID)
	e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-09-01')`, id, statusIDByName(t, e.db, status))
	return id
}

func (e *sensorEnv) plantSensors(t *testing.T, plantID int) []int {
	t.Helper()
	var raw string
	require.NoError(t, e.db.QueryRow(`SELECT sensors FROM plant WHERE id = $1`, plantID).Scan(&raw))
	var ids []int
	require.NoError(t, json.Unmarshal([]byte(raw), &ids))
	return ids
}

func (e *sensorEnv) putSensor(t *testing.T, id int, body map[string]any) int {
	t.Helper()
	resp := e.client.SessionPutJSON(t, "/sensors/"+strconv.Itoa(id), e.token, body)
	defer testutil.DrainAndClose(resp)
	return resp.StatusCode
}

func TestUpdateSensor_SavesFieldsShowsOnAndPlantLinks(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	tentA := testutil.SeedZone(t, e.db, "Tent A")
	tentB := testutil.SeedZone(t, e.db, "Tent B")
	sid := testutil.SeedSensor(t, e.db, "acinfinity", "DEV1", "ACI.1.0")
	e.exec(t, `UPDATE sensors SET zone_id = $1 WHERE id = $2`, tentA, sid)
	growing := livingPlant(t, e, "Growing", tentB, "Flower")
	other := livingPlant(t, e, "Other", tentB, "Veg")
	dead := livingPlant(t, e, "Gone", tentB, "Dead")
	e.exec(t, `UPDATE plant SET sensors = $1 WHERE id = $2`, fmt.Sprintf("[%d]", sid), dead)
	e.exec(t, `UPDATE plant SET sensors = '[999]' WHERE id = $1`, other)

	require.Equal(t, http.StatusOK, e.putSensor(t, sid, map[string]any{
		"name": " Tent temp ", "zone_id": tentB, "unit": "°C",
		"show_env": false, "show_home": false, "hidden": false, "plant_ids": []int{growing},
	}))
	var name, unit, vis string
	var zone int
	var show bool
	require.NoError(t, e.db.QueryRow(`SELECT name, zone_id, unit, show, visibility FROM sensors WHERE id = $1`, sid).
		Scan(&name, &zone, &unit, &show, &vis))
	assert.Equal(t, "Tent temp", name)
	assert.Equal(t, tentB, zone)
	assert.Equal(t, "°C", unit)
	assert.False(t, show, "Environment unticked")
	assert.Equal(t, "plant", vis, "Home unticked, not hidden")
	assert.Equal(t, []int{sid}, e.plantSensors(t, growing))
	assert.Equal(t, []int{999}, e.plantSensors(t, other), "other plants keep their links")
	assert.Equal(t, []int{sid}, e.plantSensors(t, dead), "finished plants are left alone")

	// Unlink, show on Environment, hide everywhere; no tent.
	require.Equal(t, http.StatusOK, e.putSensor(t, sid, map[string]any{
		"name": "Tent temp", "zone_id": 0, "unit": "°C", "show_env": true, "show_home": true, "hidden": true, "plant_ids": []int{},
	}))
	var zoneNull sql.NullInt64
	require.NoError(t, e.db.QueryRow(`SELECT zone_id, show, visibility FROM sensors WHERE id = $1`, sid).Scan(&zoneNull, &show, &vis))
	assert.False(t, zoneNull.Valid)
	assert.True(t, show)
	assert.Equal(t, "hide", vis)
	assert.Empty(t, e.plantSensors(t, growing))

	require.Equal(t, http.StatusOK, e.putSensor(t, sid, map[string]any{"name": "Tent temp", "show_home": true}))
	require.NoError(t, e.db.QueryRow(`SELECT visibility FROM sensors WHERE id = $1`, sid).Scan(&vis))
	assert.Equal(t, "zone_plant", vis, "Home ticked")

	// Rejected: a finished plant, empty name, unknown tent, unknown sensor.
	assert.Equal(t, http.StatusBadRequest, e.putSensor(t, sid, map[string]any{"name": "x", "plant_ids": []int{dead}}))
	assert.Equal(t, http.StatusBadRequest, e.putSensor(t, sid, map[string]any{"name": "  "}))
	assert.Equal(t, http.StatusBadRequest, e.putSensor(t, sid, map[string]any{"name": "x", "zone_id": 9999}))
	assert.Equal(t, http.StatusNotFound, e.putSensor(t, 9999, map[string]any{"name": "x"}))
}

func TestUpdateSensor_NeedsCSRF(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	sid := testutil.SeedSensor(t, e.db, "acinfinity", "DEV1", "ACI.1.0")
	resp := e.client.SessionPutJSON(t, "/sensors/"+strconv.Itoa(sid), "wrong-token", map[string]any{"name": "x"})
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAddScannedSensor(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	add := func(body map[string]any) (int, int) {
		resp := e.client.SessionPostJSON(t, "/sensors/add", e.token, body)
		defer testutil.DrainAndClose(resp)
		var out struct {
			ID int `json:"id"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out.ID
	}
	body := map[string]any{"source": "acinfinity", "device": "DEV9", "type": "ACIP.3", "name": "Clip fan", "unit": "%", "new_zone": "New tent"}
	code, id := add(body)
	require.Equal(t, http.StatusOK, code)
	var zoneName, name string
	require.NoError(t, e.db.QueryRow(`SELECT z.name, s.name FROM sensors s JOIN zones z ON z.id = s.zone_id WHERE s.id = $1`, id).Scan(&zoneName, &name))
	assert.Equal(t, "New tent", zoneName)
	assert.Equal(t, "Clip fan", name)

	code, again := add(map[string]any{"source": "acinfinity", "device": "DEV9", "type": "ACIP.3", "name": "Clip fan", "unit": "%"})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, id, again, "adding twice is a no-op")
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM sensors WHERE device = 'DEV9'`).Scan(&n))
	assert.Equal(t, 1, n)

	code, _ = add(map[string]any{"source": "bogus", "device": "D", "type": "T", "name": "N"})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = add(map[string]any{"source": "ecowitt", "device": "D", "type": "T", "name": "N", "zone_id": 9999})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = add(map[string]any{"source": "ecowitt", "device": "D", "type": "T", "name": ""})
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestScanPreview_ListsOnlyNewSensorsAndSavesNothing(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedSensor(t, db, "acinfinity", "DEV1", "ACI.tempF")
	testutil.SeedSensor(t, db, "ecowitt", "192.168.1.40", "WH25.InTemp")
	count := func() int {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensors`).Scan(&n))
		return n
	}
	before := count()

	aci := handlers.PreviewACIScan(db, types.ACIResponse{Data: []types.ACIDeviceData{{
		DevCode: "DEV1",
		DeviceInfo: types.ACIDeviceInfo{
			Sensors: []types.ACISensor{{SensorType: 0, AccessPort: 1}},
			Ports:   []types.ACIPort{{Port: 1, PortName: "Light"}},
		},
	}}})
	assert.NotContains(t, aci, "acinfinity|DEV1|ACI.tempF", "already saved")
	assert.Contains(t, aci, "acinfinity|DEV1|ACI.1.0")
	assert.Contains(t, aci, "acinfinity|DEV1|ACIP.1")

	eco := handlers.PreviewEcowittScan(db, "192.168.1.40", types.ECWAPIResponse{CHSoil: []types.ECWCHSoil{{Channel: "2", Name: "Soil 2"}}})
	assert.NotContains(t, eco, "ecowitt|192.168.1.40|WH25.InTemp")
	assert.Contains(t, eco, "ecowitt|192.168.1.40|Soil.2")

	assert.Equal(t, before, count(), "a preview saves nothing")
}

func TestSensorPages_Render(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	zone := testutil.SeedZone(t, e.db, "Page Tent")
	sid := testutil.SeedSensor(t, e.db, "acinfinity", "DEV1", "ACI.1.0")
	e.exec(t, `UPDATE sensors SET zone_id = $1, name = 'Page temp', unit = '°F' WHERE id = $2`, zone, sid)
	e.exec(t, `INSERT INTO sensor_data (sensor_id, value) VALUES ($1, 71.5)`, sid)

	for _, path := range []string{"/sensors"} {
		resp := e.client.Get(path)
		body, err := io.ReadAll(resp.Body)
		testutil.DrainAndClose(resp)
		require.NoError(t, err)
		require.Equalf(t, http.StatusOK, resp.StatusCode, path)
		assert.Containsf(t, string(body), "Page temp", "%s carries the sensor list", path)
		assert.Containsf(t, string(body), "71.5", "%s carries the latest reading", path)
	}
	page := func(path string) string {
		resp := e.client.Get(path)
		defer testutil.DrainAndClose(resp)
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	assert.Contains(t, page("/sensors"), `id="envPage"`)
	assert.Contains(t, page("/sensors"), "sensors-page.js")

	anon := e.server.NewClient(t)
	resp := anon.Get("/sensors")
	defer testutil.DrainAndClose(resp)
	assert.NotContains(t, []int{http.StatusOK}, resp.StatusCode, "the Environment page needs a login")
}
