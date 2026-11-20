package handlers

// Sensors > Export readings (built 2026-11-20 from mockup
// data/mockups/sensors/export/v6.html). GET /sensors/export/preview gives the
// headings, the first rows and the row count for the window; GET
// /sensors/export streams the CSV or Excel file. Every reading, 15 and 30
// minutes come from sensor_data (which Data Retention prunes); 1 hour and
// 1 day come from sensor_data_hourly, which is kept forever. Times are
// wall-clock times in the Settings time zone, and an interval starts on the
// clock (11:00, 11:15 ...).

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"

	"isley/logger"
	"isley/model"
	"isley/utils"
)

const (
	exportMaxSensors  = 500
	exportPreviewRows = 6
	// Excel's row limit; the heading row counts.
	xlsxMaxRows = 1048576
	// exportWall is how From, To and the first-reading times travel between
	// the page and the server: wall clock in the Settings time zone.
	exportWall = "2006-01-02T15:04"
)

var exportIntervals = map[int]bool{0: true, 15: true, 30: true, 60: true, 1440: true}

var errExportEnough = errors.New("enough rows")

// exportSensor is one chosen sensor, with the unit and decimals its readings
// show in on the Environment page.
type exportSensor struct {
	ID               int
	Name, Zone, Unit string
	dp               int
	conv             func(float64) float64
}

// exportReq is one export: what the window chose.
type exportReq struct {
	sensors  []exportSensor // in the order the window lists them
	index    map[int]int
	from, to time.Time // UTC; to includes its whole minute
	every    int       // minutes; 0 = every reading
	cols     bool      // one column per sensor
	xlsx     bool
	loc      *time.Location
	T        map[string]string
}

// exTime is a row's time cell; exNum a reading.
type exTime struct {
	t    time.Time
	date bool
}
type exNum struct {
	v  float64
	dp int
}

func (q *exportReq) t(key string) string {
	if s := q.T[key]; s != "" {
		return s
	}
	return key
}

// parseExportRequest reads ids, from, to, every, layout and format.
func parseExportRequest(c *gin.Context) (*exportReq, bool) {
	store := ConfigStoreFromContext(c)
	q := &exportReq{loc: settingsLocation(store), index: map[int]int{},
		T: utils.TranslationService.GetTranslations(utils.GetLanguage(c))}
	var ids []int
	for _, part := range strings.Split(c.Query("ids"), ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil || id <= 0 {
			return nil, false
		}
		if _, dup := q.index[id]; !dup {
			q.index[id] = len(ids)
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || len(ids) > exportMaxSensors {
		return nil, false
	}
	from, errF := time.ParseInLocation(exportWall, c.Query("from"), q.loc)
	to, errT := time.ParseInLocation(exportWall, c.Query("to"), q.loc)
	if errF != nil || errT != nil || !from.Before(to) {
		return nil, false
	}
	q.from, q.to = from.UTC(), to.Add(time.Minute-time.Second).UTC()
	every, err := strconv.Atoi(c.DefaultQuery("every", "0"))
	if err != nil || !exportIntervals[every] {
		return nil, false
	}
	q.every = every
	switch c.DefaultQuery("layout", "rows") {
	case "rows":
	case "cols":
		q.cols = true
	default:
		return nil, false
	}
	switch c.DefaultQuery("format", "csv") {
	case "csv":
	case "xlsx":
		q.xlsx = true
	default:
		return nil, false
	}

	ph, args := inPlaceholders(ids)
	rows, err := DBFromContext(c).Query(`SELECT s.id, s.name, s.type, COALESCE(s.unit, ''), COALESCE(z.name, '')
		FROM sensors s LEFT JOIN zones z ON z.id = s.zone_id WHERE s.id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	q.sensors = make([]exportSensor, len(ids))
	found := 0
	units := store.Units()
	for rows.Next() {
		var s exportSensor
		var typ string
		if rows.Scan(&s.ID, &s.Name, &typ, &s.Unit, &s.Zone) != nil {
			return nil, false
		}
		kind := sensorKind(typ, s.Name, s.Unit)
		s.dp = sensorDecimals(kind)
		s.Unit, s.conv = sensorDisplay(kind, s.Unit, units)
		q.sensors[q.index[s.ID]] = s
		found++
	}
	if rows.Err() != nil || found != len(ids) {
		return nil, false
	}
	return q, true
}

func (q *exportReq) ids() []int {
	out := make([]int, len(q.sensors))
	for i, s := range q.sensors {
		out[i] = s.ID
	}
	return out
}

// hourly says whether the file comes from the hourly averages.
func (q *exportReq) hourly() bool { return q.every >= 60 }

// headings is the file's first row, in the page's language.
func (q *exportReq) headings() []string {
	timeHead := "en_ex_h_start"
	switch q.every {
	case 0:
		timeHead = "en_ex_h_datetime"
	case 1440:
		timeHead = "en_ex_h_date"
	}
	out := []string{q.t(timeHead)}
	if q.cols {
		zones := map[string]bool{}
		for _, s := range q.sensors {
			zones[s.Zone] = true
		}
		for _, s := range q.sensors {
			h := s.Name
			if len(zones) > 1 && s.Zone != "" {
				h = s.Zone + " · " + h
			}
			if s.Unit != "" {
				h += " (" + s.Unit + ")"
			}
			out = append(out, h)
		}
		return out
	}
	out = append(out, q.t("en_ex_h_zone"), q.t("en_ex_h_sensor"))
	if q.every == 0 {
		out = append(out, q.t("en_ex_h_value"))
	} else {
		out = append(out, q.t("en_ex_h_avg"), q.t("en_ex_h_low"), q.t("en_ex_h_high"))
	}
	return append(out, q.t("en_ex_h_unit"))
}

// ----------------------------------------------------------------------
// Reading the data
// ----------------------------------------------------------------------

// exStat is one sensor's readings in one interval.
type exStat struct {
	sum, weight, lo, hi, last float64
	n                         int
}

func (s *exStat) add(avg, lo, hi, weight float64) {
	if s.n == 0 || lo < s.lo {
		s.lo = lo
	}
	if s.n == 0 || hi > s.hi {
		s.hi = hi
	}
	s.sum += avg * weight
	s.weight += weight
	s.last = avg
	s.n++
}

// intervalStart is the start of the interval a reading belongs to: on the
// clock in the Settings time zone (a day starts at midnight).
func (q *exportReq) intervalStart(t time.Time) time.Time {
	switch q.every {
	case 0:
		return t.Truncate(time.Minute)
	case 1440:
		l := t.In(q.loc)
		return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, q.loc).UTC()
	}
	_, off := t.In(q.loc).Zone()
	shift := time.Duration(off) * time.Second
	return t.Add(shift).Truncate(time.Duration(q.every) * time.Minute).Add(-shift)
}

// query returns the readings in [from, to], oldest first: sensor, time,
// average, lowest, highest and how many readings each row stands for.
func (q *exportReq) query(db *sql.DB, from, to time.Time) (*sql.Rows, error) {
	ph, args := inPlaceholders(q.ids())
	n := len(q.sensors)
	if q.hourly() {
		// An hour's bucket starts on the hour: take the one From falls in.
		args = append(args, from.Truncate(time.Hour).Format(utils.LayoutDB), to.Format(utils.LayoutDB))
		return db.Query(fmt.Sprintf(`SELECT sensor_id, bucket, avg_val, min_val, max_val, sample_count
			FROM sensor_data_hourly WHERE sensor_id IN (%s) AND bucket BETWEEN $%d AND $%d
			ORDER BY bucket, sensor_id`, ph, n+1, n+2), args...)
	}
	args = append(args, from.Format(utils.LayoutDB), to.Format(utils.LayoutDB))
	return db.Query(fmt.Sprintf(`SELECT sensor_id, create_dt, value, value, value, 1
		FROM sensor_data WHERE sensor_id IN (%s) AND create_dt BETWEEN $%d AND $%d
		ORDER BY create_dt, sensor_id`, ph, n+1, n+2), args...)
}

// walk sends the file's rows (after the headings) to emit, oldest first.
// emit may return errExportEnough to stop early.
func (q *exportReq) walk(db *sql.DB, from, to time.Time, emit func([]any) error) error {
	rows, err := q.query(db, from, to)
	if err != nil {
		return err
	}
	defer rows.Close()

	stats := make([]exStat, len(q.sensors))
	var cur time.Time
	open := false
	flush := func() error {
		if !open {
			return nil
		}
		open = false
		label := exTime{t: cur, date: q.every == 1440}
		if q.cols {
			row := make([]any, 1, len(stats)+1)
			row[0] = label
			any := false
			for i := range stats {
				if stats[i].n == 0 {
					row = append(row, nil)
					continue
				}
				v := stats[i].last
				if q.every > 0 {
					v = stats[i].sum / stats[i].weight
				}
				row = append(row, exNum{q.sensors[i].conv(v), q.sensors[i].dp})
				any = true
			}
			for i := range stats {
				stats[i] = exStat{}
			}
			if !any {
				return nil
			}
			return emit(row)
		}
		for i := range stats {
			st := stats[i]
			if st.n == 0 {
				continue
			}
			stats[i] = exStat{}
			s := q.sensors[i]
			if err := emit([]any{label, s.Zone, s.Name, exNum{s.conv(st.sum / st.weight), s.dp},
				exNum{s.conv(st.lo), s.dp}, exNum{s.conv(st.hi), s.dp}, s.Unit}); err != nil {
				return err
			}
		}
		return nil
	}

	for rows.Next() {
		var id, count int
		var at time.Time
		var avg, lo, hi float64
		if err := rows.Scan(&id, &at, &avg, &lo, &hi, &count); err != nil {
			return err
		}
		i, ok := q.index[id]
		if !ok {
			continue
		}
		s := q.sensors[i]
		if q.every == 0 && !q.cols {
			// Every reading, one row each: nothing to gather.
			if err := emit([]any{exTime{t: at}, s.Zone, s.Name, exNum{s.conv(avg), s.dp}, s.Unit}); err != nil {
				return err
			}
			continue
		}
		start := q.intervalStart(at)
		if !open || !start.Equal(cur) {
			if err := flush(); err != nil {
				return err
			}
			cur, open = start, true
		}
		stats[i].add(avg, lo, hi, float64(max(count, 1)))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return flush()
}

// count is how many rows the file has (after the headings), counted in the
// database. Intervals are counted with the time zone's offset now, so a day
// that changes clocks can make it differ by one from the file.
func (q *exportReq) count(db *sql.DB) (int, error) {
	ph, args := inPlaceholders(q.ids())
	n := len(q.sensors)
	table, col, from := "sensor_data", "create_dt", q.from
	if q.hourly() {
		table, col, from = "sensor_data_hourly", "bucket", q.from.Truncate(time.Hour)
	}
	args = append(args, from.Format(utils.LayoutDB), q.to.Format(utils.LayoutDB))
	where := fmt.Sprintf("FROM %s WHERE sensor_id IN (%s) AND %s BETWEEN $%d AND $%d", table, ph, col, n+1, n+2)

	var group string
	switch {
	case !q.cols && (q.every == 0 || q.every == 60):
		group = "" // one row per stored row
	case q.every == 60:
		group = col
	default:
		epoch := fmt.Sprintf("CAST(strftime('%%s', %s) AS INTEGER)", col)
		if model.IsPostgres() {
			epoch = fmt.Sprintf("CAST(EXTRACT(EPOCH FROM %s) AS BIGINT)", col)
		}
		_, off := time.Now().In(q.loc).Zone()
		size := max(q.every, 1) * 60
		group = fmt.Sprintf("(%s + %d) / %d", epoch, off, size)
	}
	var out int
	var err error
	if group == "" {
		err = db.QueryRow("SELECT COUNT(*) "+where, args...).Scan(&out)
	} else {
		if !q.cols {
			group = "sensor_id, " + group
		}
		err = db.QueryRow("SELECT COUNT(*) FROM (SELECT 1 AS one "+where+" GROUP BY "+group+") x", args...).Scan(&out)
	}
	return out, err
}

// firstAt is the time of the chosen sensors' first reading in the span.
func (q *exportReq) firstAt(db *sql.DB) (time.Time, bool, error) {
	ph, args := inPlaceholders(q.ids())
	n := len(q.sensors)
	table, col, from := "sensor_data", "create_dt", q.from
	if q.hourly() {
		table, col, from = "sensor_data_hourly", "bucket", q.from.Truncate(time.Hour)
	}
	args = append(args, from.Format(utils.LayoutDB), q.to.Format(utils.LayoutDB))
	var at time.Time
	err := db.QueryRow(fmt.Sprintf("SELECT %s FROM %s WHERE sensor_id IN (%s) AND %s BETWEEN $%d AND $%d ORDER BY %s LIMIT 1",
		col, table, ph, col, n+1, n+2, col), args...).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return at, false, nil
	}
	return at, err == nil, err
}

// ----------------------------------------------------------------------
// Writing it
// ----------------------------------------------------------------------

// texts is a row as text, as the CSV file and the preview show it.
func (q *exportReq) texts(row []any) []string {
	out := make([]string, len(row))
	for i, v := range row {
		switch v := v.(type) {
		case exTime:
			l := v.t.In(q.loc)
			if v.date {
				out[i] = l.Format("2006-01-02")
			} else {
				out[i] = l.Format("2006-01-02 15:04")
			}
		case exNum:
			out[i] = strconv.FormatFloat(roundTo(v.v, v.dp), 'f', v.dp, 64)
		case string:
			out[i] = v
		}
	}
	return out
}

func roundTo(v float64, dp int) float64 {
	p := math.Pow(10, float64(dp))
	r := math.Round(v*p) / p
	if r == 0 {
		return 0 // no "-0.0"
	}
	return r
}

// csvSafe keeps a spreadsheet program from reading a name as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func (q *exportReq) filename() string {
	from, to := q.from.In(q.loc).Format("2006-01-02"), q.to.In(q.loc).Format("2006-01-02")
	name := "budminder-readings-" + from
	if to != from {
		name += "-to-" + to
	}
	if q.xlsx {
		return name + ".xlsx"
	}
	return name + ".csv"
}

// SensorExportPreviewHandler answers the Export readings window:
// GET /sensors/export/preview (same query as the download).
func SensorExportPreviewHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SensorExportPreviewHandler")
	q, ok := parseExportRequest(c)
	if !ok {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	db := DBFromContext(c)
	count, err := q.count(db)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to count readings")
		apiInternalError(c, "api_sensor_query_failed")
		return
	}
	head := q.headings()
	sample := [][]string{}
	if count > 0 {
		first, found, err := q.firstAt(db)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to find the first reading")
			apiInternalError(c, "api_sensor_query_failed")
			return
		}
		if found {
			// Only a short stretch from the first reading is read: enough for
			// the first rows without sorting the whole span.
			span := time.Hour
			if q.every > 0 {
				span = time.Duration(q.every*(exportPreviewRows+2)) * time.Minute
			}
			to := first.Add(span)
			if to.After(q.to) {
				to = q.to
			}
			err = q.walk(db, first, to, func(r []any) error {
				sample = append(sample, q.texts(r))
				if len(sample) >= exportPreviewRows {
					return errExportEnough
				}
				return nil
			})
			if err != nil && !errors.Is(err, errExportEnough) {
				fieldLogger.WithError(err).Error("Failed to read readings")
				apiInternalError(c, "api_sensor_query_failed")
				return
			}
		}
	}
	// The file's size, from the CSV size of the rows shown.
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(head)
	w.Flush()
	headBytes := buf.Len()
	for _, r := range sample {
		_ = w.Write(r)
	}
	w.Flush()
	size := headBytes
	if len(sample) > 0 {
		size += (buf.Len() - headBytes) * count / len(sample)
	}
	c.JSON(http.StatusOK, gin.H{"head": head, "rows": sample, "count": count, "bytes": size, "xlsx_max": xlsxMaxRows})
}

// SensorExportHandler downloads the file: GET /sensors/export.
func SensorExportHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SensorExportHandler")
	q, ok := parseExportRequest(c)
	if !ok {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	db := DBFromContext(c)
	if q.xlsx {
		count, err := q.count(db)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to count readings")
			apiInternalError(c, "api_sensor_query_failed")
			return
		}
		if !ExportFitsExcel(count) {
			c.JSON(http.StatusBadRequest, gin.H{"error": strings.ReplaceAll(q.t("en_ex_xlsx_max"), "{n}", strconv.Itoa(xlsxMaxRows))})
			return
		}
		if err := q.writeXLSX(c, db); err != nil {
			fieldLogger.WithError(err).Error("Failed to export readings")
			apiInternalError(c, "api_sensor_query_failed")
		}
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, q.filename()))
	// A byte order mark: without it Excel reads the file as ANSI and shows
	// "°F" and "CO₂" garbled.
	_, _ = c.Writer.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(c.Writer)
	_ = w.Write(safeRow(q.headings()))
	n := 0
	err := q.walk(db, q.from, q.to, func(r []any) error {
		n++
		return w.Write(safeRow(q.texts(r)))
	})
	w.Flush()
	if err == nil {
		err = w.Error()
	}
	if err != nil {
		// The file has started: all that's left is to say so in the log.
		fieldLogger.WithError(err).Error("Readings CSV export stopped")
		return
	}
	fieldLogger.WithFields(logrus.Fields{"rows": n, "sensors": len(q.sensors)}).Info("Readings CSV exported")
}

// ExportFitsExcel says whether a file of count rows (and its heading row)
// fits in an Excel sheet.
func ExportFitsExcel(count int) bool { return count+1 <= xlsxMaxRows }

func safeRow(r []string) []string {
	for i := range r {
		r[i] = csvSafe(r[i])
	}
	return r
}

// writeXLSX builds the Excel file (one sheet, frozen heading row, real dates
// and numbers) and sends it.
func (q *exportReq) writeXLSX(c *gin.Context, db *sql.DB) error {
	f := excelize.NewFile()
	defer f.Close()
	sheet := q.t("en_ex_sheet")
	if len([]rune(sheet)) > 31 || strings.ContainsAny(sheet, `:\/?*[]`) {
		sheet = "Readings"
	}
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return err
	}
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	timeFmt := "yyyy-mm-dd hh:mm"
	if q.every == 1440 {
		timeFmt = "yyyy-mm-dd"
	}
	timeStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: &timeFmt})
	numStyles := map[int]int{}
	numStyle := func(dp int) int {
		if id, ok := numStyles[dp]; ok {
			return id
		}
		format := "0"
		if dp > 0 {
			format += "." + strings.Repeat("0", dp)
		}
		id, _ := f.NewStyle(&excelize.Style{CustomNumFmt: &format})
		numStyles[dp] = id
		return id
	}

	head := q.headings()
	_ = sw.SetColWidth(1, 1, 18)
	if q.cols {
		_ = sw.SetColWidth(2, len(head), 16)
	} else {
		_ = sw.SetColWidth(2, 3, 22)
		_ = sw.SetColWidth(4, len(head), 11)
	}
	if err := sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	cells := make([]any, len(head))
	for i, h := range head {
		cells[i] = excelize.Cell{StyleID: bold, Value: h}
	}
	if err := sw.SetRow("A1", cells); err != nil {
		return err
	}
	rowNo := 1
	err = q.walk(db, q.from, q.to, func(r []any) error {
		rowNo++
		out := make([]any, len(r))
		for i, v := range r {
			switch v := v.(type) {
			case exTime:
				// Excel has no time zones: write the wall-clock time.
				l := v.t.In(q.loc)
				out[i] = excelize.Cell{StyleID: timeStyle, Value: time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), 0, 0, time.UTC)}
			case exNum:
				out[i] = excelize.Cell{StyleID: numStyle(v.dp), Value: roundTo(v.v, v.dp)}
			default:
				out[i] = v
			}
		}
		cell, _ := excelize.CoordinatesToCellName(1, rowNo)
		return sw.SetRow(cell, out)
	})
	if err != nil {
		return err
	}
	if err := sw.Flush(); err != nil {
		return err
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, q.filename()))
	if err := f.Write(c.Writer); err != nil {
		logger.Log.WithError(err).Error("Failed to send readings XLSX")
		return nil
	}
	logger.Log.WithFields(logrus.Fields{"rows": rowNo - 1, "sensors": len(q.sensors)}).Info("Readings XLSX exported")
	return nil
}

// SensorExportPageData is what the Export readings window needs (signed in
// only): the time zone's name, now and each sensor's first reading as wall
// clock in that zone, and Data Retention in days.
func SensorExportPageData(c *gin.Context) gin.H {
	store := ConfigStoreFromContext(c)
	loc := settingsLocation(store)
	db := DBFromContext(c)
	tz := store.Timezone()
	if tz == "" {
		tz = time.Now().In(loc).Format("MST")
	}
	first := map[string]string{}
	rows, err := db.Query(`SELECT id FROM sensors`)
	if err == nil {
		var ids []int
		for rows.Next() {
			var id int
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			// Each with ORDER BY ... LIMIT 1 on its own index, so it stays
			// quick on a big table (and keeps the column's time type).
			var best time.Time
			for _, sq := range []string{
				`SELECT create_dt FROM sensor_data WHERE sensor_id = $1 ORDER BY create_dt LIMIT 1`,
				`SELECT bucket FROM sensor_data_hourly WHERE sensor_id = $1 ORDER BY bucket LIMIT 1`,
			} {
				var at time.Time
				if db.QueryRow(sq, id).Scan(&at) == nil && (best.IsZero() || at.Before(best)) {
					best = at
				}
			}
			if !best.IsZero() {
				first[strconv.Itoa(id)] = best.In(loc).Format(exportWall)
			}
		}
	}
	return gin.H{"tz": tz, "now": time.Now().In(loc).Format(exportWall),
		"retention": store.SensorRetention(), "first": first}
}
