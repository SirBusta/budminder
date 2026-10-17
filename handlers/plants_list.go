package handlers

import (
	"database/sql"
	"fmt"
	"isley/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// PlantListRow is one plant on the Plants page (mockup PL3): which list it
// belongs to, its current stage, the dates the page works its numbers out
// from (all YYYY-MM-DD, "" when unknown) and its latest photo.
type PlantListRow struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Strain        string  `json:"strain"`
	StrainID      int     `json:"strain_id"`
	Breeder       string  `json:"breeder"`
	Zone          string  `json:"zone"`
	ZoneID        int     `json:"zone_id"`
	Autoflower    bool    `json:"autoflower"`
	Clone         bool    `json:"clone"`
	Group         string  `json:"group"` // living | harvested | dead
	Stage         string  `json:"stage"`
	StageSince    string  `json:"stage_since"`
	Start         string  `json:"start"`
	LastWater     string  `json:"last_water"`
	LastFeed      string  `json:"last_feed"`
	EstMin        string  `json:"est_min"` // estimated harvest window
	EstMax        string  `json:"est_max"`
	HarvestWeight float64 `json:"harvest_weight"`
	Photo         string  `json:"photo"`
	// Smoke reports: how many, and the latest one's overall score / id.
	Smoke SmokeSummary `json:"smoke"`
}

// plantGroup sorts a plant into the Plants page's lists by its current stage.
func plantGroup(status string, active bool) string {
	switch {
	case active:
		return "living"
	case status == "Dead":
		return "dead"
	default:
		return "harvested"
	}
}

// LoadPlantsList returns every plant for the Plants page.
func LoadPlantsList(db *sql.DB) ([]PlantListRow, error) {
	base, err := LoadNotebookPlants(db) // stage history, tent, latest photo
	if err != nil {
		return nil, err
	}

	type extra struct {
		breeder        string
		auto, clone    bool
		cycle, cycleMn int
		weight         float64
	}
	extras := map[int]extra{}
	rows, err := db.Query(`
		SELECT p.id, COALESCE(b.name, ''), COALESCE(s.autoflower, false), COALESCE(p.clone, false),
		       COALESCE(s.cycle_time, 0), COALESCE(s.cycle_time_min, 0), COALESCE(p.harvest_weight, 0)
		FROM plant p
		LEFT OUTER JOIN strain s ON s.id = p.strain_id
		LEFT OUTER JOIN breeder b ON b.id = s.breeder_id`)
	if err != nil {
		return nil, fmt.Errorf("query plant details: %w", err)
	}
	for rows.Next() {
		var id int
		var e extra
		if err := rows.Scan(&id, &e.breeder, &e.auto, &e.clone, &e.cycle, &e.cycleMn, &e.weight); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan plant details: %w", err)
		}
		extras[id] = e
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Last watering / feeding, compared as times in Go: stored dates mix
	// "…T08:10:00" and "… 18:30:00" text, which don't sort correctly in SQL.
	lastWater, lastFeed := map[int]time.Time{}, map[int]time.Time{}
	rows, err = db.Query(`
		SELECT pa.plant_id, COALESCE(a.is_watering, false), COALESCE(a.is_feeding, false), pa.date
		FROM plant_activity pa
		JOIN activity a ON a.id = pa.activity_id
		WHERE a.is_watering = true OR a.is_feeding = true`)
	if err != nil {
		return nil, fmt.Errorf("query water/feed: %w", err)
	}
	for rows.Next() {
		var id int
		var water, feed bool
		var when time.Time
		if err := rows.Scan(&id, &water, &feed, &when); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan water/feed: %w", err)
		}
		when = utils.AsLocal(when)
		if water && when.After(lastWater[id]) {
			lastWater[id] = when
		}
		if feed && when.After(lastFeed[id]) {
			lastFeed[id] = when
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	smoke := SmokeSummariesByPlant(db)
	out := make([]PlantListRow, 0, len(base))
	for _, p := range base {
		e := extras[p.ID]
		r := PlantListRow{
			ID: p.ID, Name: p.Name, Strain: p.Strain, StrainID: p.StrainID, Breeder: e.breeder,
			Zone: p.Zone, ZoneID: p.ZoneID, Autoflower: e.auto, Clone: e.clone,
			Group: plantGroup(p.Status, p.Active), Stage: p.Status, Start: p.Start,
			LastWater: dateOrEmpty(lastWater[p.ID]), LastFeed: dateOrEmpty(lastFeed[p.ID]),
			HarvestWeight: e.weight, Photo: p.Photo, Smoke: smoke[p.ID],
		}
		if n := len(p.History); n > 0 {
			r.StageSince = p.History[n-1].Date
		}
		r.EstMin, r.EstMax = estimateHarvest(p, e.auto, e.cycleMn, e.cycle)
		out = append(out, r)
	}
	return out, nil
}

// estimateHarvest is the Plant page's harvest window for a list row:
// autoflowers count from the start date, photoperiods from the first day
// of flower (no estimate before flower). A missing minimum uses the maximum.
func estimateHarvest(p NotebookPlant, auto bool, minDays, maxDays int) (string, string) {
	if maxDays <= 0 {
		return "", ""
	}
	if minDays <= 0 || minDays > maxDays {
		minDays = maxDays
	}
	from := ""
	if auto {
		from = p.Start
	} else {
		for _, h := range p.History {
			if h.Status == "Flower" {
				from = h.Date
				break
			}
		}
	}
	t, err := time.Parse(dayLayout, from)
	if err != nil {
		return "", ""
	}
	return t.AddDate(0, 0, minDays).Format(dayLayout), t.AddDate(0, 0, maxDays).Format(dayLayout)
}

// PlantsPageData is the JSON the Plants page draws from.
func PlantsPageData(c *gin.Context, loggedIn bool, lang string, translations map[string]string) gin.H {
	plants, err := LoadPlantsList(DBFromContext(c))
	if err != nil {
		plants = []PlantListRow{}
	}
	t := map[string]string{}
	for k, v := range translations {
		if strings.HasPrefix(k, "pl_") {
			t[k] = v
		}
	}
	return gin.H{
		"plants":    plants,
		"stages":    plantStages(DBFromContext(c), translations),
		"today":     time.Now().Format(dayLayout),
		"logged_in": loggedIn,
		"lang":      lang,
		"t":         t,
	}
}

// stageLabel is a plant stage with its name in the page's language.
type stageLabel struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// plantStages lists the plant stages in their order with translated names
// (the "<stage>_label" keys, falling back to the stored name).
func plantStages(db *sql.DB, translations map[string]string) []stageLabel {
	stages := []stageLabel{}
	rows, err := db.Query(`SELECT status FROM plant_status ORDER BY status_order`)
	if err != nil {
		return stages
	}
	defer rows.Close()
	for rows.Next() {
		var st stageLabel
		if rows.Scan(&st.Name) == nil {
			st.Label = st.Name
			if l := translations[strings.ToLower(st.Name)+"_label"]; l != "" {
				st.Label = l
			}
			stages = append(stages, st)
		}
	}
	return stages
}
