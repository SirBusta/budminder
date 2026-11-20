package handlers_test

// Sensors > Export readings (handlers/sensor_export.go): the preview and the
// CSV / Excel file, through the real engine. runExportChecks also runs on
// Postgres (sensor_export_postgres_test.go).

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"isley/handlers"
	"isley/tests/testutil"
)

type exportFixture struct {
	e         *sensorEnv
	temp, hum int // in "Tent A": °C stored, shown in °F; %
	fan       int // in "Veg": an AC Infinity port
	base      time.Time
	tent, veg string
}

// newExportFixture: Tent A has a temperature and a humidity sensor read every
// minute from 10:07 to 10:36 UTC on 2026-11-10 (temp 20, 21, 22, 20, ...°C;
// humidity 50); Veg has a fan port read every 5 minutes. Times are in UTC.
func newExportFixture(t *testing.T, e *sensorEnv) *exportFixture {
	t.Helper()
	e.server.ConfigStore.SetTimezone("UTC")
	e.server.ConfigStore.SetUnits("imperial")
	f := &exportFixture{e: e, tent: "Tent A", veg: "Veg", base: time.Date(2026, 11, 10, 10, 7, 0, 0, time.UTC)}
	zA := testutil.SeedZone(t, e.db, f.tent)
	zV := testutil.SeedZone(t, e.db, f.veg)
	f.temp = testutil.SeedSensor(t, e.db, "acinfinity", "D1", "ACI.1.temp")
	f.hum = testutil.SeedSensor(t, e.db, "acinfinity", "D1", "ACI.1.hum")
	f.fan = testutil.SeedSensor(t, e.db, "acinfinity", "D1", "ACIP.1.1")
	e.exec(t, `UPDATE sensors SET name = 'Inside temp', unit = '°C', zone_id = $1 WHERE id = $2`, zA, f.temp)
	e.exec(t, `UPDATE sensors SET name = 'Inside humidity', unit = '%', zone_id = $1 WHERE id = $2`, zA, f.hum)
	e.exec(t, `UPDATE sensors SET name = 'Inline fan', unit = '%', zone_id = $1 WHERE id = $2`, zV, f.fan)
	for i := 0; i < 30; i++ {
		at := utcDB(f.base.Add(time.Duration(i) * time.Minute))
		e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`, f.temp, 20+float64(i%3), at)
		e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`, f.hum, 50.0, at)
		if i%5 == 0 {
			e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`, f.fan, 40.0, at)
		}
	}
	return f
}

func (f *exportFixture) query(ids []int, every int, layout, format string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return url.Values{"ids": {strings.Join(parts, ",")}, "from": {"2026-11-10T10:00"}, "to": {"2026-11-10T11:00"},
		"every": {fmt.Sprint(every)}, "layout": {layout}, "format": {format}}.Encode()
}

type exportPreview struct {
	Head  []string   `json:"head"`
	Rows  [][]string `json:"rows"`
	Count int        `json:"count"`
	Bytes int        `json:"bytes"`
}

func (f *exportFixture) preview(t *testing.T, q string) exportPreview {
	t.Helper()
	resp := f.e.client.Get("/sensors/export/preview?" + q)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out exportPreview
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func (f *exportFixture) csv(t *testing.T, q string) [][]string {
	t.Helper()
	resp := f.e.client.Get("/sensors/export?" + q)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "budminder-readings-2026-11-10.csv")
	return readCSV(t, resp.Body)
}

// readCSV reads an exported CSV file, which starts with a byte order mark
// so Excel reads it as UTF-8.
func readCSV(t *testing.T, r io.Reader) [][]string {
	t.Helper()
	body, err := io.ReadAll(r)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(body, []byte("\xEF\xBB\xBF")), "byte order mark")
	rows, err := csv.NewReader(bytes.NewReader(body[3:])).ReadAll()
	require.NoError(t, err)
	return rows
}

// runExportChecks covers the file's contents for each interval and layout.
func runExportChecks(t *testing.T, e *sensorEnv) {
	f := newExportFixture(t, e)

	// Every reading, one row each: °C shown in °F, ordered by time.
	rows := f.csv(t, f.query([]int{f.temp, f.hum}, 0, "rows", "csv"))
	require.Len(t, rows, 61)
	assert.Equal(t, []string{"Date and time", "Zone", "Sensor", "Value", "Unit"}, rows[0])
	assert.Equal(t, []string{"2026-11-10 10:07", "Tent A", "Inside temp", "68.0", "°F"}, rows[1])
	assert.Equal(t, []string{"2026-11-10 10:07", "Tent A", "Inside humidity", "50.0", "%"}, rows[2])
	assert.Equal(t, "69.8", rows[3][3], "21 °C")

	// 15 minutes: on the clock (10:00, 10:15, 10:30), average, lowest, highest.
	rows = f.csv(t, f.query([]int{f.temp}, 15, "rows", "csv"))
	require.Len(t, rows, 4)
	assert.Equal(t, []string{"Interval start", "Zone", "Sensor", "Average", "Lowest", "Highest", "Unit"}, rows[0])
	// 10:07-10:14: 20,21,22,20,21,22,20,21 °C -> average 20.875 = 69.6 °F.
	assert.Equal(t, []string{"2026-11-10 10:00", "Tent A", "Inside temp", "69.6", "68.0", "71.6", "°F"}, rows[1])
	assert.Equal(t, "2026-11-10 10:15", rows[2][0])
	assert.Equal(t, "2026-11-10 10:30", rows[3][0])

	// One column per sensor, every reading: readings in the same minute share a
	// row; sensors from two zones get the zone in the heading.
	rows = f.csv(t, f.query([]int{f.temp, f.fan}, 0, "cols", "csv"))
	require.Len(t, rows, 31)
	assert.Equal(t, []string{"Date and time", "Tent A · Inside temp (°F)", "Veg · Inline fan (%)"}, rows[0])
	assert.Equal(t, []string{"2026-11-10 10:07", "68.0", "40"}, rows[1])
	assert.Equal(t, []string{"2026-11-10 10:08", "69.8", ""}, rows[2])

	// The preview counts what the file holds and shows its first rows.
	for _, c := range []struct {
		every  int
		layout string
		rows   int
	}{{0, "rows", 36}, {0, "cols", 30}, {15, "rows", 6}, {15, "cols", 3}, {30, "rows", 4}} {
		q := f.query([]int{f.temp, f.fan}, c.every, c.layout, "csv")
		p := f.preview(t, q)
		assert.Equalf(t, c.rows, p.Count, "every %d %s", c.every, c.layout)
		assert.Equalf(t, c.rows, len(f.csv(t, q))-1, "file rows, every %d %s", c.every, c.layout)
		assert.LessOrEqual(t, len(p.Rows), 6)
		assert.Positive(t, p.Bytes)
	}
	p := f.preview(t, f.query([]int{f.temp}, 0, "rows", "csv"))
	require.Len(t, p.Rows, 6)
	assert.Equal(t, []string{"2026-11-10 10:07", "Tent A", "Inside temp", "68.0", "°F"}, p.Rows[0])

	// 1 hour and 1 day come from the hourly averages, which outlive Data
	// Retention: here there are no raw readings that day at all.
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	e.exec(t, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES ($1, $2, 10, 12, 10, 1)`, f.hum, utcDB(day.Add(5*time.Hour)))
	e.exec(t, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES ($1, $2, 18, 30, 20, 3)`, f.hum, utcDB(day.Add(6*time.Hour)))
	q := url.Values{"ids": {fmt.Sprint(f.hum)}, "from": {"2026-11-03T00:00"}, "to": {"2026-11-03T23:59"}, "every": {"1440"}}.Encode()
	resp := e.client.Get("/sensors/export?" + q)
	daily := readCSV(t, resp.Body)
	testutil.DrainAndClose(resp)
	require.Len(t, daily, 2)
	assert.Equal(t, "Date", daily[0][0])
	// Weighted by readings: (10*1 + 20*3) / 4 = 17.5.
	assert.Equal(t, []string{"2026-11-03", "Tent A", "Inside humidity", "17.5", "10.0", "30.0", "%"}, daily[1])
	q = url.Values{"ids": {fmt.Sprint(f.hum)}, "from": {"2026-11-03T00:00"}, "to": {"2026-11-03T23:59"}, "every": {"60"}, "layout": {"cols"}}.Encode()
	assert.Equal(t, 2, f.preview(t, q).Count)
	q = url.Values{"ids": {fmt.Sprint(f.hum)}, "from": {"2026-11-03T00:00"}, "to": {"2026-11-03T23:59"}, "every": {"15"}}.Encode()
	assert.Equal(t, 0, f.preview(t, q).Count, "15 minutes reads only the kept readings")

	// Excel: a real workbook with dates and numbers.
	resp = e.client.Get("/sensors/export?" + f.query([]int{f.temp}, 0, "rows", "xlsx"))
	body, err := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Disposition"), ".xlsx")
	book, err := excelize.OpenReader(bytes.NewReader(body))
	require.NoError(t, err)
	defer book.Close()
	sheet := book.GetSheetName(0)
	assert.Equal(t, "Readings", sheet)
	got, _ := book.GetCellValue(sheet, "A1")
	assert.Equal(t, "Date and time", got)
	got, _ = book.GetCellValue(sheet, "A2")
	assert.Equal(t, "2026-11-10 10:07", got)
	got, _ = book.GetCellValue(sheet, "D2")
	assert.Equal(t, "68.0", got)
	typ, _ := book.GetCellType(sheet, "D2")
	assert.NotEqual(t, excelize.CellTypeSharedString, typ, "readings are numbers")
	all, err := book.GetRows(sheet)
	require.NoError(t, err)
	assert.Len(t, all, 31)
}

func TestSensorExport_Files(t *testing.T) {
	t.Parallel()
	runExportChecks(t, newSensorEnv(t))
}

// Times are wall clock in the Settings time zone, and a day starts at its midnight.
func TestSensorExport_SettingsTimeZone(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	f := newExportFixture(t, e)
	e.server.ConfigStore.SetTimezone("America/New_York")
	// 2026-11-10 03:30 UTC is 22:30 on 11/9 in New York.
	e.exec(t, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 55, $2)`, f.hum, "2026-11-10 03:30:00")
	q := url.Values{"ids": {fmt.Sprint(f.hum)}, "from": {"2026-11-09T22:00"}, "to": {"2026-11-09T23:00"}}.Encode()
	resp := e.client.Get("/sensors/export?" + q)
	rows := readCSV(t, resp.Body)
	testutil.DrainAndClose(resp)
	require.Len(t, rows, 2)
	assert.Equal(t, "2026-11-09 22:30", rows[1][0])
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "budminder-readings-2026-11-09.csv")
}

func TestSensorExport_NamesAreNotFormulas(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	f := newExportFixture(t, e)
	e.exec(t, `UPDATE sensors SET name = '=HYPERLINK("x")' WHERE id = $1`, f.hum)
	rows := f.csv(t, f.query([]int{f.hum}, 0, "rows", "csv"))
	assert.Equal(t, `'=HYPERLINK("x")`, rows[1][2])
}

func TestSensorExport_RejectsBadInput(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	f := newExportFixture(t, e)
	ok := url.Values{"ids": {fmt.Sprint(f.temp)}, "from": {"2026-11-10T10:00"}, "to": {"2026-11-10T11:00"}}
	for name, change := range map[string]func(v url.Values){
		"no sensors":     func(v url.Values) { v.Del("ids") },
		"unknown sensor": func(v url.Values) { v.Set("ids", "99999") },
		"bad id":         func(v url.Values) { v.Set("ids", "x") },
		"from after to":  func(v url.Values) { v.Set("from", "2026-11-10T12:00") },
		"bad date":       func(v url.Values) { v.Set("to", "1700000000000") },
		"bad interval":   func(v url.Values) { v.Set("every", "5") },
		"bad layout":     func(v url.Values) { v.Set("layout", "wide") },
		"bad format":     func(v url.Values) { v.Set("format", "pdf") },
	} {
		v := url.Values{}
		for k, x := range ok {
			v[k] = x
		}
		change(v)
		for _, path := range []string{"/sensors/export?", "/sensors/export/preview?"} {
			resp := e.client.Get(path + v.Encode())
			testutil.DrainAndClose(resp)
			assert.Equalf(t, http.StatusBadRequest, resp.StatusCode, "%s %s", path, name)
		}
	}
	anon := e.server.NewClient(t)
	resp := anon.Get("/sensors/export?" + ok.Encode())
	testutil.DrainAndClose(resp)
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, resp.StatusCode, "signed in only")
}

func TestSensorExport_ExcelRowLimit(t *testing.T) {
	t.Parallel()
	assert.True(t, handlers.ExportFitsExcel(1048575), "with the heading row: exactly full")
	assert.False(t, handlers.ExportFitsExcel(1048576))
}

// The page offers Export readings only to signed-in users, with each
// sensor's first reading for All dates.
func TestSensorExport_PageLink(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	f := newExportFixture(t, e)
	resp := e.client.Get("/sensors")
	body, _ := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	assert.Contains(t, string(body), `id="exOpen"`)
	assert.Contains(t, string(body), fmt.Sprintf(`&#34;%d&#34;:&#34;2026-11-10T10:07&#34;`, f.temp), "first reading in page data")

	e.server.ConfigStore.SetGuestMode(1)
	anon := e.server.NewClient(t)
	resp = anon.Get("/sensors")
	body, _ = io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	assert.NotContains(t, string(body), `id="exOpen"`)
}
