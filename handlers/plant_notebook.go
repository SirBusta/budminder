package handlers

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

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
	actInfo := map[int]types.Activity{}
	metricActivity := map[int]int{}
	for _, a := range GetActivities(db) {
		actInfo[a.ID] = a
		for _, m := range a.Metrics {
			if _, taken := metricActivity[m.MetricID]; !taken {
				metricActivity[m.MetricID] = a.ID
			}
		}
	}

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
