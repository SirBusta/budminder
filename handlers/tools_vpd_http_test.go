package handlers_test

// Tools > VPD chart (handlers/tools_vpd.go): the page, its menu item and
// each zone's stage, current reading and leaf temperature offset.

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type vpdPage struct {
	Units         string             `json:"units"`
	DefaultOffset float64            `json:"default_offset_f"`
	T             map[string]string  `json:"t"`
	Zones         []vpdPageZoneProbe `json:"zones"`
}

type vpdPageZoneProbe struct {
	Name    string   `json:"name"`
	Stage   string   `json:"stage"`
	Week    int      `json:"week"`
	TempC   *float64 `json:"temp_c"`
	RH      *float64 `json:"rh"`
	OffsetF *float64 `json:"offset_f"`
}

func vpdPageData(t *testing.T, body string) vpdPage {
	t.Helper()
	const open = `data-page='`
	i := strings.Index(body, open)
	require.GreaterOrEqual(t, i, 0, "page data")
	rest := body[i+len(open):]
	j := strings.Index(rest, `'`)
	require.GreaterOrEqual(t, j, 0)
	var p vpdPage
	require.NoError(t, json.Unmarshal([]byte(html.UnescapeString(rest[:j])), &p))
	return p
}

func TestVPDChart_Page(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	tent := testutil.SeedZone(t, e.db, "Tent")
	testutil.SeedZone(t, e.db, "Attic") // no plants, no sensors
	temp := testutil.SeedSensor(t, e.db, "acinfinity", "D1", "ACI.1.0")
	hum := testutil.SeedSensor(t, e.db, "acinfinity", "D1", "ACI.1.2")
	e.exec(t, `UPDATE sensors SET zone_id = $1, unit = '°F' WHERE id = $2`, tent, temp)
	e.exec(t, `UPDATE sensors SET zone_id = $1, unit = '%' WHERE id = $2`, tent, hum)
	now := utcDB(time.Now().Add(-time.Minute))
	e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 77, $2)`, temp, now)
	e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 55, $2)`, hum, now)
	e.exec(t, `UPDATE zones SET vpd_temp_sensor_id = $1, vpd_humidity_sensor_id = $2, vpd_source = 'calc', leaf_temp_offset = -3 WHERE id = $3`, temp, hum, tent)
	livingPlant(t, e, "Tent plant", tent, "Flower")

	// /tools opens the VPD chart.
	resp := e.client.Get("/tools")
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/tools/vpd", resp.Header.Get("Location"))

	body := e.page(t, "/tools/vpd")
	assert.Contains(t, body, `href="/tools" class="nav-link active"`, "the Tools menu item is marked")
	assert.Contains(t, body, `src="/static/js/tools-vpd.js"`)
	p := vpdPageData(t, body)
	assert.Equal(t, "imperial", p.Units)
	assert.InDelta(t, -2, p.DefaultOffset, 1e-9)
	assert.NotEmpty(t, p.T["vp_chart_title"])
	assert.NotEmpty(t, p.T["flower_label"])
	assert.NotEmpty(t, p.T["cl_week_n"])

	require.Len(t, p.Zones, 2)
	attic, z := p.Zones[0], p.Zones[1]
	assert.Equal(t, "Attic", attic.Name, "sorted by name")
	assert.Empty(t, attic.Stage)
	assert.Nil(t, attic.TempC)
	assert.Nil(t, attic.OffsetF)

	assert.Equal(t, "Tent", z.Name)
	assert.Equal(t, "flower", z.Stage)
	assert.Positive(t, z.Week)
	require.NotNil(t, z.TempC)
	assert.InDelta(t, 25, *z.TempC, 0.01, "77 °F in °C")
	require.NotNil(t, z.RH)
	assert.InDelta(t, 55, *z.RH, 1e-9)
	require.NotNil(t, z.OffsetF)
	assert.InDelta(t, -3, *z.OffsetF, 1e-9)

	// A stale reading is no reading.
	e.exec(t, `UPDATE sensor_data SET create_dt = $1`, utcDB(time.Now().Add(-6*time.Hour)))
	z = vpdPageData(t, e.page(t, "/tools/vpd")).Zones[1]
	assert.Nil(t, z.TempC)
	assert.Nil(t, z.RH)
}

func TestVPDChart_MetricUnits(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	e.server.ConfigStore.SetUnits("metric")
	assert.Equal(t, "metric", vpdPageData(t, e.page(t, "/tools/vpd")).Units)
}
