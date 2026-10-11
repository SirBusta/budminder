package types

// NotebookDay is one row of a plant's notebook: everything logged for one
// plant on one day. Each ticked activity type is still its own
// plant_activity row (Rows); the day's single note comes from
// plant_day_entry, or — for days logged before that table existed — from
// the rows' own notes joined together.
type NotebookDay struct {
	PlantID      int                   `json:"plant_id"`
	Date         string                `json:"date"` // YYYY-MM-DD
	Types        []NotebookType        `json:"types"`
	Note         string                `json:"note"`
	HasEntry     bool                  `json:"has_entry"`
	Measurements []NotebookMeasurement `json:"measurements"`
	Photos       []NotebookPhoto       `json:"photos"`
	Rows         []NotebookRow         `json:"rows"`
}

// NotebookType is one activity type logged on the day (Count > 1 when it
// was logged more than once, e.g. watered twice).
type NotebookType struct {
	ActivityID int    `json:"activity_id"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
	Count      int    `json:"count"`
}

// NotebookRow is one underlying plant_activity row.
type NotebookRow struct {
	ID         int    `json:"id"`
	ActivityID int    `json:"activity_id"`
	Time       string `json:"time"` // HH:MM:SS, recorded but not shown
}

// NotebookMeasurement is a value recorded on the day. ActivityID is the
// activity type it belongs to (0 if none); Standalone marks measurements
// added on their own (the old "Add measurement" button) rather than with an
// activity.
type NotebookMeasurement struct {
	ID         int     `json:"id"`
	MetricID   int     `json:"metric_id"`
	Name       string  `json:"name"`
	Unit       string  `json:"unit"`
	Value      float64 `json:"value"`
	ActivityID int     `json:"activity_id"`
	Standalone bool    `json:"standalone"`
}

// NotebookPhoto is a plant photo dated on the day.
type NotebookPhoto struct {
	ID          int    `json:"id"`
	Path        string `json:"path"`
	Description string `json:"description"`
}
