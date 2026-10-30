package handlers

// Demo mode (BUDMINDER_DEMO=1): BudMinder as a public, locked-down demo.
//
// Everything here is enforced on the server. The demo pages also lock text
// boxes and script the strain search, but that is only for looks: anyone can
// skip the page and send requests directly, so this file decides what a
// request may change.
//
//   - Every visitor is signed in automatically; the login, password and
//     settings pages can't be used.
//   - Reading is allowed, except exports, logs, backups and anything that
//     reaches outside the demo.
//   - A change (POST/PUT/DELETE) is refused unless its route is in demoRules.
//     Each rule lists the JSON fields it accepts and checks every value:
//     numbers within limits, dates within a window, choices from fixed
//     lists, short notes (DemoMaxNote), and other text only from values the
//     demo already holds or the server fills in itself. Unknown fields are
//     refused. Notes can be typed freely because each visitor works in a
//     private copy (package demo) that nobody else sees.
//   - Request bodies are capped and changes are rate-limited per visitor.
//   - StrainCompass answers come from a recording in demodata/, so the demo
//     never contacts an outside service.

import (
	"bytes"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"isley/alerts"
	"isley/climate"
	"isley/config"
	"isley/logger"
)

const (
	// DemoMaxBody caps every request body; nothing in the demo needs more.
	DemoMaxBody = 64 << 10
	// DemoWritesPerMinute is how many changes one visitor may send a minute.
	DemoWritesPerMinute = 15
	// DemoMaxNote caps a typed note (about 80-90 words).
	DemoMaxNote = 500
	// DemoStraincompassHost is the made-up address the demo's StrainCompass
	// lookups go to; demoTransport answers it from the recording.
	DemoStraincompassHost = "straincompass.demo.invalid"
)

//go:embed demodata/straincompass_grape_og.json
var demoData embed.FS

// demoStraincompass is the recorded StrainCompass search, parsed once.
var demoStraincompass = sync.OnceValues(func() ([]byte, map[string]map[string]any) {
	raw, err := demoData.ReadFile("demodata/straincompass_grape_og.json")
	if err != nil {
		panic(err)
	}
	var list struct {
		Strains []map[string]any `json:"strains"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		panic(err)
	}
	bySlug := map[string]map[string]any{}
	for _, s := range list.Strains {
		if slug, _ := s["slug"].(string); slug != "" {
			bySlug[slug] = s
		}
	}
	return raw, bySlug
})

var installDemoTransport sync.Once

// SetupDemo points this engine's strain lookups at the recording (CannaDB
// off) and makes the shared HTTP client answer that made-up address from the
// recording. Other addresses go out as usual, so engines that aren't demos,
// such as parallel tests, are unaffected.
func SetupDemo(store interface {
	SetStraincompassEnabled(int)
	SetStraincompassBaseURL(string)
	SetStraincompassAPIKey(string)
	SetCannadbEnabled(int)
}) {
	store.SetStraincompassEnabled(1)
	store.SetStraincompassBaseURL("http://" + DemoStraincompassHost + "/api/")
	store.SetStraincompassAPIKey("")
	store.SetCannadbEnabled(0)
	installDemoTransport.Do(func() {
		next := httpClient.Transport
		if next == nil {
			next = http.DefaultTransport
		}
		httpClient.Transport = demoTransport{next: next}
	})
}

type demoTransport struct{ next http.RoundTripper }

func (t demoTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != DemoStraincompassHost {
		return t.next.RoundTrip(req)
	}
	raw, _ := demoStraincompass()
	body := raw
	q := strings.ToLower(req.URL.Query().Get("q"))
	if !strings.Contains(q, "grape") {
		body = []byte(`{"strains":[],"total":0}`)
	} else if b := req.URL.Query().Get("breeder"); b != "" {
		// Same shape as StrainCompass: the list, narrowed to that breeder.
		var list map[string]any
		_ = json.Unmarshal(raw, &list)
		var kept []any
		for _, s := range list["strains"].([]any) {
			if m, _ := s.(map[string]any); strings.EqualFold(fmt.Sprint(m["breeder"]), b) {
				kept = append(kept, m)
			}
		}
		list["strains"], list["total"] = kept, len(kept)
		body, _ = json.Marshal(list)
	}
	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header:  http.Header{"Content-Type": []string{"application/json"}},
		Body:    io.NopCloser(bytes.NewReader(body)),
		Request: req, ContentLength: int64(len(body)),
	}, nil
}

// demoBlockedReads are pages and downloads the demo doesn't serve.
var demoBlockedReads = map[string]bool{
	"/activities/export/csv":           true,
	"/activities/export/xlsx":          true,
	"/settings/logs":                   true,
	"/settings/logs/download":          true,
	"/settings/backup/status":          true,
	"/settings/backup/list":            true,
	"/settings/backup/download/:name":  true,
	"/settings/backup/restore/status":  true,
	"/settings/backup/sqlite/download": true,
	"/sensors/dumpACI":                 true,
	"/strains/cannadb/search":          true,
	"/api/overlay":                     true,
}

// demoCheck checks one JSON value and returns the value to pass on.
type demoCheck func(c *gin.Context, v any) (any, error)

type demoRule struct {
	fields map[string]demoCheck
	// dropUnknown leaves out fields not in the list instead of refusing:
	// for pages that send their whole form, where a left-out field means
	// "keep what's stored".
	dropUnknown bool
	// fix sets the values the server decides, after the field checks.
	fix func(c *gin.Context, body map[string]any) error
}

var errDemo = errors.New("not allowed in the demo")

// ---------------------------------------------------------------------------
// Field checks
// ---------------------------------------------------------------------------

func dNumber(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

// dInt accepts a whole number from min to max.
func dInt(min, max int64) demoCheck {
	return func(_ *gin.Context, v any) (any, error) {
		n, ok := v.(json.Number)
		if !ok {
			return nil, errDemo
		}
		i, err := n.Int64()
		if err != nil || i < min || i > max {
			return nil, errDemo
		}
		return v, nil
	}
}

// dNum accepts any number from min to max.
func dNum(min, max float64) demoCheck {
	return func(_ *gin.Context, v any) (any, error) {
		f, ok := dNumber(v)
		if !ok || f < min || f > max {
			return nil, errDemo
		}
		return v, nil
	}
}

// dOpt also accepts null.
func dOpt(check demoCheck) demoCheck {
	return func(c *gin.Context, v any) (any, error) {
		if v == nil {
			return nil, nil
		}
		return check(c, v)
	}
}

func dBool(_ *gin.Context, v any) (any, error) {
	if _, ok := v.(bool); !ok {
		return nil, errDemo
	}
	return v, nil
}

// dOneOf accepts one of the given strings.
// dRange is dInt for a [min, max] pair.
func dRange(r [2]int) demoCheck { return dInt(int64(r[0]), int64(r[1])) }

func dOneOf(options ...string) demoCheck {
	return func(_ *gin.Context, v any) (any, error) {
		s, ok := v.(string)
		if !ok {
			return nil, errDemo
		}
		for _, o := range options {
			if s == o {
				return v, nil
			}
		}
		return nil, errDemo
	}
}

// dEmpty accepts only nothing: null or "".
func dEmpty(_ *gin.Context, v any) (any, error) {
	if v == nil || v == "" {
		return v, nil
	}
	return nil, errDemo
}

var demoDateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}(:\d{2})?)?$`)

// dDate accepts a date (optionally with a time) from 2015 to two years ahead.
func dDate(allowEmpty bool) demoCheck {
	return func(_ *gin.Context, v any) (any, error) {
		s, ok := v.(string)
		if !ok {
			return nil, errDemo
		}
		if s == "" && allowEmpty {
			return v, nil
		}
		if !demoDateRE.MatchString(s) {
			return nil, errDemo
		}
		d, err := time.Parse("2006-01-02", s[:10])
		if err != nil || d.Year() < 2015 || d.After(time.Now().AddDate(2, 0, 0)) {
			return nil, errDemo
		}
		return v, nil
	}
}

// dIntList accepts up to max whole numbers, each 1 or more.
func dIntList(max int) demoCheck {
	return func(c *gin.Context, v any) (any, error) {
		list, ok := v.([]any)
		if !ok || len(list) > max {
			return nil, errDemo
		}
		for _, x := range list {
			if _, err := dInt(1, 1<<31-1)(c, x); err != nil {
				return nil, err
			}
		}
		return v, nil
	}
}

// dObject accepts an object with only these fields.
func dObject(fields map[string]demoCheck) demoCheck {
	return func(c *gin.Context, v any) (any, error) {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, errDemo
		}
		if err := demoCheckFields(c, m, fields, false); err != nil {
			return nil, err
		}
		return m, nil
	}
}

// dList accepts up to max objects with only these fields.
func dList(max int, fields map[string]demoCheck) demoCheck {
	return func(c *gin.Context, v any) (any, error) {
		list, ok := v.([]any)
		if !ok || len(list) > max {
			return nil, errDemo
		}
		for _, x := range list {
			if _, err := dObject(fields)(c, x); err != nil {
				return nil, err
			}
		}
		return v, nil
	}
}

// dTuple accepts a list of exactly len(checks) items, each passing its check.
func dTuple(checks ...demoCheck) demoCheck {
	return func(c *gin.Context, v any) (any, error) {
		list, ok := v.([]any)
		if !ok || len(list) != len(checks) {
			return nil, errDemo
		}
		for i, x := range list {
			if _, err := checks[i](c, x); err != nil {
				return nil, err
			}
		}
		return v, nil
	}
}

// dLightSchedule accepts Edit zone's light schedule: null, or lights-on and
// lights-off times ("HH:MM") for Seedling, Veg and Flower.
func dLightSchedule(c *gin.Context, v any) (any, error) {
	if v == nil {
		return v, nil
	}
	clock := func(_ *gin.Context, x any) (any, error) {
		if s, ok := x.(string); ok {
			if _, ok := climate.ParseClock(s); ok {
				return x, nil
			}
		}
		return nil, errDemo
	}
	pair := dTuple(clock, clock)
	return dObject(map[string]demoCheck{climate.Seedling: pair, climate.Veg: pair, climate.Flower: pair})(c, v)
}

// dNote accepts a typed note up to DemoMaxNote characters. It is stored
// as plain text like any note and only shown in this visitor's copy.
func dNote(_ *gin.Context, v any) (any, error) {
	s, ok := v.(string)
	if !ok || utf8.RuneCountInString(s) > DemoMaxNote {
		return nil, errDemo
	}
	return v, nil
}

// dKnown accepts "" or text the demo already holds: the values query returns.
// The demo's own data was written by its build, so repeating it is safe.
func dKnown(query string) demoCheck {
	return func(c *gin.Context, v any) (any, error) {
		s, ok := v.(string)
		if !ok {
			return nil, errDemo
		}
		if s == "" {
			return v, nil
		}
		rows, err := DBFromContext(c).Query(query)
		if err != nil {
			return nil, errDemo
		}
		defer rows.Close()
		for rows.Next() {
			var known sql.NullString
			if rows.Scan(&known) == nil && known.String == s {
				return v, nil
			}
		}
		return nil, errDemo
	}
}

var demoPercentRE = regexp.MustCompile(`^\d{1,2}(\.\d{1,2})?\s?%?$`)

// dPercentText accepts "" or a short percentage such as "22" or "21.5%".
func dPercentText(_ *gin.Context, v any) (any, error) {
	s, ok := v.(string)
	if !ok || (s != "" && !demoPercentRE.MatchString(s)) {
		return nil, errDemo
	}
	return v, nil
}

// dRecordedSlug accepts a StrainCompass slug from the recording (or "").
func dRecordedSlug(allowEmpty bool) demoCheck {
	return func(_ *gin.Context, v any) (any, error) {
		s, ok := v.(string)
		if !ok {
			return nil, errDemo
		}
		_, bySlug := demoStraincompass()
		if (s == "" && allowEmpty) || bySlug[s] != nil {
			return v, nil
		}
		return nil, errDemo
	}
}

func demoCheckFields(c *gin.Context, body map[string]any, fields map[string]demoCheck, dropUnknown bool) error {
	for k, v := range body {
		check, ok := fields[k]
		if !ok {
			if dropUnknown {
				delete(body, k)
				continue
			}
			return errDemo
		}
		nv, err := check(c, v)
		if err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		body[k] = nv
	}
	return nil
}

// ---------------------------------------------------------------------------
// The changes the demo allows
// ---------------------------------------------------------------------------

var (
	demoID      = dInt(1, 1<<31-1)
	demoOptID   = dOpt(demoID)
	demoPercent = dOpt(dNum(0, 100))

	demoAlertRuleFields = map[string]demoCheck{
		"sensor_id": demoID, "side": dOneOf(alerts.SideLow, alerts.SideHigh),
		"limit":        dNum(-alerts.RuleLimitMax, alerts.RuleLimitMax),
		"hold_minutes": dRange(alerts.RuleHoldRange), "enabled": dBool,
	}

	demoDayFields = map[string]demoCheck{
		"activity_ids": dIntList(20),
		"note":         dNote,
		"measurements": dList(20, map[string]demoCheck{
			"activity_id": demoID, "metric_id": demoID, "value": dOpt(dNum(-1000, 100000)),
		}),
	}

	smokeKnown = func(col string) demoCheck {
		return dKnown("SELECT DISTINCT " + col + " FROM smoke_report")
	}
	demoSmokeFields = map[string]demoCheck{
		"plant_id": demoOptID, "strain_id": demoOptID,
		"source":       dOneOf(smokeSourceGrown, smokeSourceBought),
		"strain_name":  demoStrainName,
		"breeder_name": dKnown("SELECT name FROM breeder"),
		"smoked_on":    dDate(true), "harvested_on": dDate(true),
		"thc": dPercentText, "cbd": dPercentText,
		"grow_space": dKnown("SELECT name FROM zones UNION SELECT grow_space FROM smoke_report"),
		"lights":     smokeKnown("lights"), "container": smokeKnown("container"),
		"medium": smokeKnown("medium"), "nutrients": smokeKnown("nutrients"),
		"dispensary": smokeKnown("dispensary"), "batch": smokeKnown("batch"),
		"method":          smokeKnown("method"),
		"days_to_harvest": dOpt(dInt(0, 3650)), "dry_days": dOpt(dInt(0, 3650)), "cure_days": dOpt(dInt(0, 3650)),
		"price": dOpt(dNum(0, 10000)), "amount_g": dOpt(dNum(0, 10000)),
		"again":     dOneOf("", "yes", "no", "maybe"),
		"score_bud": dOpt(dInt(1, 10)), "score_taste": dOpt(dInt(1, 10)),
		"score_smell": dOpt(dInt(1, 10)), "score_effects": dOpt(dInt(1, 10)),
		"notes": dNote,
		// Tags only from BudMinder's built-in lists (the save checks the name).
		"tags": dList(maxSmokeTags, map[string]demoCheck{
			"kind": demoTagKind, "name": dTagName, "custom": dOneOfBool(false),
		}),
		"add_to_strains":     dBool,
		"sc_slug":            dRecordedSlug(true),
		"sc_listing_breeder": demoRecordedBreeder,
	}
)

func demoTagKind(c *gin.Context, v any) (any, error) {
	s, ok := v.(string)
	if _, known := smokeTagKinds[s]; !ok || !known {
		return nil, errDemo
	}
	return v, nil
}

func dTagName(_ *gin.Context, v any) (any, error) {
	if s, ok := v.(string); !ok || s == "" || len(s) > 60 {
		return nil, errDemo
	}
	return v, nil
}

func dOneOfBool(want bool) demoCheck {
	return func(_ *gin.Context, v any) (any, error) {
		if b, ok := v.(bool); !ok || b != want {
			return nil, errDemo
		}
		return v, nil
	}
}

// demoStrainName accepts a strain the demo has, or one from the recording.
func demoStrainName(c *gin.Context, v any) (any, error) {
	s, _ := v.(string)
	_, bySlug := demoStraincompass()
	for _, r := range bySlug {
		if r["name"] == s {
			return v, nil
		}
	}
	return dKnown("SELECT name FROM strain")(c, v)
}

func demoRecordedBreeder(_ *gin.Context, v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return nil, errDemo
	}
	if s == "" {
		return v, nil
	}
	_, bySlug := demoStraincompass()
	for _, r := range bySlug {
		if r["breeder"] == s {
			return v, nil
		}
	}
	return nil, errDemo
}

var demoRules = map[string]demoRule{
	// Notebook: tick activities, enter numbers, pick a sample note.
	"POST /plants/day": {fields: merge(demoDayFields, map[string]demoCheck{
		"plant_ids": dIntList(50), "date": dDate(false),
	})},
	"PUT /plant/:id/day/:date":    {fields: demoDayFields},
	"DELETE /plant/:id/day/:date": {fields: map[string]demoCheck{}},
	"POST /plant/status": {fields: map[string]demoCheck{
		"plant_id": demoID, "status_id": demoID, "date": dDate(false),
	}},

	// Add a plant from a strain the demo has; the server names it.
	"POST /plants": {
		fields: map[string]demoCheck{
			"name": dTagName, "zone_id": demoOptID, "new_zone": dEmpty,
			"strain_id": demoID, "new_strain": dEmpty, "status_id": demoID,
			"date": dDate(false), "sensors": dOneOf("", "[]"),
			"clone": dInt(0, 1), "parent_id": dInt(0, 1<<31-1), "decrement_seed_count": dBool,
		},
		fix: demoNamePlant,
	},

	// Seed Vault and Wanted.
	"POST /strains/:id/seeds": {fields: map[string]demoCheck{
		"count": dInt(1, 100), "seed_type": dOpt(dOneOf("", "clone", "feminized", "regular")),
		"seeds_added_on": dOpt(dDate(true)),
		"seed_location":  dOpt(dKnown("SELECT DISTINCT seed_location FROM strain")),
	}},
	"POST /strains/:id/seeds/use": {fields: map[string]demoCheck{"count": dInt(1, 100)}},
	"POST /strains/:id/wanted":    {fields: map[string]demoCheck{"wanted": dBool}},

	// Edit a strain's numbers; its names and text stay as stored.
	"PUT /strains/:id": {
		fields: map[string]demoCheck{
			"indica": dInt(0, 100), "sativa": dInt(0, 100), "autoflower": dBool,
			"seed_count": dInt(0, 1000), "cycle_time": dInt(0, 365), "cycle_time_min": dOpt(dInt(0, 365)),
			"wanted": dOpt(dBool), "seeds_added_on": dOpt(dDate(true)),
			"seed_type": dOpt(dOneOf("", "clone", "feminized", "regular")),
			"cannabinoids": dOpt(dObject(map[string]demoCheck{
				"thc_min": demoPercent, "thc_max": demoPercent, "cbd_min": demoPercent,
				"cbd_max": demoPercent, "cbn_max": demoPercent, "cbg_max": demoPercent,
			})),
		},
		dropUnknown: true,
		fix:         demoKeepStrainText,
	},

	// One-click import from the recorded StrainCompass search.
	"POST /strains/straincompass/import": {
		fields: map[string]demoCheck{
			"slug": dRecordedSlug(false), "name": dTagName, "listing_breeder": demoRecordedBreeder,
			"breeder_id": demoOptID, "new_breeder": dEmpty,
		},
		fix: demoRecordedName,
	},

	// Settings > Photos: Keep originals / Compression Options and the
	// quality (also "Use quality NN" on the compare page). Only the
	// visitor's copy changes; uploads stay blocked, and Compress existing
	// photos (/settings/photos/existing/...) has no rule, so it's refused.
	"POST /settings/photos": {fields: map[string]demoCheck{"compress": dBool, "quality": dPhotoQuality}},

	// Settings > Grow setup > Climate targets: switches, weeks, limits and
	// targets (the handler also checks each measure's range and order).
	"POST /settings/climate": {fields: map[string]demoCheck{
		"split_lights": dBool, "flower_phases": dBool, "temp_by_stage": dBool, "rh_by_stage": dBool,
		"phase_weeks": dTuple(dInt(1, 30), dInt(1, 30)),
		"temp_limits": dTuple(dOpt(dNum(-20, 60)), dOpt(dNum(-20, 60))),
		"rh_limits":   dTuple(dOpt(dNum(0, 100)), dOpt(dNum(0, 100))),
		"targets": dList(len(climateRows()), map[string]demoCheck{
			"stage":   dOneOf(climate.Seedling, climate.Veg, climate.Flower, climate.Drying),
			"phase":   dOneOf("", climate.Stretch, climate.Bloom, climate.Ripen),
			"measure": dOneOf(climate.VPD, climate.Temp, climate.RH),
			"lights":  dOneOf("on", "off"),
			"low":     dOpt(dNum(-20, 100)), "target": dOpt(dNum(-20, 100)), "high": dOpt(dNum(-20, 100)),
		}),
	}},

	// Settings > Alerts > Climate alerts: the mode and times, nothing else.
	// Mail settings and test emails have no rule, so they stay refused.
	"POST /settings/alerts/climate": {fields: map[string]demoCheck{
		"mode":                    dOneOf(alerts.ModeOff, alerts.ModeDryRun, alerts.ModeEmail),
		"safety_readings":         dRange(alerts.SafetyReadingsRange),
		"window_minutes":          dRange(alerts.WindowMinutesRange),
		"drift_hours":             dRange(alerts.DriftHoursRange),
		"pause_minutes":           dRange(alerts.PauseMinutesRange),
		"repeat_critical_minutes": dRange(alerts.RepeatCriticalRange),
		"repeat_warning_hours":    dRange(alerts.RepeatWarningRange),
	}},

	// Sensor alerts: numbers and fixed choices only.
	"POST /settings/alerts/rules":       {fields: demoAlertRuleFields},
	"PUT /settings/alerts/rules/:id":    {fields: demoAlertRuleFields},
	"DELETE /settings/alerts/rules/:id": {fields: map[string]demoCheck{}},

	// Edit zone: a zone name the demo has, the light schedule, light check,
	// plant stage and climate sensors (the handler checks they're in the zone).
	"PUT /zones/:id": {fields: map[string]demoCheck{
		"zone_name": dKnown("SELECT name FROM zones"), "light_schedule": dLightSchedule,
		"light_sensor_id": demoOptID, "light_on_above": dOpt(dNum(0, 100000)), "stage_plant_id": demoOptID,
		"vpd_temp_sensor_id": demoOptID, "vpd_humidity_sensor_id": demoOptID,
		"vpd_source": dOpt(dOneOf("sensor", "calc")), "vpd_sensor_id": demoOptID,
		"leaf_temp_offset": dOpt(dNum(-10, 10)),
	}},

	// Smoke reports: scores, tags and choices; photos can't be added.
	"POST /smoke":                 {fields: demoSmokeFields},
	"PUT /smoke/:id":              {fields: demoSmokeFields},
	"PUT /smoke/:id/photos/order": {fields: map[string]demoCheck{"ids": dIntList(maxSmokePhotos)}},
}

// dPhotoQuality accepts a quality Settings > Photos offers (50-90, steps of 5).
func dPhotoQuality(c *gin.Context, v any) (any, error) {
	if _, err := dInt(50, 90)(c, v); err != nil {
		return nil, err
	}
	n, _ := v.(json.Number).Int64()
	if !ValidPhotoQuality(int(n)) {
		return nil, errDemo
	}
	return v, nil
}

func merge(a, b map[string]demoCheck) map[string]demoCheck {
	out := map[string]demoCheck{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// demoNamePlant names a new plant after its strain: "Grape OG #3".
func demoNamePlant(c *gin.Context, body map[string]any) error {
	db := DBFromContext(c)
	id := body["strain_id"]
	var name string
	if err := db.QueryRow(`SELECT name FROM strain WHERE id = $1`, fmt.Sprint(id)).Scan(&name); err != nil {
		return errDemo
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM plant WHERE strain_id = $1`, fmt.Sprint(id)).Scan(&n)
	body["name"] = fmt.Sprintf("%s #%d", name, n+1)
	return nil
}

// demoKeepStrainText puts a strain's stored name, breeder and text back into
// an edit, so only its numbers and choices can change.
func demoKeepStrainText(c *gin.Context, body map[string]any) error {
	var name, desc, short, url sql.NullString
	var breeder sql.NullInt64
	err := DBFromContext(c).QueryRow(`SELECT name, breeder_id, description, short_desc, url FROM strain WHERE id = $1`,
		c.Param("id")).Scan(&name, &breeder, &desc, &short, &url)
	if err != nil {
		return errDemo
	}
	body["name"], body["description"], body["short_desc"], body["url"] = name.String, desc.String, short.String, url.String
	body["new_breeder"] = ""
	if breeder.Valid {
		body["breeder_id"] = breeder.Int64
	} else {
		body["breeder_id"] = nil
	}
	return nil
}

// demoRecordedName takes the strain's name and breeder from the recording.
func demoRecordedName(_ *gin.Context, body map[string]any) error {
	_, bySlug := demoStraincompass()
	rec := bySlug[fmt.Sprint(body["slug"])]
	if rec == nil {
		return errDemo
	}
	body["name"], body["listing_breeder"] = rec["name"], rec["breeder"]
	return nil
}

// ---------------------------------------------------------------------------
// The middleware
// ---------------------------------------------------------------------------

// DemoGuard runs before the session and CSRF checks: it caps the body before
// anything reads it, and drops X-API-KEY, which would otherwise skip the CSRF
// check (the demo has no sensor ingest).
func DemoGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, DemoMaxBody)
		c.Request.Header.Del("X-API-KEY")
		c.Next()
	}
}

// DemoCopies hands each demo visitor their own database and settings
// (package demo). A visitor reads the shared demo until their first change.
type DemoCopies interface {
	ForRequest(id, ip string, write bool) (newID string, db *sql.DB, store *config.Store, err error)
}

// DemoMiddleware applies demo mode to every request. Register it after the
// session, CSRF and database middlewares and before the routes. copies may
// be nil (every visitor then shares one database).
func DemoMiddleware(copies DemoCopies) gin.HandlerFunc {
	var mu sync.Mutex
	writes := map[string]int{}
	window := time.Now()

	return func(c *gin.Context) {
		c.Header("X-Robots-Tag", "noindex, nofollow")
		route := c.FullPath()

		demoSignIn(c)

		read := c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead
		if read {
			switch {
			case route == "/login" || route == "/change-password":
				c.Redirect(http.StatusFound, "/")
				c.Abort()
				return
			case demoBlockedReads[route]:
				c.String(http.StatusForbidden, T(c, "demo_not_available"))
				c.Abort()
				return
			}
			if demoUseCopy(c, copies, false) {
				c.Next()
			}
			return
		}

		rule, ok := demoRules[c.Request.Method+" "+route]
		if !ok {
			demoRefuse(c, "route")
			return
		}

		mu.Lock()
		if time.Since(window) > time.Minute {
			writes, window = map[string]int{}, time.Now()
		}
		writes[c.ClientIP()]++
		tooMany := writes[c.ClientIP()] > DemoWritesPerMinute
		mu.Unlock()
		if tooMany {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": T(c, "demo_slow_down"), "demo": true})
			return
		}

		// JSON only. A form body is refused even when it looks empty here:
		// the CSRF check may already have read it.
		if ct := c.ContentType(); ct != "" && ct != "application/json" {
			demoRefuse(c, "content type")
			return
		}
		// The visitor's own copy, before any check reads the database.
		if !demoUseCopy(c, copies, true) {
			return
		}
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": T(c, "demo_not_available"), "demo": true})
			return
		}
		body := map[string]any{}
		if len(bytes.TrimSpace(raw)) > 0 {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if c.ContentType() != "application/json" || dec.Decode(&body) != nil || dec.More() {
				demoRefuse(c, "body")
				return
			}
		}
		if err := demoCheckFields(c, body, rule.fields, rule.dropUnknown); err != nil {
			demoRefuse(c, err.Error())
			return
		}
		if rule.fix != nil {
			if err := rule.fix(c, body); err != nil {
				demoRefuse(c, "fix")
				return
			}
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			raw, _ = json.Marshal(body)
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		c.Request.ContentLength = int64(len(raw))
		c.Next()
	}
}

// demoUseCopy points this request at the visitor's copy (making one for a
// change) and remembers it in their session. False means it answered.
func demoUseCopy(c *gin.Context, copies DemoCopies, write bool) bool {
	if copies == nil {
		return true
	}
	session := sessions.Default(c)
	id, _ := session.Get("demo_copy").(string)
	newID, db, store, err := copies.ForRequest(id, c.ClientIP(), write)
	if err != nil {
		logger.Log.WithError(err).Warn("Demo: no copy for this visitor")
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": T(c, "demo_slow_down"), "demo": true})
		return false
	}
	if newID != id {
		session.Set("demo_copy", newID)
		_ = session.Save()
	}
	c.Set(contextKeyDB, db)
	SetConfigStoreOnContext(c, store)
	return true
}

func demoRefuse(c *gin.Context, why string) {
	logger.Log.WithField("route", c.Request.Method+" "+c.FullPath()).WithField("why", why).Info("Demo refused a change")
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": T(c, "demo_not_available"), "demo": true})
}

// demoSignIn signs the visitor in, so the demo needs no password.
func demoSignIn(c *gin.Context) {
	session := sessions.Default(c)
	if v, _ := session.Get("logged_in").(bool); v {
		return
	}
	version, _ := GetSetting(DBFromContext(c), "session_version")
	if session.Get("csrf_token") == nil {
		session.Set("csrf_token", GenerateCSRFToken())
	}
	session.Set("logged_in", true)
	session.Set("force_password_change", false)
	session.Set("session_version", version)
	_ = session.Save()
}
