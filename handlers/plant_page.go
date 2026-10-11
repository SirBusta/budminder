package handlers

import (
	"database/sql"
	"strings"
	"time"

	"isley/model/types"
)

// PlantPageData is everything the Plant page script needs, embedded in the
// page as one JSON attribute (separately fetched .js files aren't
// templated, so translated strings travel here too, under T).
type PlantPageData struct {
	Plant    PlantPageInfo         `json:"plant"`
	Statuses []PlantPageStatus     `json:"statuses"`
	History  []PlantPageStage      `json:"history"`
	Types    []types.Activity      `json:"types"`
	Metrics  []types.Metric        `json:"metrics"`
	Days     []types.NotebookDay   `json:"days"`
	Stream   *PlantPageStream      `json:"stream"`
	Prev     int                   `json:"prev"`
	Next     int                   `json:"next"`
	LoggedIn bool                  `json:"logged_in"`
	Today    string                `json:"today"`
	Lang     string                `json:"lang"`
	T        map[string]string     `json:"t"`
}

// PlantPageInfo is the header's facts about the plant.
type PlantPageInfo struct {
	ID            uint    `json:"id"`
	Name          string  `json:"name"`
	Strain        string  `json:"strain"`
	StrainID      int     `json:"strain_id"`
	StrainURL     string  `json:"strain_url"`
	Breeder       string  `json:"breeder"`
	Zone          string  `json:"zone"`
	ZoneID        int     `json:"zone_id"`
	Description   string  `json:"description"`
	Autoflower    bool    `json:"autoflower"`
	Clone         bool    `json:"clone"`
	ParentID      uint    `json:"parent_id"`
	ParentName    string  `json:"parent_name"`
	Start         string  `json:"start"` // YYYY-MM-DD
	Status        string  `json:"status"`
	StatusID      int     `json:"status_id"`
	HarvestWeight float64 `json:"harvest_weight"`
	EstMin        string  `json:"est_min"` // YYYY-MM-DD or ""
	EstMax        string  `json:"est_max"`
	CycleTime     int     `json:"cycle_time"`
	CycleTimeMin  int     `json:"cycle_time_min"`
	Photo         string  `json:"photo"` // latest photo, "" if none
}

// PlantPageStatus is one stage on the track, in order.
type PlantPageStatus struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
}

// PlantPageStage is one stage change in the plant's history.
type PlantPageStage struct {
	ID       uint   `json:"id"`
	StatusID int    `json:"status_id"`
	Status   string `json:"status"`
	Date     string `json:"date"` // YYYY-MM-DDTHH:MM:SS
}

// PlantPageStream is the camera in the plant's zone, if any.
type PlantPageStream struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

func dateOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dayLayout)
}

// BuildPlantPageData assembles the Plant page's data for a loaded plant.
// translations is the request language's map (for status labels and the
// page's "pp_" strings).
func BuildPlantPageData(db *sql.DB, plant types.Plant, loggedIn bool, lang string, translations map[string]string) (PlantPageData, error) {
	d := PlantPageData{
		Plant: PlantPageInfo{
			ID: plant.ID, Name: plant.Name, Strain: plant.StrainName, StrainID: plant.StrainID, StrainURL: plant.StrainUrl,
			Breeder: plant.BreederName, Zone: plant.ZoneName, ZoneID: plant.ZoneID, Description: plant.Description,
			Autoflower: plant.Autoflower, Clone: plant.IsClone, ParentID: plant.ParentID, ParentName: plant.ParentName,
			Start: dateOrEmpty(plant.StartDT), Status: plant.Status, StatusID: plant.StatusID, HarvestWeight: plant.HarvestWeight,
			EstMin: dateOrEmpty(plant.EstHarvestMin), EstMax: dateOrEmpty(plant.EstHarvestDate),
			CycleTime: plant.CycleTime, CycleTimeMin: plant.CycleTimeMin,
		},
		History:  []PlantPageStage{},
		Types:    GetActivities(db),
		Metrics:  GetMetrics(db),
		LoggedIn: loggedIn,
		Today:    time.Now().Format(dayLayout),
		Lang:     lang,
		T:        map[string]string{},
	}
	if plant.LatestImage.ID != 0 {
		d.Plant.Photo = plant.LatestImage.ImagePath
	}

	rows, err := db.Query(`SELECT id, status, active FROM plant_status ORDER BY status_order`)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var s PlantPageStatus
		var active int
		if err := rows.Scan(&s.ID, &s.Status, &active); err != nil {
			rows.Close()
			return d, err
		}
		s.Active = active != 0
		s.Label = s.Status
		if l := translations[strings.ToLower(s.Status)+"_label"]; l != "" {
			s.Label = l
		}
		d.Statuses = append(d.Statuses, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return d, err
	}

	// StatusHistory comes newest first; the page wants oldest first.
	for i := len(plant.StatusHistory) - 1; i >= 0; i-- {
		s := plant.StatusHistory[i]
		d.History = append(d.History, PlantPageStage{ID: s.ID, StatusID: s.StatusID, Status: s.Status, Date: s.Date.Format("2006-01-02T15:04:05")})
	}

	if plant.ZoneID != 0 {
		var s PlantPageStream
		err := db.QueryRow(`SELECT id, name, url FROM streams WHERE zone_id = $1 AND visible = $2 ORDER BY id LIMIT 1`, plant.ZoneID, true).Scan(&s.ID, &s.Name, &s.URL)
		if err == nil {
			d.Stream = &s
		} else if err != sql.ErrNoRows {
			return d, err
		}
	}

	if d.Days, err = LoadPlantNotebook(db, []int{int(plant.ID)}); err != nil {
		return d, err
	}
	for k, v := range translations {
		if strings.HasPrefix(k, "pp_") {
			d.T[k] = v
		}
	}
	return d, nil
}
