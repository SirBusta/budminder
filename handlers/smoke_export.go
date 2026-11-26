package handlers

// Smoke report export (Export button on a report's page): everything the
// one-page Humidor / Botanical layouts show, as display-ready text. The page
// itself, the PNG and the PDF are drawn in the browser (smoke-export.js).
// Decisions: data/mockups/smoke/export/DECISIONS.md.

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"isley/utils"
)

// SmokeExportSeg is a piece of a line; B marks the bold (named) parts.
type SmokeExportSeg struct {
	T string `json:"t"`
	B bool   `json:"b,omitempty"`
}

// SmokeExportBox is a titled list of label/value rows.
type SmokeExportBox struct {
	Title string      `json:"title"`
	Rows  [][2]string `json:"rows"`
}

// SmokeExportScore is one score row; Value 0 = not scored.
type SmokeExportScore struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// SmokeExportTags is one group of picked words (Effects, Smell, ...).
type SmokeExportTags struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Words []string `json:"words"`
}

// SmokeExportShot is one plant photo in Botanical's "over time" strip.
type SmokeExportShot struct {
	URL   string `json:"url"`
	Day   string `json:"day"`
	Stage string `json:"stage"`
}

// SmokeExportStrip is the plant-over-time photo strip (3 to 5 photos).
type SmokeExportStrip struct {
	Title string            `json:"title"`
	Shots []SmokeExportShot `json:"shots"`
}

// SmokeExport is the export page's data (page data `srExportData`).
type SmokeExport struct {
	Name   string             `json:"name"`
	Smoked string             `json:"smoked"` // YYYY-MM-DD; the browser formats it
	Kicker string             `json:"kicker"` // breeder · lineage
	Sub    []SmokeExportSeg   `json:"sub"`
	Again  string             `json:"again"`      // text, "" when not given or hidden
	AgainK string             `json:"again_kind"` // yes, maybe, no
	Seal   []string           `json:"seal"`       // Humidor: lines beside the score
	Chips  []SmokeExportSeg   `json:"chips"`      // Botanical: B marks the Home grown chip
	Score  string             `json:"score"`      // "8.5", "" when nothing is scored
	Scores []SmokeExportScore `json:"scores"`
	Glance [][2]string        `json:"glance"` // Humidor: At a glance
	Box    *SmokeExportBox    `json:"box"`    // Humidor: Grow details (home grown only)
	BoxC   *SmokeExportBox    `json:"box_c"`  // Botanical: Grow details / Where it came from
	Notes  string             `json:"notes"`
	Tags   []SmokeExportTags  `json:"tags"` // effect, side, smell, taste, bud (empty groups left out)
	Photo  string             `json:"photo"`
	Strip  *SmokeExportStrip  `json:"strip"`
	File   string             `json:"file"` // file name without the extension
	T      map[string]string  `json:"t"`
}

// smokeExportKeys are the labels the export window and pages use.
var smokeExportKeys = []string{"sr_x_title", "sr_x_style", "sr_x_humidor", "sr_x_botanical", "sr_x_blend", "sr_x_blend_note",
	"sr_x_file_type", "sr_x_png", "sr_x_png_note", "sr_x_pdf", "sr_x_pdf_note", "sr_x_two_pages", "sr_x_pages", "sr_x_tall",
	"sr_x_tall_note", "sr_x_two", "sr_x_two_note", "sr_x_badge", "sr_x_image_n", "sr_x_page_n", "sr_x_download", "sr_x_working",
	"sr_x_failed", "sr_x_mast", "sr_x_smoked", "sr_x_glance", "sr_scores", "sr_x_tasting", "sr_x_out_of", "sr_x_overall",
	"sr_x_cont_next", "sr_x_cont_prev", "sr_x_cont_below", "sr_x_cont", "sr_how_it_felt", "sr_nose", "sr_x_foot", "cancel"}

var smokeFileUnsafe = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

// smokeBoldFill fills {name} placeholders, keeping each filled value as a
// bold segment ("from {plant}, home grown" -> from, Grape OG #1, , home grown).
func smokeBoldFill(tpl string, vals map[string]string) []SmokeExportSeg {
	var out []SmokeExportSeg
	for tpl != "" {
		i := strings.IndexByte(tpl, '{')
		j := -1
		if i >= 0 {
			j = strings.IndexByte(tpl[i:], '}')
		}
		if i < 0 || j < 0 {
			out = append(out, SmokeExportSeg{T: tpl})
			break
		}
		if i > 0 {
			out = append(out, SmokeExportSeg{T: tpl[:i]})
		}
		key := tpl[i+1 : i+j]
		if v, ok := vals[key]; ok {
			out = append(out, SmokeExportSeg{T: v, B: true})
		} else {
			out = append(out, SmokeExportSeg{T: tpl[i : i+j+1]})
		}
		tpl = tpl[i+j+1:]
	}
	return out
}

// smokeLineage is a strain's parents as "A × B" (empty when none are known).
func smokeLineage(db *sql.DB, strainID int) string {
	rows, err := db.Query(`SELECT parent_name FROM strain_lineage WHERE strain_id = $1 ORDER BY id`, strainID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil && strings.TrimSpace(n) != "" {
			names = append(names, strings.TrimSpace(n))
		}
	}
	if rows.Err() != nil {
		return ""
	}
	return strings.Join(names, " × ")
}

// smokeStrip picks up to five of the plant's photos spread over the grow,
// each with its day (1-based from the start) and the stage it was in. Fewer
// than three photos: no strip.
func smokeStrip(db *sql.DB, lcl map[string]string, plantID int, plantName, until string) *SmokeExportStrip {
	plants, err := LoadNotebookPlants(db)
	if err != nil {
		return nil
	}
	var np *NotebookPlant
	for i := range plants {
		if plants[i].ID == plantID {
			np = &plants[i]
		}
	}
	if np == nil {
		return nil
	}
	rows, err := db.Query(`SELECT image_path, image_date FROM plant_images WHERE plant_id = $1 ORDER BY image_date, image_order, id`, plantID)
	if err != nil {
		return nil
	}
	type shot struct{ path, date string }
	var all []shot
	seen := map[string]bool{}
	for rows.Next() {
		var p string
		var when time.Time
		if rows.Scan(&p, &when) != nil {
			continue
		}
		d := utils.AsLocal(when).Format(utils.LayoutDate)
		if seen[d] || (until != "" && d > until) {
			continue // one photo per day, none after the day smoked
		}
		seen[d] = true
		all = append(all, shot{p, d})
	}
	rows.Close()
	if rows.Err() != nil || len(all) < 3 {
		return nil
	}
	n := min(5, len(all))
	strip := &SmokeExportStrip{Title: strings.ReplaceAll(lcl["sr_x_over_time"], "{name}", plantName)}
	for k := range n {
		s := all[k*(len(all)-1)/(n-1)]
		sh := SmokeExportShot{URL: PhotoThumbURL(webPath(strings.ReplaceAll(s.path, "\\", "/")))}
		if np.Start != "" {
			if d := smokeDays(np.Start, s.date); d != nil {
				sh.Day = strings.ReplaceAll(lcl["sr_x_day"], "{n}", strconv.Itoa(*d+1))
			}
		}
		stage := ""
		for _, h := range np.History { // oldest first: the last stage reached by that day
			if h.Date <= s.date {
				stage = h.Status
			}
		}
		if stage != "" {
			if t := lcl[strings.ToLower(stage)+"_label"]; t != "" {
				sh.Stage = t
			} else {
				sh.Stage = stage
			}
		}
		strip.Shots = append(strip.Shots, sh)
	}
	return strip
}

// BuildSmokeExport turns a report's view into the export's data. Fields hidden
// in Smoke report settings stay out, as on the report's page.
func BuildSmokeExport(db *sql.DB, lcl map[string]string, v SmokeReportView) SmokeExport {
	r, h := v.R, v.Hidden
	days := func(n *int) string { return strings.ReplaceAll(lcl["sr_days"], "{n}", strconv.Itoa(*n)) }
	e := SmokeExport{Name: r.StrainName, Smoked: r.SmokedOn, Notes: strings.TrimSpace(r.Notes), Score: v.OverallText, T: map[string]string{}}
	for _, k := range smokeExportKeys {
		e.T[k] = lcl[k]
	}

	// Kicker: breeder · parents.
	var kick []string
	if r.BreederName != "" {
		kick = append(kick, r.BreederName)
	}
	if r.StrainID != nil {
		if l := smokeLineage(db, *r.StrainID); l != "" {
			kick = append(kick, l)
		}
	}
	e.Kicker = strings.Join(kick, " · ")

	// The line under the name.
	space := ""
	if r.GrowSpace != "" && !h["grow_space"] {
		space = r.GrowSpace
	}
	switch {
	case !v.Grown:
		if r.Dispensary != "" && !h["dispensary"] {
			e.Sub = smokeBoldFill(lcl["sr_x_sub_bought"], map[string]string{"place": r.Dispensary})
		}
	case r.PlantName != "" && space != "":
		e.Sub = smokeBoldFill(lcl["sr_x_sub_plant_space"], map[string]string{"plant": r.PlantName, "space": space})
	case r.PlantName != "":
		e.Sub = smokeBoldFill(lcl["sr_x_sub_plant"], map[string]string{"plant": r.PlantName})
	case space != "":
		e.Sub = smokeBoldFill(lcl["sr_x_sub_space"], map[string]string{"space": space})
	default:
		e.Sub = []SmokeExportSeg{{T: lcl["sr_grown"]}}
	}

	// Would grow / buy again.
	if r.Again != "" && !h["again"] {
		e.AgainK = r.Again
		switch {
		case r.Again == "yes" && v.Grown:
			e.Again = lcl["sr_again_yes"]
		case r.Again == "yes":
			e.Again = lcl["sr_buy_again_yes"]
		case r.Again == "maybe":
			e.Again = lcl["sr_again_maybe"]
		case v.Grown:
			e.Again = lcl["sr_again_no"]
		default:
			e.Again = lcl["sr_buy_again_no"]
		}
	}

	// Strain type, THC and CBD.
	var typ []string
	if st := v.Strain; st != nil {
		in, _ := st["indica"].(int)
		sa, _ := st["sativa"].(int)
		switch {
		case in > sa:
			typ = append(typ, fmt.Sprintf(lcl["sd_indica"], in))
		case sa > in:
			typ = append(typ, fmt.Sprintf(lcl["sd_sativa"], sa))
		default:
			typ = append(typ, lcl["sd_hybrid"])
		}
		if auto, _ := st["autoflower"].(bool); auto {
			typ = append(typ, lcl["sp_autoflower"])
		} else {
			typ = append(typ, lcl["sp_photoperiod"])
		}
	}
	var thc [][2]string
	if !h["thc_cbd"] {
		if r.THC != "" {
			thc = append(thc, [2]string{lcl["sr_thc"], r.THC + "%"})
		}
		if r.CBD != "" {
			thc = append(thc, [2]string{lcl["sr_cbd"], r.CBD + "%"})
		}
	}
	e.Seal = typ
	if len(e.Seal) == 0 {
		for _, t := range thc {
			e.Seal = append(e.Seal, t[0]+" "+t[1])
		}
	} else {
		e.Glance = append(e.Glance, thc...)
	}
	if v.Grown {
		e.Chips = append(e.Chips, SmokeExportSeg{T: lcl["sr_grown"], B: true})
	}
	for _, t := range typ {
		e.Chips = append(e.Chips, SmokeExportSeg{T: t})
	}
	for _, t := range thc {
		e.Chips = append(e.Chips, SmokeExportSeg{T: t[0] + " " + t[1]})
	}

	for _, s := range v.Scores {
		e.Scores = append(e.Scores, SmokeExportScore{Label: lcl[s.Key], Value: s.Value})
	}

	method := ""
	if v.Method != "" && !h["method"] {
		method = v.Method
	}
	daysLabel := lcl["sr_f_days_to_harvest"]
	if v.Clone {
		daysLabel = lcl["sr_f_clone_to_harvest"]
	}
	add := func(rows *[][2]string, hide string, label, value string) {
		if value != "" && (hide == "" || !h[hide]) {
			*rows = append(*rows, [2]string{label, value})
		}
	}
	if v.Grown {
		harvested, toHarvest, dry, cure := "", "", "", ""
		if r.HarvestedOn != "" {
			harvested = r.HarvestedOn // the browser formats the date (sent as "@date:")
		}
		if r.DaysToHarvest != nil {
			toHarvest = days(r.DaysToHarvest)
		}
		if r.DryDays != nil {
			dry = days(r.DryDays)
		}
		if r.CureDays != nil {
			cure = days(r.CureDays)
		}
		if harvested != "" {
			harvested = "@date:" + harvested
		}
		add(&e.Glance, "harvested", lcl["sr_f_harvested"], harvested)
		add(&e.Glance, "days_to_harvest", daysLabel, toHarvest)

		box := &SmokeExportBox{Title: lcl["sr_grow_details"]}
		add(&box.Rows, "lights", lcl["sr_f_lights"], r.Lights)
		add(&box.Rows, "container", lcl["sr_f_container"], r.Container)
		add(&box.Rows, "medium", lcl["sr_f_medium"], r.Medium)
		add(&box.Rows, "nutrients", lcl["sr_f_nutrients"], r.Nutrients)
		add(&box.Rows, "dry_cure", lcl["sr_f_dry"], dry)
		add(&box.Rows, "dry_cure", lcl["sr_f_cure"], cure)
		add(&box.Rows, "", lcl["sr_f_method"], method)
		if len(box.Rows) > 0 {
			e.Box = box
		}

		boxC := &SmokeExportBox{Title: lcl["sr_grow_details"]}
		add(&boxC.Rows, "", lcl["sr_f_grow_space"], space)
		add(&boxC.Rows, "harvested", lcl["sr_f_harvested"], harvested)
		add(&boxC.Rows, "lights", lcl["sr_f_lights"], r.Lights)
		add(&boxC.Rows, "container", lcl["sr_f_container"], r.Container)
		add(&boxC.Rows, "medium", lcl["sr_f_medium"], r.Medium)
		add(&boxC.Rows, "nutrients", lcl["sr_f_nutrients"], r.Nutrients)
		add(&boxC.Rows, "days_to_harvest", daysLabel, toHarvest)
		add(&boxC.Rows, "dry_cure", lcl["sr_f_dry"], dry)
		add(&boxC.Rows, "dry_cure", lcl["sr_f_cure"], cure)
		add(&boxC.Rows, "", lcl["sr_f_method"], method)
		if len(boxC.Rows) > 0 {
			e.BoxC = boxC
		}
	} else {
		price := ""
		if !h["dispensary"] {
			price = v.PriceText
			if price != "" && r.AmountG != nil {
				price += " · " + strconv.FormatFloat(*r.AmountG, 'f', -1, 64) + " g"
			}
		}
		ppg := ""
		if !h["dispensary"] {
			ppg = v.PPGText
		}
		add(&e.Glance, "", lcl["sr_f_price"], price)
		add(&e.Glance, "", lcl["sr_f_ppg"], ppg)
		add(&e.Glance, "", lcl["sr_f_method"], method)

		boxC := &SmokeExportBox{Title: lcl["sr_where_from"]}
		if !h["dispensary"] {
			add(&boxC.Rows, "", lcl["sr_f_dispensary"], r.Dispensary)
			add(&boxC.Rows, "", lcl["sr_f_price"], price)
			add(&boxC.Rows, "", lcl["sr_f_ppg"], ppg)
			add(&boxC.Rows, "", lcl["sr_f_batch"], r.Batch)
		}
		add(&boxC.Rows, "", lcl["sr_f_method"], method)
		if len(boxC.Rows) > 0 {
			e.BoxC = boxC
		}
	}

	for _, g := range []struct{ kind, label, hide string }{
		{"effect", "sr_effects", ""}, {"side", "sr_side_effects", "side"}, {"smell", "sr_smell", ""},
		{"taste", "sr_taste", ""}, {"bud", "sr_bud", "bud"}} {
		if w := v.Words[g.kind]; len(w) > 0 && (g.hide == "" || !h[g.hide]) {
			e.Tags = append(e.Tags, SmokeExportTags{Key: g.kind, Label: lcl[g.label], Words: w})
		}
	}

	if len(r.Photos) > 0 {
		e.Photo = r.Photos[0].URL
	}
	if v.Grown && r.PlantID != nil && r.PlantName != "" {
		e.Strip = smokeStrip(db, lcl, *r.PlantID, r.PlantName, r.SmokedOn)
	}

	file := strings.ReplaceAll(lcl["sr_x_file"], "{name}", r.StrainName)
	if r.SmokedOn != "" {
		file += " " + r.SmokedOn
	}
	file = strings.TrimSpace(smokeFileUnsafe.ReplaceAllString(file, " "))
	if file == "" {
		file = "smoke-report"
	}
	e.File = file
	return e
}
