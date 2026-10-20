package handlers

import (
	"database/sql"
	"fmt"
	"isley/logger"
	"isley/model/types"
	"isley/utils"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// The Home ("Today") page, mockup H1: what needs attention, each tent with
// its readings, camera and plant cards, and recent activity.

const (
	homeRecentLimit = 8
	// Seed batches older than this show in "Needs attention".
	homeOldSeedYears = 2
)

// HomePlant is a growing plant's card: the Plants page row plus its latest
// photo date and the last value recorded for each measurement (so the Log
// window can prefill e.g. the last water amount).
type HomePlant struct {
	PlantListRow
	PhotoDate string          `json:"photo_date"`
	Last      map[int]float64 `json:"last"` // metric id -> last value
}

// HomeReading is a Home-visible sensor in a tent.
type HomeReading struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Unit   string   `json:"unit"`
	Value  *float64 `json:"value"`
	LastAt int64    `json:"last_at"`
	Stale  bool     `json:"stale"`
}

// HomeCamera is a visible camera stream with its latest snapshot.
type HomeCamera struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Snapshot string `json:"snapshot"` // "" when no snapshot has been taken yet
	SnapAt   int64  `json:"snap_at"`  // epoch ms
}

// HomeTent is one section of the page.
type HomeTent struct {
	ID       int           `json:"id"` // 0 = plants without a tent
	Name     string        `json:"name"`
	Readings []HomeReading `json:"readings"`
	Ports    []HomeReading `json:"ports"` // AC Infinity ports (lights, fans)
	Quiet    []HomeReading `json:"quiet"` // off the summary but stopped reporting
	Cameras  []HomeCamera  `json:"cameras"`
	Plants   []int         `json:"plants"`
}

// HomeRecent is one recent notebook entry; entries logged for several
// plants at once (same day, types, note and measurements) are merged.
type HomeRecent struct {
	Date         string                      `json:"date"`
	PlantIDs     []int                       `json:"plant_ids"`
	Types        []types.NotebookType        `json:"types"`
	Note         string                      `json:"note"`
	Measurements []types.NotebookMeasurement `json:"measurements"`
}

// HomePageData is the JSON the Home page draws from (and /home/data
// returns for its refresh). Guests get the same page without alerts.
func HomePageData(c *gin.Context, loggedIn bool, lang string, translations map[string]string) (gin.H, error) {
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)

	all, err := LoadPlantsList(db)
	if err != nil {
		return nil, err
	}
	var plants []HomePlant
	var ids []int
	for _, p := range all {
		if p.Group == "living" {
			plants = append(plants, HomePlant{PlantListRow: p, Last: map[int]float64{}})
			ids = append(ids, p.ID)
		}
	}
	if plants == nil {
		plants = []HomePlant{}
	}
	if err := addHomePlantExtras(db, plants); err != nil {
		return nil, err
	}

	tents := homeTents(c, db, store.PollingInterval(), plants)

	recent, err := homeRecent(db, ids)
	if err != nil {
		return nil, err
	}

	t := map[string]string{}
	for k, v := range translations {
		if strings.HasPrefix(k, "hm_") {
			t[k] = v
		}
	}
	data := gin.H{
		"plants":    plants,
		"tents":     tents,
		"recent":    recent,
		"types":     GetActivities(db),
		"metrics":   GetMetrics(db),
		"stages":    plantStages(db, translations),
		"units":     utils.NormalizeUnits(store.Units()),
		"poll":      store.PollingInterval(),
		"today":     time.Now().Format(dayLayout),
		"now":       time.Now().UnixMilli(),
		"logged_in": loggedIn,
		"lang":      lang,
		"t":         t,
	}
	if loggedIn {
		data["old_seed_batches"] = countOldSeedBatches(db, time.Now())
		data["integrations"] = IntegrationStatuses(db, store)
	}
	return data, nil
}

// HomeDataHandler serves HomePageData for the page's refresh (GET /home/data).
func HomeDataHandler(c *gin.Context) {
	lang := utils.GetLanguage(c)
	loggedIn := sessions.Default(c).Get("logged_in") != nil
	data, err := HomePageData(c, loggedIn, lang, utils.TranslationService.GetTranslations(lang))
	if err != nil {
		logger.Log.WithField("handler", "HomeDataHandler").WithError(err).Error("Failed to load home data")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(200, data)
}

// addHomePlantExtras fills each plant's latest photo date and last value
// per measurement (compared as times in Go, like the rest of the notebook).
func addHomePlantExtras(db *sql.DB, plants []HomePlant) error {
	if len(plants) == 0 {
		return nil
	}
	byID := map[int]*HomePlant{}
	ids := make([]int, 0, len(plants))
	for i := range plants {
		byID[plants[i].ID] = &plants[i]
		ids = append(ids, plants[i].ID)
	}
	in, args := inPlaceholders(ids)

	rows, err := db.Query(`SELECT plant_id, image_date FROM plant_images WHERE plant_id IN (`+in+`)`, args...)
	if err != nil {
		return fmt.Errorf("query photo dates: %w", err)
	}
	latestPhoto := map[int]time.Time{}
	for rows.Next() {
		var id int
		var when sql.NullTime
		if err := rows.Scan(&id, &when); err != nil {
			rows.Close()
			return fmt.Errorf("scan photo date: %w", err)
		}
		if when.Valid && when.Time.After(latestPhoto[id]) {
			latestPhoto[id] = when.Time
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for id, when := range latestPhoto {
		byID[id].PhotoDate = utils.AsLocal(when).Format(dayLayout)
	}

	rows, err = db.Query(`SELECT plant_id, metric_id, value, date FROM plant_measurements WHERE plant_id IN (`+in+`)`, args...)
	if err != nil {
		return fmt.Errorf("query measurements: %w", err)
	}
	type key struct{ plant, metric int }
	lastAt := map[key]time.Time{}
	for rows.Next() {
		var plant, metric int
		var value float64
		var when time.Time
		if err := rows.Scan(&plant, &metric, &value, &when); err != nil {
			rows.Close()
			return fmt.Errorf("scan measurement: %w", err)
		}
		k := key{plant, metric}
		if prev, ok := lastAt[k]; !ok || !when.Before(prev) {
			lastAt[k] = when
			byID[plant].Last[metric] = value
		}
	}
	rows.Close()
	return rows.Err()
}

// homeTents builds a section per tent that has growing plants, with its
// Home-visible readings, ports, cameras and any other sensor that stopped
// reporting; plants without a tent come last.
func homeTents(c *gin.Context, db *sql.DB, pollSeconds int, plants []HomePlant) []HomeTent {
	byZone := map[int]*HomeTent{}
	var order []int
	for _, p := range plants {
		t := byZone[p.ZoneID]
		if t == nil {
			t = &HomeTent{ID: p.ZoneID, Name: p.Zone, Readings: []HomeReading{}, Ports: []HomeReading{}, Quiet: []HomeReading{}, Cameras: []HomeCamera{}, Plants: []int{}}
			byZone[p.ZoneID] = t
			order = append(order, p.ZoneID)
		}
		t.Plants = append(t.Plants, p.ID)
	}

	for _, s := range ListManagedSensors(db, pollSeconds) {
		t := byZone[s.ZoneID]
		if t == nil || s.ZoneID == 0 || s.Hidden {
			continue
		}
		r := HomeReading{ID: s.ID, Name: s.Name, Type: s.Type, Unit: s.Unit, Value: s.Value, LastAt: s.LastAt, Stale: s.Stale}
		if strings.HasPrefix(s.Type, "ACIP.") {
			t.Ports = append(t.Ports, r)
		} else if s.ShowHome {
			t.Readings = append(t.Readings, r)
		} else if s.Stale && s.LastAt > 0 {
			t.Quiet = append(t.Quiet, r)
		}
	}

	streamDir := StreamDirFromContext(c)
	for _, s := range GetStreams(db) {
		t := byZone[int(s.ZoneID)]
		if t == nil || !s.Visible {
			continue
		}
		cam := HomeCamera{ID: int(s.ID), Name: s.Name, URL: s.URL}
		file := fmt.Sprintf("stream_%d_latest.jpg", s.ID)
		if fi, err := os.Stat(filepath.Join(streamDir, file)); err == nil {
			cam.Snapshot = "/uploads/streams/" + file
			cam.SnapAt = fi.ModTime().UnixMilli()
		}
		t.Cameras = append(t.Cameras, cam)
	}

	out := make([]HomeTent, 0, len(order))
	for _, id := range order {
		out = append(out, *byZone[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].ID == 0) != (out[j].ID == 0) {
			return out[j].ID == 0 // "No tent" last
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// homeRecent returns the latest notebook entries across the given plants,
// newest first, merging entries logged for several plants at once.
func homeRecent(db *sql.DB, plantIDs []int) ([]HomeRecent, error) {
	out := []HomeRecent{}
	if len(plantIDs) == 0 {
		return out, nil
	}
	days, err := LoadPlantNotebook(db, plantIDs)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(days, func(i, j int) bool { return days[i].Date > days[j].Date })
	seen := map[string]int{}
	for _, d := range days {
		if len(d.Types) == 0 && d.Note == "" && len(d.Measurements) == 0 {
			continue // photo-only days
		}
		key := recentKey(d)
		if i, ok := seen[key]; ok {
			out[i].PlantIDs = append(out[i].PlantIDs, d.PlantID)
			continue
		}
		if len(out) == homeRecentLimit {
			continue
		}
		seen[key] = len(out)
		ms := d.Measurements
		if ms == nil {
			ms = []types.NotebookMeasurement{}
		}
		out = append(out, HomeRecent{Date: d.Date, PlantIDs: []int{d.PlantID}, Types: d.Types, Note: d.Note, Measurements: ms})
	}
	return out, nil
}

func recentKey(d types.NotebookDay) string {
	var b strings.Builder
	b.WriteString(d.Date + "|" + d.Note + "|")
	for _, t := range d.Types {
		fmt.Fprintf(&b, "%d,", t.ActivityID)
	}
	b.WriteString("|")
	for _, m := range d.Measurements {
		fmt.Fprintf(&b, "%d=%g,", m.MetricID, m.Value)
	}
	return b.String()
}

// countOldSeedBatches counts strains with seeds in stock that were added
// more than homeOldSeedYears ago.
func countOldSeedBatches(db *sql.DB, now time.Time) int {
	cutoff := now.AddDate(-homeOldSeedYears, 0, 0).Format(dayLayout)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM strain WHERE seed_count > 0 AND seeds_added_on IS NOT NULL AND seeds_added_on <> '' AND seeds_added_on < $1`, cutoff).Scan(&n); err != nil {
		logger.Log.WithField("func", "countOldSeedBatches").WithError(err).Error("Error counting old seeds")
		return 0
	}
	return n
}
