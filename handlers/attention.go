package handlers

import (
	"database/sql"
	"encoding/json"
	"isley/logger"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// The Today page's "Needs attention" settings, repeating tasks and hidden
// lines (mockup data/mockups/home/attention/v3.html). Everything is kept in
// four JSON settings, so it travels with backups and the Postgres move:
//
//	attention.settings  which reminder types are on, and their days
//	attention.tasks     repeating tasks: name, icon, every N days, and per
//	                    tent (zone id) the day it was last done or added
//	attention.hidden    lines hidden from the list (line key -> how)
//	attention.burped    curing plants: plant id -> day the jars were last burped
//
// The page builds the lines itself (home-page.js); these routes only store
// what the grower chose. Tasks never write to plant journals.

const (
	attentionSettingsName = "attention.settings"
	attentionTasksName    = "attention.tasks"
	attentionHiddenName   = "attention.hidden"
	attentionBurpedName   = "attention.burped"

	attentionMaxTasks    = 50
	attentionMaxHidden   = 500
	attentionMaxNameLen  = 60
	attentionMaxDays     = 90  // Due/Overdue, photo and burp days
	attentionMaxEvery    = 365 // a task's "every N days"
	attentionHideKeepFor = 180 // days an "until …" hide is kept
)

// attentionMu serializes the read-modify-write of the attention settings
// (two quick clicks on the page must not lose one of them).
var attentionMu sync.Mutex

// AttentionCare is Watering or Feeding: on/off and the days without one
// before a plant is due (amber) and overdue (red).
type AttentionCare struct {
	On   bool `json:"on"`
	Due  int  `json:"due"`
	Over int  `json:"over"`
}

type AttentionPhoto struct {
	On   bool `json:"on"`
	Days int  `json:"days"`
}

type AttentionSwitch struct {
	On bool `json:"on"`
}

// AttentionBurp is Burp curing jars: every day for the first Daily days of
// curing, then every Then days.
type AttentionBurp struct {
	On    bool `json:"on"`
	Daily int  `json:"daily"`
	Then  int  `json:"then"`
}

// AttentionSettings is what the Needs attention settings window saves.
type AttentionSettings struct {
	Water   AttentionCare   `json:"water"`
	Feed    AttentionCare   `json:"feed"`
	Photo   AttentionPhoto  `json:"photo"`
	Seeds   AttentionSwitch `json:"seeds"`
	Sensors AttentionSwitch `json:"sensors"`
	Burp    AttentionBurp   `json:"burp"`
}

// DefaultAttentionSettings are the values before the window was ever saved
// (the Today page's and Plants page's days before 2.5).
func DefaultAttentionSettings() AttentionSettings {
	return AttentionSettings{
		Water:   AttentionCare{On: true, Due: 3, Over: 6},
		Feed:    AttentionCare{On: true, Due: 10, Over: 18},
		Photo:   AttentionPhoto{On: true, Days: 14},
		Seeds:   AttentionSwitch{On: true},
		Sensors: AttentionSwitch{On: true},
		Burp:    AttentionBurp{On: false, Daily: 14, Then: 7},
	}
}

// AttentionTask is one repeating task. Zones maps each tent it covers to
// the day it was last done (or added to the task), YYYY-MM-DD.
type AttentionTask struct {
	ID    int            `json:"id"`
	Name  string         `json:"name"`
	Icon  string         `json:"icon"`
	Every int            `json:"every"`
	Zones map[int]string `json:"zones"`
}

// attentionHide is one hidden line. "tomorrow" hides it through the day
// before Until; "until" hides it while the line's Basis (what the page
// derives the line from, e.g. the plant's last watering day) is unchanged.
type attentionHide struct {
	How   string `json:"how"`
	Until string `json:"until,omitempty"`
	Basis string `json:"basis,omitempty"`
	At    string `json:"at"`
}

// AttentionTaskPresets are the suggestions in the Add task form: the
// translation key of the name, its icon and the days it suggests.
var AttentionTaskPresets = []struct {
	Key  string `json:"key"`
	Icon string `json:"icon"`
	Days int    `json:"days"`
}{
	{"hm_na_p_refill", "fa-fill-drip", 4},
	{"hm_na_p_change", "fa-arrows-rotate", 7},
	{"hm_na_p_ph", "fa-vial", 2},
	{"hm_na_p_humidifier", "fa-cloud", 2},
	{"hm_na_p_dehumidifier", "fa-bucket", 2},
	{"hm_na_p_clean_hum", "fa-soap", 14},
	{"hm_na_p_pests", "fa-bug", 7},
	{"hm_na_p_cuttings", "fa-scissors", 14},
	{"hm_na_p_packs", "fa-jar", 30},
	{"hm_na_p_filter", "fa-filter", 90},
	{"hm_na_p_calibrate", "fa-vial-circle-check", 30},
}

const attentionDefaultIcon = "fa-repeat"

// attentionIcon is icon when it's one of the presets' icons, else the
// plain repeat icon (the page puts it in a class attribute).
func attentionIcon(icon string) string {
	for _, p := range AttentionTaskPresets {
		if p.Icon == icon {
			return icon
		}
	}
	return attentionDefaultIcon
}

// attentionToday is today's date in the Settings time zone.
func attentionToday(c *gin.Context) string {
	return time.Now().In(requestLocation(c)).Format(dayLayout)
}

func addDays(day string, n int) string {
	d, err := time.Parse(dayLayout, day)
	if err != nil {
		return day
	}
	return d.AddDate(0, 0, n).Format(dayLayout)
}

// ---------------------------------------------------------------------------
// Loading and saving
// ---------------------------------------------------------------------------

func loadAttentionJSON(db *sql.DB, name string, into any) {
	raw, err := GetSetting(db, name)
	if err != nil || raw == "" {
		return
	}
	if err := json.Unmarshal([]byte(raw), into); err != nil {
		logger.Log.WithField("setting", name).WithError(err).Warn("Ignoring unreadable attention setting")
	}
}

func saveAttentionJSON(db *sql.DB, name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return UpdateSetting(db, nil, name, string(raw))
}

// LoadAttentionSettings returns the saved settings, with any missing or
// out-of-range value at its default.
func LoadAttentionSettings(db *sql.DB) AttentionSettings {
	s := DefaultAttentionSettings()
	loadAttentionJSON(db, attentionSettingsName, &s)
	d := DefaultAttentionSettings()
	okDays := func(n int) bool { return n >= 1 && n <= attentionMaxDays }
	for _, pair := range []struct{ got, def *AttentionCare }{{&s.Water, &d.Water}, {&s.Feed, &d.Feed}} {
		if !okDays(pair.got.Due) || !okDays(pair.got.Over) || pair.got.Over <= pair.got.Due {
			pair.got.Due, pair.got.Over = pair.def.Due, pair.def.Over
		}
	}
	if !okDays(s.Photo.Days) {
		s.Photo.Days = d.Photo.Days
	}
	if !okDays(s.Burp.Daily) {
		s.Burp.Daily = d.Burp.Daily
	}
	if !okDays(s.Burp.Then) {
		s.Burp.Then = d.Burp.Then
	}
	return s
}

func loadAttentionTasks(db *sql.DB) []AttentionTask {
	tasks := []AttentionTask{}
	loadAttentionJSON(db, attentionTasksName, &tasks)
	for i := range tasks {
		if tasks[i].Zones == nil {
			tasks[i].Zones = map[int]string{}
		}
	}
	return tasks
}

func loadAttentionHidden(db *sql.DB) map[string]attentionHide {
	h := map[string]attentionHide{}
	loadAttentionJSON(db, attentionHiddenName, &h)
	return h
}

func loadAttentionBurped(db *sql.DB) map[int]string {
	b := map[int]string{}
	loadAttentionJSON(db, attentionBurpedName, &b)
	return b
}

// pruneAttentionHidden drops hides that have run out: "tomorrow" ones once
// their day has come, "until" ones after attentionHideKeepFor days.
func pruneAttentionHidden(h map[string]attentionHide, today string) {
	oldest := addDays(today, -attentionHideKeepFor)
	for k, v := range h {
		if (v.How == "tomorrow" && v.Until <= today) || (v.How != "tomorrow" && v.At < oldest) {
			delete(h, k)
		}
	}
}

// zoneNames lists every tent, sorted by name.
type attentionZone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func attentionZones(db *sql.DB) []attentionZone {
	out := []attentionZone{}
	for _, z := range GetZones(db) {
		out = append(out, attentionZone{ID: int(z.ID), Name: z.Name})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// attentionCuring is a plant in the Curing stage (for Burp curing jars).
type attentionCuring struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	ZoneID int    `json:"zone_id"`
	Since  string `json:"since"` // the day it entered Curing
}

// attentionOldSeed is a strain whose seeds are over 2 years old.
type attentionOldSeed struct {
	ID    int    `json:"id"`
	Added string `json:"added"`
}

func oldSeedStrains(db *sql.DB, now time.Time) []attentionOldSeed {
	cutoff := now.AddDate(-homeOldSeedYears, 0, 0).Format(dayLayout)
	out := []attentionOldSeed{}
	rows, err := db.Query(`SELECT id, seeds_added_on FROM strain WHERE seed_count > 0 AND seeds_added_on IS NOT NULL AND seeds_added_on <> '' AND seeds_added_on < $1 ORDER BY id`, cutoff)
	if err != nil {
		logger.Log.WithField("func", "oldSeedStrains").WithError(err).Error("Error listing old seeds")
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s attentionOldSeed
		if rows.Scan(&s.ID, &s.Added) == nil {
			if len(s.Added) > 10 {
				s.Added = s.Added[:10]
			}
			out = append(out, s)
		}
	}
	return out
}

// attentionPageData is the Needs attention part of the Today page's data
// (signed-in only).
func attentionPageData(c *gin.Context, db *sql.DB, all []PlantListRow, today string) gin.H {
	zones := attentionZones(db)
	known := map[int]bool{}
	for _, z := range zones {
		known[z.ID] = true
	}
	tasks := loadAttentionTasks(db)
	for i := range tasks {
		for zid := range tasks[i].Zones {
			if !known[zid] {
				delete(tasks[i].Zones, zid) // a tent deleted since
			}
		}
	}
	hidden := loadAttentionHidden(db)
	pruneAttentionHidden(hidden, today)
	curing := []attentionCuring{}
	burped := map[int]string{}
	saved := loadAttentionBurped(db)
	for _, p := range all {
		if p.Stage == "Curing" {
			curing = append(curing, attentionCuring{ID: p.ID, Name: p.Name, ZoneID: p.ZoneID, Since: p.StageSince})
			if d, ok := saved[p.ID]; ok {
				burped[p.ID] = d
			}
		}
	}
	return gin.H{
		"settings": LoadAttentionSettings(db),
		"tasks":    tasks,
		"zones":    zones,
		"hidden":   hidden,
		"burped":   burped,
		"curing":   curing,
		"presets":  AttentionTaskPresets,
	}
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

type attentionTaskIn struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Every int    `json:"every"`
	Zones []int  `json:"zones"`
}

// SaveAttentionHandler saves the settings window: reminder types, their
// days and the repeating tasks (POST /home/attention/settings). A task keeps
// the day each of its tents was last done; a tent added to it counts from
// today.
func SaveAttentionHandler(c *gin.Context) {
	var req struct {
		Settings *AttentionSettings `json:"settings"`
		Tasks    []attentionTaskIn  `json:"tasks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Settings == nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	s := *req.Settings
	okDays := func(n int) bool { return n >= 1 && n <= attentionMaxDays }
	for _, care := range []AttentionCare{s.Water, s.Feed} {
		if !okDays(care.Due) || !okDays(care.Over) {
			apiBadRequest(c, "hm_na_err_days")
			return
		}
		if care.Over <= care.Due {
			apiBadRequest(c, "hm_na_err_over")
			return
		}
	}
	if !okDays(s.Photo.Days) || !okDays(s.Burp.Daily) || !okDays(s.Burp.Then) {
		apiBadRequest(c, "hm_na_err_days")
		return
	}
	if len(req.Tasks) > attentionMaxTasks {
		apiBadRequest(c, "api_invalid_payload")
		return
	}

	db := DBFromContext(c)
	today := attentionToday(c)
	known := map[int]bool{}
	for _, z := range attentionZones(db) {
		known[z.ID] = true
	}

	attentionMu.Lock()
	defer attentionMu.Unlock()
	old := map[int]AttentionTask{}
	nextID := 1
	for _, t := range loadAttentionTasks(db) {
		old[t.ID] = t
		if t.ID >= nextID {
			nextID = t.ID + 1
		}
	}
	for _, t := range req.Tasks {
		if t.ID >= nextID {
			nextID = t.ID + 1
		}
	}

	tasks := make([]AttentionTask, 0, len(req.Tasks))
	used := map[int]bool{}
	for _, in := range req.Tasks {
		name := strings.Join(strings.Fields(in.Name), " ")
		if name == "" || utf8.RuneCountInString(name) > attentionMaxNameLen {
			apiBadRequest(c, "hm_na_err_name")
			return
		}
		if in.Every < 1 || in.Every > attentionMaxEvery {
			apiBadRequest(c, "hm_na_err_every")
			return
		}
		prev, existed := old[in.ID]
		if !existed || used[in.ID] {
			in.ID = nextID
			nextID++
		}
		used[in.ID] = true
		t := AttentionTask{ID: in.ID, Name: name, Icon: attentionIcon(in.Icon), Every: in.Every, Zones: map[int]string{}}
		for _, zid := range in.Zones {
			if !known[zid] {
				apiBadRequest(c, "hm_na_err_tents")
				return
			}
			if d, ok := prev.Zones[zid]; ok && existed {
				t.Zones[zid] = d
			} else {
				t.Zones[zid] = today
			}
		}
		if len(t.Zones) == 0 {
			apiBadRequest(c, "hm_na_err_tents")
			return
		}
		tasks = append(tasks, t)
	}

	if err := saveAttentionJSON(db, attentionSettingsName, s); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	if err := saveAttentionJSON(db, attentionTasksName, tasks); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(200, gin.H{"settings": s, "tasks": tasks})
}

var (
	attentionKeyRE   = regexp.MustCompile(`^[a-z]+(:[0-9a-z]+){0,2}$`)
	attentionBasisRE = regexp.MustCompile(`^[0-9A-Za-z@,:._-]*$`)
)

// HideAttentionHandler hides one line (POST /home/attention/hide):
// how "tomorrow" until the next day, or "until" while basis is unchanged.
func HideAttentionHandler(c *gin.Context) {
	var req struct {
		Key   string `json:"key"`
		How   string `json:"how"`
		Basis string `json:"basis"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Key) > 60 || !attentionKeyRE.MatchString(req.Key) ||
		(req.How != "tomorrow" && req.How != "until") || len(req.Basis) > 4000 || !attentionBasisRE.MatchString(req.Basis) {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	db := DBFromContext(c)
	today := attentionToday(c)
	h := attentionHide{How: req.How, At: today}
	if req.How == "tomorrow" {
		h.Until = addDays(today, 1)
	} else {
		h.Basis = req.Basis
	}
	attentionMu.Lock()
	defer attentionMu.Unlock()
	hidden := loadAttentionHidden(db)
	pruneAttentionHidden(hidden, today)
	hidden[req.Key] = h
	if len(hidden) > attentionMaxHidden {
		// Drop the oldest so the setting can't grow without end.
		keys := make([]string, 0, len(hidden))
		for k := range hidden {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return hidden[keys[i]].At < hidden[keys[j]].At })
		for _, k := range keys[:len(hidden)-attentionMaxHidden] {
			if k != req.Key {
				delete(hidden, k)
			}
		}
	}
	if err := saveAttentionJSON(db, attentionHiddenName, hidden); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(200, gin.H{"hidden": hidden})
}

// ShowAttentionHandler brings a hidden line back (POST /home/attention/show).
func ShowAttentionHandler(c *gin.Context) {
	var req struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Key) > 60 || !attentionKeyRE.MatchString(req.Key) {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	db := DBFromContext(c)
	today := attentionToday(c)
	attentionMu.Lock()
	defer attentionMu.Unlock()
	hidden := loadAttentionHidden(db)
	pruneAttentionHidden(hidden, today)
	delete(hidden, req.Key)
	if err := saveAttentionJSON(db, attentionHiddenName, hidden); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(200, gin.H{"hidden": hidden})
}

// validAttentionDay is a YYYY-MM-DD day from ten years ago up to today.
func validAttentionDay(day, today string) bool {
	if _, err := time.Parse(dayLayout, day); err != nil {
		return false
	}
	return day <= today && day >= addDays(today, -3650)
}

// DoneAttentionHandler records a task as done in one tent, or curing
// jars as burped (POST /home/attention/done). It sets the day given and
// returns the previous one, which Undo sends back:
//
//	{"task_id": 1, "zone_id": 2, "day": "2026-11-10"}
//	{"burp": [{"plant_id": 7, "day": "2026-11-10"}, {"plant_id": 8, "day": ""}]}
//
// A burp day of "" forgets the plant's last burp (Undo of its first one).
func DoneAttentionHandler(c *gin.Context) {
	var req struct {
		TaskID int    `json:"task_id"`
		ZoneID int    `json:"zone_id"`
		Day    string `json:"day"`
		Burp   []struct {
			PlantID int    `json:"plant_id"`
			Day     string `json:"day"`
		} `json:"burp"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	db := DBFromContext(c)
	today := attentionToday(c)
	attentionMu.Lock()
	defer attentionMu.Unlock()

	if req.Burp != nil {
		if len(req.Burp) == 0 || len(req.Burp) > 500 {
			apiBadRequest(c, "api_invalid_payload")
			return
		}
		burped := loadAttentionBurped(db)
		prev := make([]gin.H, 0, len(req.Burp))
		for _, b := range req.Burp {
			if b.Day != "" && !validAttentionDay(b.Day, today) {
				apiBadRequest(c, "api_invalid_payload")
				return
			}
			var exists int
			if err := db.QueryRow(`SELECT COUNT(*) FROM plant WHERE id = $1`, b.PlantID).Scan(&exists); err != nil || exists == 0 {
				apiNotFound(c, "api_plant_not_found")
				return
			}
			prev = append(prev, gin.H{"plant_id": b.PlantID, "day": burped[b.PlantID]})
			if b.Day == "" {
				delete(burped, b.PlantID)
			} else {
				burped[b.PlantID] = b.Day
			}
		}
		// Forget plants deleted since (the page ignores ones no longer curing).
		alive := map[int]bool{}
		if rows, err := db.Query(`SELECT id FROM plant`); err == nil {
			for rows.Next() {
				var id int
				if rows.Scan(&id) == nil {
					alive[id] = true
				}
			}
			rows.Close()
			for id := range burped {
				if !alive[id] {
					delete(burped, id)
				}
			}
		}
		if err := saveAttentionJSON(db, attentionBurpedName, burped); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
		c.JSON(200, gin.H{"previous": prev})
		return
	}

	if !validAttentionDay(req.Day, today) {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	tasks := loadAttentionTasks(db)
	for i := range tasks {
		if tasks[i].ID != req.TaskID {
			continue
		}
		prev, ok := tasks[i].Zones[req.ZoneID]
		if !ok {
			break
		}
		tasks[i].Zones[req.ZoneID] = req.Day
		if err := saveAttentionJSON(db, attentionTasksName, tasks); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
		c.JSON(200, gin.H{"previous": prev})
		return
	}
	apiNotFound(c, "hm_na_err_task_gone")
}
