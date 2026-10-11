package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"isley/logger"
	"isley/model/types"
	"isley/utils"
)

// The plant notebook shows one row per plant per day. Each ticked activity
// type stays its own plant_activity row (with its own time, recorded but
// not shown); the day's single note lives in plant_day_entry. Days logged
// before plant_day_entry existed have their notes on the rows themselves —
// those are shown joined, and the first edit of that day moves the text
// into plant_day_entry. Dates are naive local-time values, so "the day" is
// simply the date part.

const dayLayout = "2006-01-02"

// inPlaceholders returns "$1, $2, …" for n values and the ids as args.
func inPlaceholders(ids []int) (string, []interface{}) {
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	return strings.Join(ph, ", "), args
}

func dayHasType(d *types.NotebookDay, activityID int) bool {
	for _, t := range d.Types {
		if t.ActivityID == activityID {
			return true
		}
	}
	return false
}

type dayKey struct {
	plantID int
	date    string
}

// LoadPlantNotebook returns the notebook days for the given plants, newest
// day first (ties by plant id). Not capped by PlantHistoryLimit: the
// notebook is the plant's whole history.
func LoadPlantNotebook(db *sql.DB, plantIDs []int) ([]types.NotebookDay, error) {
	if len(plantIDs) == 0 {
		return []types.NotebookDay{}, nil
	}
	in, args := inPlaceholders(plantIDs)
	days := map[dayKey]*types.NotebookDay{}
	day := func(plantID int, date string) *types.NotebookDay {
		k := dayKey{plantID, date}
		d, ok := days[k]
		if !ok {
			d = &types.NotebookDay{PlantID: plantID, Date: date, Types: []types.NotebookType{},
				Measurements: []types.NotebookMeasurement{}, Photos: []types.NotebookPhoto{}, Rows: []types.NotebookRow{}}
			days[k] = d
		}
		return d
	}
	addType := func(d *types.NotebookDay, id int, name, icon, color string) {
		for i := range d.Types {
			if d.Types[i].ActivityID == id {
				d.Types[i].Count++
				return
			}
		}
		d.Types = append(d.Types, types.NotebookType{ActivityID: id, Name: name, Icon: icon, Color: color, Count: 1})
	}

	// Activity types, for icons and for linking standalone measurements to
	// the type that records that metric (e.g. Height).
	allActs := GetActivities(db)
	actInfo := map[int]types.Activity{}
	for _, a := range allActs {
		actInfo[a.ID] = a
	}
	metricActivity := metricOwners(allActs)

	// 1. Activity rows. Sorted by time in Go, not SQL: old rows were stored
	// in mixed text formats ("…T08:10:00" vs "… 18:30:00"), which don't sort
	// correctly as text.
	type actRow struct {
		id, plantID, activityID int
		when                    time.Time
		note, name, icon, color string
	}
	var actRows []actRow
	rows, err := db.Query(fmt.Sprintf(`
		SELECT pa.id, pa.plant_id, pa.activity_id, pa.date, pa.note,
		       COALESCE(a.name, ''), COALESCE(a.icon, 'fa-tag'), COALESCE(a.color, '#64748b')
		FROM plant_activity pa
		LEFT OUTER JOIN activity a ON a.id = pa.activity_id
		WHERE pa.plant_id IN (%s)`, in), args...)
	if err != nil {
		return nil, fmt.Errorf("query activities: %w", err)
	}
	for rows.Next() {
		var r actRow
		if err := rows.Scan(&r.id, &r.plantID, &r.activityID, &r.when, &r.note, &r.name, &r.icon, &r.color); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		r.when = utils.AsLocal(r.when)
		actRows = append(actRows, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(actRows, func(i, j int) bool {
		if !actRows[i].when.Equal(actRows[j].when) {
			return actRows[i].when.Before(actRows[j].when)
		}
		return actRows[i].id < actRows[j].id
	})
	rowDay := map[int]*types.NotebookDay{}
	rowActivity := map[int]int{}
	legacyNotes := map[dayKey][]string{}
	for _, r := range actRows {
		d := day(r.plantID, r.when.Format(dayLayout))
		addType(d, r.activityID, r.name, r.icon, r.color)
		d.Rows = append(d.Rows, types.NotebookRow{ID: r.id, ActivityID: r.activityID, Time: r.when.Format("15:04:05")})
		rowDay[r.id] = d
		rowActivity[r.id] = r.activityID
		if n := strings.TrimSpace(r.note); n != "" {
			k := dayKey{r.plantID, d.Date}
			if prev := legacyNotes[k]; len(prev) == 0 || prev[len(prev)-1] != n {
				legacyNotes[k] = append(prev, n)
			}
		}
	}

	// 2. Day notes.
	rows, err = db.Query(fmt.Sprintf(`SELECT plant_id, entry_date, note FROM plant_day_entry WHERE plant_id IN (%s)`, in), args...)
	if err != nil {
		return nil, fmt.Errorf("query day entries: %w", err)
	}
	for rows.Next() {
		var plantID int
		var date, note string
		if err := rows.Scan(&plantID, &date, &note); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan day entry: %w", err)
		}
		d := day(plantID, date)
		d.Note, d.HasEntry = note, true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for k, notes := range legacyNotes {
		if d := days[k]; !d.HasEntry {
			d.Note = strings.Join(notes, "\n")
		}
	}

	// 3. Measurements: with their activity row, or standalone on their day.
	rows, err = db.Query(fmt.Sprintf(`
		SELECT pm.id, pm.plant_id, pm.metric_id, COALESCE(me.name, ''), COALESCE(me.unit, ''), pm.value, pm.date,
		       COALESCE(pm.plant_activity_id, 0)
		FROM plant_measurements pm
		LEFT OUTER JOIN metric me ON me.id = pm.metric_id
		WHERE pm.plant_id IN (%s)
		ORDER BY pm.date, pm.id`, in), args...)
	if err != nil {
		return nil, fmt.Errorf("query measurements: %w", err)
	}
	for rows.Next() {
		var m types.NotebookMeasurement
		var plantID, rowID int
		var when time.Time
		if err := rows.Scan(&m.ID, &plantID, &m.MetricID, &m.Name, &m.Unit, &m.Value, &when, &rowID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan measurement: %w", err)
		}
		d, linked := rowDay[rowID]
		if linked {
			m.ActivityID = rowActivity[rowID]
		} else {
			d = day(plantID, utils.AsLocal(when).Format(dayLayout))
			m.Standalone = true
			if aid, ok := metricActivity[m.MetricID]; ok {
				m.ActivityID = aid
				a := actInfo[aid]
				if !dayHasType(d, aid) {
					addType(d, aid, a.Name, a.Icon, a.Color)
				}
			}
		}
		d.Measurements = append(d.Measurements, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 4. Photos, by the date stored with each photo.
	rows, err = db.Query(fmt.Sprintf(`
		SELECT id, plant_id, image_path, COALESCE(image_description, ''), image_date
		FROM plant_images WHERE plant_id IN (%s)
		ORDER BY image_date, image_order, id`, in), args...)
	if err != nil {
		return nil, fmt.Errorf("query images: %w", err)
	}
	for rows.Next() {
		var p types.NotebookPhoto
		var plantID int
		var when time.Time
		if err := rows.Scan(&p.ID, &plantID, &p.Path, &p.Description, &when); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan image: %w", err)
		}
		p.Path = "/" + strings.ReplaceAll(p.Path, "\\", "/")
		d := day(plantID, utils.AsLocal(when).Format(dayLayout))
		d.Photos = append(d.Photos, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]types.NotebookDay, 0, len(days))
	for _, d := range days {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].PlantID < out[j].PlantID
	})
	return out, nil
}

// ---------------------------------------------------------------------------
// Saving a day
// ---------------------------------------------------------------------------

// dayMeasurementInput is a number typed into a ticked activity's box (e.g.
// Height 16). A nil Value means the box was left empty.
type dayMeasurementInput struct {
	ActivityID int      `json:"activity_id"`
	MetricID   int      `json:"metric_id"`
	Value      *float64 `json:"value"`
}

type dayInput struct {
	ActivityIDs  []int                 `json:"activity_ids"`
	Note         string                `json:"note"`
	Measurements []dayMeasurementInput `json:"measurements"`
}

type dayRow struct {
	id, activityID int
	stamp          string // stored date-time, reused for linked measurements
	note           string
}

type dayMeasurement struct {
	id, metricID, rowID int // rowID 0 = standalone
}

// validateDayInput checks the date, note, ticked types and their numbers,
// returning the de-duplicated ticks and a per-type metric→value map. The
// returned string is an API message key when the input is rejected.
func validateDayInput(date string, in *dayInput, acts map[int]types.Activity) ([]int, map[int]map[int]float64, string) {
	if _, err := time.Parse(dayLayout, date); err != nil {
		return nil, nil, "api_invalid_date"
	}
	if err := utils.ValidateStringLength("note", in.Note, utils.MaxNotesLength); err != nil {
		return nil, nil, "api_invalid_input"
	}
	in.Note = strings.TrimSpace(in.Note)
	seen := map[int]bool{}
	var ticks []int
	for _, id := range in.ActivityIDs {
		if _, ok := acts[id]; !ok {
			return nil, nil, "api_invalid_input"
		}
		if !seen[id] {
			seen[id] = true
			ticks = append(ticks, id)
		}
	}
	values := map[int]map[int]float64{}
	for _, m := range in.Measurements {
		if !seen[m.ActivityID] || m.Value == nil {
			continue
		}
		linked := false
		for _, l := range acts[m.ActivityID].Metrics {
			linked = linked || l.MetricID == m.MetricID
		}
		if !linked {
			return nil, nil, "api_invalid_input"
		}
		if values[m.ActivityID] == nil {
			values[m.ActivityID] = map[int]float64{}
		}
		values[m.ActivityID][m.MetricID] = *m.Value
	}
	for _, id := range ticks {
		for _, l := range acts[id].Metrics {
			if _, ok := values[id][l.MetricID]; l.Required && !ok {
				return nil, nil, "api_measurement_required"
			}
		}
	}
	return ticks, values, ""
}

func activityMap(db *sql.DB) map[int]types.Activity {
	m := map[int]types.Activity{}
	for _, a := range GetActivities(db) {
		m[a.ID] = a
	}
	return m
}

// sameDay matches a stored date-time column against a YYYY-MM-DD day ($2)
// on both SQLite (text, in mixed formats) and Postgres (TIMESTAMP).
func sameDay(col string) string {
	return "substr(CAST(" + col + " AS TEXT), 1, 10) = $2"
}

func loadDayRows(tx *sql.Tx, plantID int, date string) ([]dayRow, error) {
	rows, err := tx.Query(`SELECT id, activity_id, CAST(date AS TEXT), note FROM plant_activity
		WHERE plant_id = $1 AND `+sameDay("date")+` ORDER BY id`, plantID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dayRow
	for rows.Next() {
		var r dayRow
		if err := rows.Scan(&r.id, &r.activityID, &r.stamp, &r.note); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadDayMeasurements returns the measurements linked to the given rows plus
// the plant's standalone measurements dated on the day.
func loadDayMeasurements(tx *sql.Tx, plantID int, date string, dayRows []dayRow) ([]dayMeasurement, error) {
	inDay := map[int]bool{}
	for _, r := range dayRows {
		inDay[r.id] = true
	}
	rows, err := tx.Query(`SELECT id, metric_id, COALESCE(plant_activity_id, 0) FROM plant_measurements
		WHERE plant_id = $1 AND (plant_activity_id IS NOT NULL OR `+sameDay("date")+`)`, plantID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dayMeasurement
	for rows.Next() {
		var m dayMeasurement
		if err := rows.Scan(&m.id, &m.metricID, &m.rowID); err != nil {
			return nil, err
		}
		if m.rowID == 0 || inDay[m.rowID] {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

// stampFor is the stored date-time for a new row on the day: the day plus
// the current time of day (recorded, never shown).
func stampFor(date string) string {
	return date + "T" + time.Now().Format("15:04:05")
}

func insertDayRow(tx *sql.Tx, plantID, activityID int, stamp string) (int, error) {
	var id int
	err := tx.QueryRow(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, '', $3) RETURNING id`,
		plantID, activityID, stamp).Scan(&id)
	return id, err
}

func insertDayMeasurement(tx *sql.Tx, plantID, metricID int, value float64, stamp string, rowID int) error {
	_, err := tx.Exec(`INSERT INTO plant_measurements (plant_id, metric_id, value, date, plant_activity_id) VALUES ($1, $2, $3, $4, $5)`,
		plantID, metricID, value, stamp, rowID)
	return err
}

func deleteRowWithMeasurements(tx *sql.Tx, rowID int) error {
	if _, err := tx.Exec(`DELETE FROM plant_measurements WHERE plant_activity_id = $1`, rowID); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM plant_activity WHERE id = $1`, rowID)
	return err
}

// dayText is the day's current note: the day entry's if there is one,
// otherwise old rows' notes joined (the same rule the notebook shows).
func dayText(tx *sql.Tx, plantID int, date string, dayRows []dayRow) (string, bool, error) {
	var note string
	err := tx.QueryRow(`SELECT note FROM plant_day_entry WHERE plant_id = $1 AND entry_date = $2`, plantID, date).Scan(&note)
	if err == nil {
		return note, true, nil
	}
	if err != sql.ErrNoRows {
		return "", false, err
	}
	var parts []string
	for _, r := range dayRows {
		if n := strings.TrimSpace(r.note); n != "" && (len(parts) == 0 || parts[len(parts)-1] != n) {
			parts = append(parts, n)
		}
	}
	return strings.Join(parts, "\n"), false, nil
}

// writeDayNote stores the day's note and clears old rows' own notes so the
// text lives in one place. An empty day (nothing left) has no entry.
func writeDayNote(tx *sql.Tx, plantID int, date, note string, keepEntry bool) error {
	if _, err := tx.Exec(`UPDATE plant_activity SET note = '' WHERE plant_id = $1 AND note <> '' AND `+sameDay("date"), plantID, date); err != nil {
		return err
	}
	if !keepEntry {
		_, err := tx.Exec(`DELETE FROM plant_day_entry WHERE plant_id = $1 AND entry_date = $2`, plantID, date)
		return err
	}
	_, err := tx.Exec(`
		INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES ($1, $2, $3)
		ON CONFLICT (plant_id, entry_date) DO UPDATE SET note = excluded.note, updated_at = CURRENT_TIMESTAMP`,
		plantID, date, note)
	return err
}

// metricOwners maps each metric to the first activity type (by name, the
// order GetActivities uses) that records it — how standalone measurements
// are shown, the same rule as LoadPlantNotebook.
func metricOwners(acts []types.Activity) map[int]int {
	owner := map[int]int{}
	for _, a := range acts {
		for _, l := range a.Metrics {
			if _, taken := owner[l.MetricID]; !taken {
				owner[l.MetricID] = a.ID
			}
		}
	}
	return owner
}

func metricOwnersFromMap(acts map[int]types.Activity) map[int]int {
	list := make([]types.Activity, 0, len(acts))
	for _, a := range acts {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}
		return list[i].ID < list[j].ID
	})
	return metricOwners(list)
}

// saveDay makes the plant's day match the input: ticked types get (or keep)
// a row, unticked types lose theirs, numbers are updated, and the note is
// replaced.
func saveDay(tx *sql.Tx, plantID int, date string, ticks []int, values map[int]map[int]float64, note string, acts map[int]types.Activity) error {
	dayRows, err := loadDayRows(tx, plantID, date)
	if err != nil {
		return err
	}
	measures, err := loadDayMeasurements(tx, plantID, date, dayRows)
	if err != nil {
		return err
	}
	ticked := map[int]bool{}
	for _, id := range ticks {
		ticked[id] = true
	}
	rowsByType := map[int][]dayRow{}
	for _, r := range dayRows {
		if !ticked[r.activityID] {
			if err := deleteRowWithMeasurements(tx, r.id); err != nil {
				return err
			}
			continue
		}
		rowsByType[r.activityID] = append(rowsByType[r.activityID], r)
	}

	handled := map[int]bool{} // measurement ids updated, adopted or deleted
	for _, aid := range ticks {
		kept := rowsByType[aid]
		if len(kept) == 0 {
			stamp := stampFor(date)
			id, err := insertDayRow(tx, plantID, aid, stamp)
			if err != nil {
				return err
			}
			kept = []dayRow{{id: id, activityID: aid, stamp: stamp}}
		}
		primary := kept[0]
		ofType := map[int]bool{}
		for _, r := range kept {
			ofType[r.id] = true
		}
		for _, link := range acts[aid].Metrics {
			// The existing value: on one of this type's rows, else a
			// standalone one for this metric on the day (adopted).
			existing := 0
			for _, m := range measures {
				if handled[m.id] || m.metricID != link.MetricID || !(ofType[m.rowID] || m.rowID == 0) {
					continue
				}
				if existing == 0 || m.rowID != 0 {
					existing = m.id
				}
			}
			v, has := values[aid][link.MetricID]
			switch {
			case has && existing != 0:
				if _, err := tx.Exec(`UPDATE plant_measurements SET value = $1, plant_activity_id = $2 WHERE id = $3`, v, primary.id, existing); err != nil {
					return err
				}
				handled[existing] = true
			case has:
				if err := insertDayMeasurement(tx, plantID, link.MetricID, v, primary.stamp, primary.id); err != nil {
					return err
				}
			case existing != 0:
				if _, err := tx.Exec(`DELETE FROM plant_measurements WHERE id = $1`, existing); err != nil {
					return err
				}
				handled[existing] = true
			}
		}
	}

	// A standalone number counts as its type's tick on the day; unticking
	// that type removes it. Numbers for metrics no type records stay.
	owner := metricOwnersFromMap(acts)
	remaining := false
	for _, m := range measures {
		if m.rowID != 0 || handled[m.id] {
			continue
		}
		if aid, ok := owner[m.metricID]; ok && !ticked[aid] {
			if _, err := tx.Exec(`DELETE FROM plant_measurements WHERE id = $1`, m.id); err != nil {
				return err
			}
			continue
		}
		remaining = true
	}

	return writeDayNote(tx, plantID, date, note, len(ticks) > 0 || note != "" || remaining)
}

// appendDay adds ticks to a plant's day without touching what's there:
// every tick becomes a new row (the same type twice is recorded twice) and
// the note is appended on a new line. Used when logging several plants.
func appendDay(tx *sql.Tx, plantID int, date string, ticks []int, values map[int]map[int]float64, note string, acts map[int]types.Activity) error {
	dayRows, err := loadDayRows(tx, plantID, date)
	if err != nil {
		return err
	}
	current, hadEntry, err := dayText(tx, plantID, date, dayRows)
	if err != nil {
		return err
	}
	for _, aid := range ticks {
		stamp := stampFor(date)
		id, err := insertDayRow(tx, plantID, aid, stamp)
		if err != nil {
			return err
		}
		for _, link := range acts[aid].Metrics {
			if v, ok := values[aid][link.MetricID]; ok {
				if err := insertDayMeasurement(tx, plantID, link.MetricID, v, stamp, id); err != nil {
					return err
				}
			}
		}
	}
	text := current
	if note != "" {
		if text != "" {
			text += "\n"
		}
		text += note
	}
	return writeDayNote(tx, plantID, date, text, hadEntry || text != "" || len(ticks) > 0 || len(dayRows) > 0)
}

// deleteDay removes everything logged for the plant on the day (photos stay).
func deleteDay(tx *sql.Tx, plantID int, date string) error {
	dayRows, err := loadDayRows(tx, plantID, date)
	if err != nil {
		return err
	}
	for _, r := range dayRows {
		if err := deleteRowWithMeasurements(tx, r.id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM plant_measurements WHERE plant_id = $1 AND plant_activity_id IS NULL AND `+sameDay("date"), plantID, date); err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM plant_day_entry WHERE plant_id = $1 AND entry_date = $2`, plantID, date)
	return err
}

func plantExists(db *sql.DB, id int) bool {
	var one int
	return db.QueryRow(`SELECT 1 FROM plant WHERE id = $1`, id).Scan(&one) == nil
}

// dayOf returns the notebook row for one plant and day (nil when empty).
func dayOf(db *sql.DB, plantID int, date string) (*types.NotebookDay, error) {
	days, err := LoadPlantNotebook(db, []int{plantID})
	if err != nil {
		return nil, err
	}
	for i := range days {
		if days[i].Date == date {
			return &days[i], nil
		}
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func paramPlantID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	return id, err == nil && id > 0
}

// PlantNotebookHandler serves GET /plant/:id/notebook.
func PlantNotebookHandler(c *gin.Context) {
	id, ok := paramPlantID(c)
	if !ok {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	days, err := LoadPlantNotebook(DBFromContext(c), []int{id})
	if err != nil {
		logger.Log.WithError(err).WithField("plant_id", id).Error("Failed to load plant notebook")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days})
}

// SavePlantDayHandler serves PUT /plant/:id/day/:date: make that day match
// the window's ticks, numbers and note.
func SavePlantDayHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "SavePlantDayHandler")
	plantID, ok := paramPlantID(c)
	if !ok {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	date := c.Param("date")
	var in dayInput
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	db := DBFromContext(c)
	acts := activityMap(db)
	ticks, values, msg := validateDayInput(date, &in, acts)
	if msg != "" {
		apiBadRequest(c, msg)
		return
	}
	if !plantExists(db, plantID) {
		apiNotFound(c, "api_plant_not_found")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()
	if err := saveDay(tx, plantID, date, ticks, values, in.Note, acts); err != nil {
		fieldLogger.WithError(err).WithFields(logrus.Fields{"plant_id": plantID, "date": date}).Error("Failed to save day")
		apiInternalError(c, "api_failed_to_save_activity")
		return
	}
	if err := tx.Commit(); err != nil {
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}
	d, err := dayOf(db, plantID, date)
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	fieldLogger.WithFields(logrus.Fields{"plant_id": plantID, "date": date, "types": ticks}).Info("Plant day saved")
	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_day_saved"), "day": d})
}

// DeletePlantDayHandler serves DELETE /plant/:id/day/:date: remove
// everything logged that day (photos stay).
func DeletePlantDayHandler(c *gin.Context) {
	plantID, ok := paramPlantID(c)
	date := c.Param("date")
	if _, err := time.Parse(dayLayout, date); !ok || err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	db := DBFromContext(c)
	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()
	if err := deleteDay(tx, plantID, date); err != nil {
		logger.Log.WithError(err).WithFields(logrus.Fields{"plant_id": plantID, "date": date}).Error("Failed to delete day")
		apiInternalError(c, "api_failed_to_delete_activity")
		return
	}
	if err := tx.Commit(); err != nil {
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}
	logger.Log.WithFields(logrus.Fields{"plant_id": plantID, "date": date}).Info("Plant day deleted")
	apiOK(c, "api_day_deleted")
}

// LogPlantsDayHandler serves POST /plants/day: log the same ticks and note
// for several plants at once, added to whatever each plant has that day.
func LogPlantsDayHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "LogPlantsDayHandler")
	var in struct {
		dayInput
		PlantIDs []int  `json:"plant_ids"`
		Date     string `json:"date"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	if len(in.PlantIDs) == 0 {
		apiBadRequest(c, "api_no_plants_selected")
		return
	}
	db := DBFromContext(c)
	acts := activityMap(db)
	ticks, values, msg := validateDayInput(in.Date, &in.dayInput, acts)
	if msg != "" {
		apiBadRequest(c, msg)
		return
	}
	if len(ticks) == 0 && in.Note == "" {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	for _, id := range in.PlantIDs {
		if !plantExists(db, id) {
			apiNotFound(c, "api_plant_not_found")
			return
		}
	}
	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()
	seen := map[int]bool{}
	for _, id := range in.PlantIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := appendDay(tx, id, in.Date, ticks, values, in.Note, acts); err != nil {
			fieldLogger.WithError(err).WithField("plant_id", id).Error("Failed to log day")
			apiInternalError(c, "api_failed_to_save_activity")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}
	fieldLogger.WithFields(logrus.Fields{"plants": in.PlantIDs, "date": in.Date, "types": ticks}).Info("Day logged for plants")
	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_day_saved")})
}

// ---------------------------------------------------------------------------
// All plants
// ---------------------------------------------------------------------------

// NotebookPlant is what the All plants view needs to label rows and draw
// the plant chips: stage history (for "Flower · Wk 2 · Day 72" on any day),
// tent, and latest photo.
type NotebookPlant struct {
	ID      int             `json:"id"`
	Name    string          `json:"name"`
	Strain  string          `json:"strain"`
	Zone    string          `json:"zone"`
	ZoneID  int             `json:"zone_id"`
	Start   string          `json:"start"` // YYYY-MM-DD, "" if unknown
	Status  string          `json:"status"`
	Active  bool            `json:"active"` // still growing (current stage is an active one)
	Photo   string          `json:"photo"`
	History []NotebookStage `json:"history"` // oldest first
}

// NotebookStage is one stage change.
type NotebookStage struct {
	Status string `json:"status"`
	Date   string `json:"date"` // YYYY-MM-DD
}

// LoadNotebookPlants returns every plant with its stage history and latest
// photo, sorted by name.
func LoadNotebookPlants(db *sql.DB) ([]NotebookPlant, error) {
	byID := map[int]*NotebookPlant{}
	var out []*NotebookPlant
	rows, err := db.Query(`
		SELECT p.id, p.name, COALESCE(s.name, ''), COALESCE(z.name, ''), COALESCE(p.zone_id, 0), p.start_dt
		FROM plant p
		LEFT OUTER JOIN strain s ON s.id = p.strain_id
		LEFT OUTER JOIN zones z ON z.id = p.zone_id`)
	if err != nil {
		return nil, fmt.Errorf("query plants: %w", err)
	}
	for rows.Next() {
		p := &NotebookPlant{History: []NotebookStage{}}
		var start sql.NullTime
		if err := rows.Scan(&p.ID, &p.Name, &p.Strain, &p.Zone, &p.ZoneID, &start); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan plant: %w", err)
		}
		if start.Valid {
			p.Start = utils.AsLocal(start.Time).Format(dayLayout)
		}
		byID[p.ID] = p
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	type stageRow struct {
		plantID int
		status  string
		active  bool
		when    time.Time
		id      int
	}
	var stages []stageRow
	rows, err = db.Query(`
		SELECT psl.id, psl.plant_id, COALESCE(ps.status, ''), COALESCE(ps.active, 0), psl.date
		FROM plant_status_log psl
		LEFT OUTER JOIN plant_status ps ON ps.id = psl.status_id`)
	if err != nil {
		return nil, fmt.Errorf("query stages: %w", err)
	}
	for rows.Next() {
		var r stageRow
		var active int
		if err := rows.Scan(&r.id, &r.plantID, &r.status, &active, &r.when); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan stage: %w", err)
		}
		r.active = active != 0
		r.when = utils.AsLocal(r.when)
		stages = append(stages, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(stages, func(i, j int) bool {
		if !stages[i].when.Equal(stages[j].when) {
			return stages[i].when.Before(stages[j].when)
		}
		return stages[i].id < stages[j].id
	})
	for _, r := range stages {
		if p := byID[r.plantID]; p != nil {
			p.History = append(p.History, NotebookStage{Status: r.status, Date: r.when.Format(dayLayout)})
			p.Status, p.Active = r.status, r.active
		}
	}

	rows, err = db.Query(`SELECT plant_id, image_path, image_date FROM plant_images`)
	if err != nil {
		return nil, fmt.Errorf("query photos: %w", err)
	}
	latest := map[int]time.Time{}
	for rows.Next() {
		var plantID int
		var path string
		var when time.Time
		if err := rows.Scan(&plantID, &path, &when); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan photo: %w", err)
		}
		if p := byID[plantID]; p != nil && (p.Photo == "" || !when.Before(latest[plantID])) {
			p.Photo = "/" + strings.ReplaceAll(path, "\\", "/")
			latest[plantID] = when
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	res := make([]NotebookPlant, len(out))
	for i, p := range out {
		res[i] = *p
	}
	return res, nil
}

// parseIDList reads "1,2,3" into ids (ignoring anything that isn't a
// positive number).
func parseIDList(raw string) []int {
	var ids []int
	for _, tok := range strings.Split(raw, ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(tok)); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// AllPlantsNotebookHandler serves GET /notebook?plant_ids=1,2: every plant
// (for labels and filters) plus the notebook days of the requested plants —
// by default the ones still growing.
func AllPlantsNotebookHandler(c *gin.Context) {
	db := DBFromContext(c)
	plants, err := LoadNotebookPlants(db)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load notebook plants")
		apiInternalError(c, "api_database_error")
		return
	}
	ids := parseIDList(c.Query("plant_ids"))
	if c.Query("plant_ids") == "" {
		for _, p := range plants {
			if p.Active {
				ids = append(ids, p.ID)
			}
		}
	}
	days, err := LoadPlantNotebook(db, ids)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load all-plants notebook")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"plants": plants, "days": days})
}

// ActivitiesRedirect serves GET /activities, the old Activity Log page: the
// notebook for all plants now lives on the Plant page's All plants view, so
// it opens there on the most recently logged growing plant (or the Plants
// list when there are no plants).
func ActivitiesRedirect(c *gin.Context) {
	db := DBFromContext(c)
	plants, err := LoadNotebookPlants(db)
	if err != nil || len(plants) == 0 {
		c.Redirect(http.StatusFound, "/plants")
		return
	}
	active := map[int]bool{}
	target := plants[0].ID
	for i := len(plants) - 1; i >= 0; i-- {
		if plants[i].Active {
			active[plants[i].ID] = true
			target = plants[i].ID
		}
	}
	// Highest row id = most recently logged (dates may be in mixed formats).
	rows, err := db.Query(`SELECT plant_id FROM plant_activity GROUP BY plant_id ORDER BY MAX(id) DESC`)
	if err == nil {
		for rows.Next() {
			var id int
			if rows.Scan(&id) == nil && active[id] {
				target = id
				break
			}
		}
		rows.Close()
	}
	c.Redirect(http.StatusFound, fmt.Sprintf("/plant/%d#all", target))
}
