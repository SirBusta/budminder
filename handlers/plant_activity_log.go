package handlers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"

	"isley/logger"
)

// activityLogMaxExport caps the number of rows an export can stream in a
// single request as a safety net.
const activityLogMaxExport = 100000

// slugify produces a lowercase, filesystem-safe slug from a human name.
func slugify(name string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-', r == '_':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "plant"
	}
	return out
}

// exportRow is one notebook day (one plant, one date) in an export.
type exportRow struct {
	Date, Plant, Strain, Zone, Activities, Measurements, Note string
}

var exportHeaders = []string{"Date", "Plant", "Strain", "Zone", "Activities", "Measurements", "Note"}

func (r exportRow) cells() []string {
	return []string{r.Date, r.Plant, r.Strain, r.Zone, r.Activities, r.Measurements, r.Note}
}

// loadExportRows returns the notebook days to export, newest first, and a
// filename descriptor. Query params (all optional): plant_ids=1,2 or
// plant_id=1 (default every plant), zone_id, activity_id (any of; repeat or
// comma-separate), from / to (YYYY-MM-DD, inclusive), q (text in the note).
// Days with only photos are left out.
func loadExportRows(db *sql.DB, c *gin.Context) ([]exportRow, string, error) {
	plants, err := LoadNotebookPlants(db)
	if err != nil {
		return nil, "", err
	}
	meta := map[int]NotebookPlant{}
	for _, p := range plants {
		meta[p.ID] = p
	}

	ids := parseIDList(c.Query("plant_ids"))
	if id, err := strconv.Atoi(strings.TrimSpace(c.Query("plant_id"))); err == nil && id > 0 {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		for _, p := range plants {
			ids = append(ids, p.ID)
		}
	}
	if zone, err := strconv.Atoi(strings.TrimSpace(c.Query("zone_id"))); err == nil && zone > 0 {
		var inZone []int
		for _, id := range ids {
			if meta[id].ZoneID == zone {
				inZone = append(inZone, id)
			}
		}
		ids = inZone
	}
	wantTypes := map[int]bool{}
	for _, raw := range c.QueryArray("activity_id") {
		for _, id := range parseIDList(raw) {
			wantTypes[id] = true
		}
	}
	from, to := strings.TrimSpace(c.Query("from")), strings.TrimSpace(c.Query("to"))
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))

	days, err := LoadPlantNotebook(db, ids)
	if err != nil {
		return nil, "", err
	}
	var out []exportRow
	for _, d := range days {
		if len(d.Types) == 0 && d.Note == "" && len(d.Measurements) == 0 {
			continue
		}
		if (from != "" && d.Date < from) || (to != "" && d.Date > to) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(d.Note), q) {
			continue
		}
		if len(wantTypes) > 0 {
			hit := false
			for _, t := range d.Types {
				hit = hit || wantTypes[t.ActivityID]
			}
			if !hit {
				continue
			}
		}
		var acts, measures []string
		for _, t := range d.Types {
			if t.Count > 1 {
				acts = append(acts, fmt.Sprintf("%s ×%d", t.Name, t.Count))
			} else {
				acts = append(acts, t.Name)
			}
		}
		for _, m := range d.Measurements {
			measures = append(measures, strings.TrimSpace(m.Name+" "+strconv.FormatFloat(m.Value, 'f', -1, 64)+" "+m.Unit))
		}
		p := meta[d.PlantID]
		out = append(out, exportRow{Date: d.Date, Plant: p.Name, Strain: p.Strain, Zone: p.Zone,
			Activities: strings.Join(acts, ", "), Measurements: strings.Join(measures, "; "), Note: d.Note})
		if len(out) >= activityLogMaxExport {
			break
		}
	}

	suffix := "all-" + time.Now().Format("20060102")
	if len(ids) == 1 {
		if p, ok := meta[ids[0]]; ok {
			suffix = slugify(p.Name) + "-" + time.Now().Format("20060102")
		}
	}
	return out, suffix, nil
}

// ExportActivitiesCSV streams the notebook as CSV, one row per plant per
// day. Auth-gated via AddProtectedRoutes.
func ExportActivitiesCSV(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "ExportActivitiesCSV")
	rows, suffix, err := loadExportRows(DBFromContext(c), c)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to load notebook for CSV export")
		apiInternalError(c, "api_database_error")
		return
	}

	filename := "isley-notebook-" + suffix + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	w := csv.NewWriter(c.Writer)
	if err := w.Write(exportHeaders); err != nil {
		fieldLogger.WithError(err).Error("Failed to write CSV header")
		return
	}
	for _, r := range rows {
		if err := w.Write(r.cells()); err != nil {
			fieldLogger.WithError(err).Error("Failed to write CSV row")
			return
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fieldLogger.WithError(err).Error("CSV writer flushed with error")
	}
	fieldLogger.WithFields(logrus.Fields{"rows": len(rows), "filename": filename}).Info("Notebook CSV exported")
}

// ExportActivitiesXLSX streams the notebook as an xlsx workbook: one
// "Notebook" sheet, frozen header row, autofilter over the data range.
// Auth-gated via AddProtectedRoutes.
func ExportActivitiesXLSX(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "ExportActivitiesXLSX")
	rows, suffix, err := loadExportRows(DBFromContext(c), c)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to load notebook for XLSX export")
		apiInternalError(c, "api_database_error")
		return
	}

	f := excelize.NewFile()
	sheet := "Notebook"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to create xlsx sheet")
		apiInternalError(c, "api_database_error")
		return
	}
	if err := f.DeleteSheet("Sheet1"); err != nil {
		fieldLogger.WithError(err).Warn("Failed to delete default Sheet1 (continuing)")
	}
	f.SetActiveSheet(idx)

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		fieldLogger.WithError(err).Warn("Failed to build header style (continuing without)")
	}
	wrapStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})
	cols := []string{"A", "B", "C", "D", "E", "F", "G"}
	widths := []float64{12, 24, 22, 16, 24, 22, 60}
	for i, h := range exportHeaders {
		cell := cellRef(cols[i], 1)
		_ = f.SetCellValue(sheet, cell, h)
		if headerStyle != 0 {
			_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
		}
		_ = f.SetColWidth(sheet, cols[i], cols[i], widths[i])
	}
	for i, r := range rows {
		for j, v := range r.cells() {
			_ = f.SetCellValue(sheet, cellRef(cols[j], i+2), v)
		}
		if wrapStyle != 0 {
			_ = f.SetCellStyle(sheet, cellRef("G", i+2), cellRef("G", i+2), wrapStyle)
		}
	}
	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	if len(rows) > 0 {
		_ = f.AutoFilter(sheet, fmt.Sprintf("A1:G%d", len(rows)+1), nil)
	}

	filename := "isley-notebook-" + suffix + ".xlsx"
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := f.Write(c.Writer); err != nil {
		fieldLogger.WithError(err).Error("Failed to stream XLSX to response")
		return
	}
	fieldLogger.WithFields(logrus.Fields{"rows": len(rows), "filename": filename}).Info("Notebook XLSX exported")
}

// cellRef returns a cell reference like "A3" from ("A", 3).  excelize has
// CoordinatesToCellName but that requires integer columns; this wrapper is
// more ergonomic when columns are fixed labels.
func cellRef(col string, row int) string {
	return fmt.Sprintf("%s%d", col, row)
}
