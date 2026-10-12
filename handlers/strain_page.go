package handlers

// The single-strain page (mockup SD1+): your own history with a strain —
// every plant grown from it, yields, how long they actually flowered, and
// their photos — next to what's known about the strain itself.

import (
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"isley/model/types"
)

// StrainGrow is one plant of the strain.
type StrainGrow struct {
	ID         int
	Name       string
	Group      string // living | harvested | dead
	Stage      string
	StageLabel string
	Zone       string
	Start      string // YYYY-MM-DD, "" if unknown
	End        string // harvested: first drying/curing/done day; dead: the day it died
	Day        int    // living: day N since start
	StageWeek  int    // living: week N of the current stage
	EstMin     string // living: estimated harvest window
	EstMax     string
	Progress   int     // living: percent from start to the estimated harvest, -1 when unknown
	Grams      float64 // harvested weight, 0 if not recorded
	FlowerDays int     // harvested: days of flower (autoflowers: start to harvest), 0 if unknown
	Photo      string
}

// StrainPhoto is one photo of a plant of the strain.
type StrainPhoto struct {
	Path    string
	Date    string
	PlantID int
	Plant   string
}

// StrainHistory is everything the page shows about your grows of a strain.
type StrainHistory struct {
	Grows          []StrainGrow
	Growing        int
	Harvested      int
	Dead           int
	AvgGrams       int     // average recorded yield, 0 if none
	AvgFlowerDays  int     // average actual flowering, 0 if unknown
	AvgFlowerWeeks float64 // the same in weeks, one decimal
	Photos         []StrainPhoto
	PhotoCount     int
}

const strainPagePhotos = 7

// harvestStages are the stages that mean the plant was cut down.
var harvestStages = map[string]bool{"Drying": true, "Curing": true, "Success": true}

// LoadStrainHistory returns the plants grown from a strain (growing first,
// then the most recent harvests), their averages and their newest photos.
// today is YYYY-MM-DD in the user's timezone.
func LoadStrainHistory(db *sql.DB, strainID int, today string, translations map[string]string) (StrainHistory, error) {
	h := StrainHistory{Grows: []StrainGrow{}, Photos: []StrainPhoto{}}
	rows, err := LoadPlantsList(db)
	if err != nil {
		return h, err
	}
	base, err := LoadNotebookPlants(db)
	if err != nil {
		return h, err
	}
	history := map[int][]NotebookStage{}
	for _, p := range base {
		history[p.ID] = p.History
	}
	labels := map[string]string{}
	for _, st := range plantStages(db, translations) {
		labels[st.Name] = st.Label
	}

	var gramSum float64
	var gramN, flowerSum, flowerN int
	for _, r := range rows {
		if r.StrainID != strainID {
			continue
		}
		g := StrainGrow{ID: r.ID, Name: r.Name, Group: r.Group, Stage: r.Stage, StageLabel: labels[r.Stage], Zone: r.Zone,
			Start: r.Start, EstMin: r.EstMin, EstMax: r.EstMax, Grams: r.HarvestWeight, Photo: r.Photo, Progress: -1}
		if g.StageLabel == "" {
			g.StageLabel = r.Stage
		}
		switch r.Group {
		case "living":
			h.Growing++
			if r.Start != "" {
				g.Day = daysBetween(r.Start, today) + 1
			}
			if r.StageSince != "" {
				g.StageWeek = int(math.Ceil(float64(daysBetween(r.StageSince, today)+1) / 7))
			}
			if r.Start != "" && r.EstMax != "" {
				if total := daysBetween(r.Start, r.EstMax); total > 0 {
					g.Progress = clampPercent(daysBetween(r.Start, today) * 100 / total)
				}
			}
		case "harvested":
			h.Harvested++
			flowerFrom := ""
			if r.Autoflower {
				flowerFrom = r.Start
			}
			for _, st := range history[r.ID] {
				if st.Status == "Flower" && flowerFrom == "" {
					flowerFrom = st.Date
				}
				if harvestStages[st.Status] && g.End == "" {
					g.End = st.Date
				}
			}
			if flowerFrom != "" && g.End != "" {
				if d := daysBetween(flowerFrom, g.End); d > 0 {
					g.FlowerDays = d
					flowerSum += d
					flowerN++
				}
			}
			if r.HarvestWeight > 0 {
				gramSum += r.HarvestWeight
				gramN++
			}
		case "dead":
			h.Dead++
			if n := len(history[r.ID]); n > 0 {
				g.End = history[r.ID][n-1].Date
			}
		}
		h.Grows = append(h.Grows, g)
	}
	if gramN > 0 {
		h.AvgGrams = int(math.Round(gramSum / float64(gramN)))
	}
	if flowerN > 0 {
		h.AvgFlowerDays = int(math.Round(float64(flowerSum) / float64(flowerN)))
		h.AvgFlowerWeeks = math.Round(float64(flowerSum)/float64(flowerN)/7*10) / 10
	}
	// Growing first (by name), then harvested and dead, newest first.
	order := map[string]int{"living": 0, "harvested": 1, "dead": 2}
	sort.SliceStable(h.Grows, func(i, j int) bool {
		a, b := h.Grows[i], h.Grows[j]
		if order[a.Group] != order[b.Group] {
			return order[a.Group] < order[b.Group]
		}
		if a.Group == "living" {
			return a.Name < b.Name
		}
		return a.End > b.End
	})

	photos, err := db.Query(`
		SELECT i.image_path, COALESCE(i.image_date, ''), p.id, p.name
		FROM plant_images i JOIN plant p ON p.id = i.plant_id
		WHERE p.strain_id = $1`, strainID)
	if err != nil {
		return h, fmt.Errorf("query strain photos: %w", err)
	}
	defer photos.Close()
	for photos.Next() {
		var ph StrainPhoto
		if err := photos.Scan(&ph.Path, &ph.Date, &ph.PlantID, &ph.Plant); err != nil {
			return h, fmt.Errorf("scan strain photo: %w", err)
		}
		ph.Path = "/" + strings.ReplaceAll(ph.Path, "\\", "/")
		if len(ph.Date) > 10 {
			ph.Date = ph.Date[:10] // stored dates mix "…T…" and "… …" forms
		}
		h.Photos = append(h.Photos, ph)
	}
	if err := photos.Err(); err != nil {
		return h, err
	}
	sort.SliceStable(h.Photos, func(i, j int) bool { return h.Photos[i].Date > h.Photos[j].Date })
	h.PhotoCount = len(h.Photos)
	if len(h.Photos) > strainPagePhotos {
		h.Photos = h.Photos[:strainPagePhotos]
	}
	return h, nil
}

// daysBetween counts whole days from a to b (YYYY-MM-DD); 0 if either is unreadable.
func daysBetween(a, b string) int {
	ta, err1 := time.Parse(dayLayout, a)
	tb, err2 := time.Parse(dayLayout, b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(math.Round(tb.Sub(ta).Hours() / 24))
}

// StrainLinkHost is the site a strain's saved link goes to ("seedjunkygenetics.com"),
// for showing on the page; "" when the link is empty or unreadable.
func StrainLinkHost(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	if !strings.Contains(link, "://") {
		link = "https://" + link
	}
	u, err := url.Parse(link)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// SafeStrainLink returns the strain's saved link if it's a web address
// (http/https), adding https:// when it has no scheme; "" otherwise, so a
// stored "javascript:" link is never rendered.
func SafeStrainLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	if !strings.Contains(link, "://") {
		link = "https://" + link
	}
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return ""
	}
	return u.String()
}

// BreederStrainCount is how many of your strains come from a breeder.
func BreederStrainCount(db *sql.DB, breederID int) int {
	if breederID <= 0 {
		return 0
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM strain WHERE breeder_id = $1`, breederID).Scan(&n); err != nil {
		return 0
	}
	return n
}

// StrainGrowURL is the Add plant link for starting a grow of a strain, named
// "<strain> #N" and using one seed when there are seeds in stock (the
// Strains page's "Start a grow").
func StrainGrowURL(s StrainGrowTarget, plantCount int) string {
	q := url.Values{}
	q.Set("strain_id", fmt.Sprint(s.ID))
	q.Set("name", fmt.Sprintf("%s #%d", s.Name, plantCount+1))
	if s.SeedCount > 0 {
		q.Set("use_seed", "1")
	}
	return "/plant/new?" + q.Encode()
}

// StrainGrowTarget is what StrainGrowURL needs from a strain.
type StrainGrowTarget struct {
	ID        int
	Name      string
	SeedCount int
}

// PotencyRow is one cannabinoid range drawn as a bar (percent of the scale).
type PotencyRow struct {
	Label string
	Text  string
	Left  float64
	Width float64
}

// StrainPotency lists THC/CBD (ranges) and CBN/CBG (maximums) that are
// known, on a shared 0–35% scale (wider if a value is higher).
func StrainPotency(s types.Strain) []PotencyRow {
	type c struct {
		label    string
		min, max *float64
	}
	all := []c{{"THC", s.ThcMin, s.ThcMax}, {"CBD", s.CbdMin, s.CbdMax}, {"CBN", nil, s.CbnMax}, {"CBG", nil, s.CbgMax}}
	scale := 35.0
	for _, x := range all {
		if x.max != nil && *x.max > scale {
			scale = *x.max
		}
	}
	rows := []PotencyRow{}
	for _, x := range all {
		if x.max == nil || *x.max <= 0 {
			continue
		}
		lo := 0.0
		text := trimFloat(*x.max) + "%"
		if x.min != nil && *x.min > 0 && *x.min < *x.max {
			lo = *x.min
			text = trimFloat(*x.min) + "–" + text
		} else if x.min != nil {
			text = "0–" + text
		}
		width := (*x.max - lo) / scale * 100
		rows = append(rows, PotencyRow{Label: x.label, Text: text, Left: lo / scale * 100, Width: math.Max(width, 2)})
	}
	return rows
}

func trimFloat(f float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}

// AttrRow is an effect (Pct) or terpene (Dots: 1 low, 2 medium, 3 high).
type AttrRow struct {
	Name  string
	Pct   int
	Level string
	Dots  int
}

// StrainEffectRows are the strain's effects with their 0–100 intensity.
func StrainEffectRows(s types.Strain) []AttrRow {
	out := []AttrRow{}
	for _, a := range s.Effects {
		r := AttrRow{Name: a.Name}
		if a.Intensity != nil {
			r.Pct = clampPercent(int(math.Round(*a.Intensity)))
		}
		out = append(out, r)
	}
	return out
}

// StrainTerpeneRows are the strain's terpenes with their low/medium/high level.
func StrainTerpeneRows(s types.Strain) []AttrRow {
	out := []AttrRow{}
	for _, a := range s.Terpenes {
		r := AttrRow{Name: a.Name}
		if a.Level != nil {
			r.Level = strings.ToLower(*a.Level)
			r.Dots = map[string]int{"low": 1, "medium": 2, "high": 3}[r.Level]
		}
		out = append(out, r)
	}
	return out
}
