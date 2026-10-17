package handlers

// Smoke reports: tasting notes for flower you grew (linked to a plant) or
// bought (source "bought"). See migration 037 and views/smoke*.html.

import (
	"database/sql"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/utils"
)

// Choice lists. Keys are stored; the text comes from sr_w_<key> (words) and
// sr_m_<key> (methods) in the locale files. Anything else the user types is
// kept as their own words (custom).
var (
	SmokeEffects = []string{"relaxed", "happy", "euphoric", "uplifted", "creative", "energetic", "focused", "talkative",
		"giggly", "tingly", "calm", "sleepy", "hungry", "aroused", "body_high", "head_high"}
	SmokeSideEffects = []string{"dry_mouth", "dry_eyes", "couch_lock", "anxious", "paranoid", "headache", "dizzy", "racing_heart", "foggy"}
	SmokeSmells      = []string{"earthy", "pine", "skunk", "diesel", "citrus", "lemon", "orange", "grape", "berry", "tropical", "sweet",
		"candy", "floral", "herbal", "spicy", "woody", "cheese", "chemical", "mint", "vanilla", "coffee", "sour", "pungent"}
	SmokeTastes = []string{"sweet", "sour", "earthy", "citrus", "lemon", "berry", "grape", "tropical", "pine", "diesel", "spicy",
		"herbal", "woody", "creamy", "vanilla", "mint", "nutty", "chocolate", "cookie", "coffee", "smooth", "harsh"}
	SmokeBud     = []string{"dense", "frosty", "sticky", "well_trimmed", "colorful", "airy", "leafy", "too_dry", "too_moist"}
	SmokeMethods = []string{"joint", "blunt", "spliff", "pipe", "bong", "bubbler", "one_hitter", "vape", "edible"}

	smokeTagKinds = map[string][]string{"effect": SmokeEffects, "side": SmokeSideEffects, "smell": SmokeSmells, "taste": SmokeTastes, "bud": SmokeBud}

	// Fields that can be switched off in Settings (smoke.hidden).
	SmokeHideableFields = []string{"harvested", "grow_space", "lights", "container", "medium", "nutrients", "days_to_harvest",
		"dry_cure", "method", "side", "bud", "again", "thc_cbd", "dispensary"}
)

const (
	smokeSourceGrown  = "grown"
	smokeSourceBought = "bought"
	maxSmokeTags      = 40
	maxSmokeText      = 255
	maxSmokePhotos    = 12
)

// SmokeTag is one picked choice (or the user's own words when Custom).
type SmokeTag struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
}

// SmokePhoto is one of a report's photos, in display order.
type SmokePhoto struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

// SmokeReport is a stored report plus what's worked out from it (overall
// score, price per gram, plant name).
type SmokeReport struct {
	ID            int          `json:"id"`
	PlantID       *int         `json:"plant_id"`
	PlantName     string       `json:"plant_name"`
	StrainID      *int         `json:"strain_id"`
	Source        string       `json:"source"`
	StrainName    string       `json:"strain_name"`
	BreederName   string       `json:"breeder_name"`
	SmokedOn      string       `json:"smoked_on"`
	THC           string       `json:"thc"`
	CBD           string       `json:"cbd"`
	HarvestedOn   string       `json:"harvested_on"`
	GrowSpace     string       `json:"grow_space"`
	Lights        string       `json:"lights"`
	Container     string       `json:"container"`
	Medium        string       `json:"medium"`
	Nutrients     string       `json:"nutrients"`
	DaysToHarvest *int         `json:"days_to_harvest"`
	DryDays       *int         `json:"dry_days"`
	CureDays      *int         `json:"cure_days"`
	Dispensary    string       `json:"dispensary"`
	Price         *float64     `json:"price"`
	AmountG       *float64     `json:"amount_g"`
	Batch         string       `json:"batch"`
	Method        string       `json:"method"`
	Again         string       `json:"again"`
	ScoreBud      *int         `json:"score_bud"`
	ScoreTaste    *int         `json:"score_taste"`
	ScoreSmell    *int         `json:"score_smell"`
	ScoreEffects  *int         `json:"score_effects"`
	Notes         string       `json:"notes"`
	Tags          []SmokeTag   `json:"tags"`
	Photos        []SmokePhoto `json:"photos"`
	Overall       *float64     `json:"overall"`
	PricePerGram  *float64     `json:"price_per_gram"`
}

// SmokeWeights says how much each score counts in the overall score (0-5;
// 0 leaves it out). Stored in settings as "bud:1,taste:1,smell:1,effects:1".
type SmokeWeights struct {
	Bud     int `json:"bud"`
	Taste   int `json:"taste"`
	Smell   int `json:"smell"`
	Effects int `json:"effects"`
}

func DefaultSmokeWeights() SmokeWeights { return SmokeWeights{1, 1, 1, 1} }

func (w SmokeWeights) String() string {
	return "bud:" + strconv.Itoa(w.Bud) + ",taste:" + strconv.Itoa(w.Taste) + ",smell:" + strconv.Itoa(w.Smell) + ",effects:" + strconv.Itoa(w.Effects)
}

func clampWeight(n int) int { return int(math.Max(0, math.Min(5, float64(n)))) }

// ParseSmokeWeights reads the stored weights; anything missing or broken
// counts 1.
func ParseSmokeWeights(s string) SmokeWeights {
	w := DefaultSmokeWeights()
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), ":")
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if !ok || err != nil {
			continue
		}
		n = clampWeight(n)
		switch strings.TrimSpace(k) {
		case "bud":
			w.Bud = n
		case "taste":
			w.Taste = n
		case "smell":
			w.Smell = n
		case "effects":
			w.Effects = n
		}
	}
	return w
}

// SmokeOverall is the weighted average of the scores that are filled in, to
// one decimal; nil when none of the counted scores is filled in.
func SmokeOverall(r SmokeReport, w SmokeWeights) *float64 {
	sum, weight := 0.0, 0
	add := func(score *int, wt int) {
		if score != nil && wt > 0 {
			sum += float64(*score * wt)
			weight += wt
		}
	}
	add(r.ScoreBud, w.Bud)
	add(r.ScoreTaste, w.Taste)
	add(r.ScoreSmell, w.Smell)
	add(r.ScoreEffects, w.Effects)
	if weight == 0 {
		return nil
	}
	v := math.Round(sum/float64(weight)*10) / 10
	return &v
}

// SmokeSettings are the Settings → Grow setup choices for smoke reports.
type SmokeSettings struct {
	Weights SmokeWeights    `json:"weights"`
	Hidden  map[string]bool `json:"hidden"`
}

func LoadSmokeSettings(db *sql.DB) SmokeSettings {
	w, _ := GetSetting(db, "smoke.weights")
	h, _ := GetSetting(db, "smoke.hidden")
	s := SmokeSettings{Weights: ParseSmokeWeights(w), Hidden: map[string]bool{}}
	for _, f := range strings.Split(h, ",") {
		if f = strings.TrimSpace(f); f != "" {
			s.Hidden[f] = true
		}
	}
	return s
}

// SaveSmokeSettingsHandler: POST /smoke/settings {weights, hidden: [...]}.
func SaveSmokeSettingsHandler(c *gin.Context) {
	var req struct {
		Weights SmokeWeights `json:"weights"`
		Hidden  []string     `json:"hidden"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	w := SmokeWeights{clampWeight(req.Weights.Bud), clampWeight(req.Weights.Taste), clampWeight(req.Weights.Smell), clampWeight(req.Weights.Effects)}
	allowed := map[string]bool{}
	for _, f := range SmokeHideableFields {
		allowed[f] = true
	}
	var hidden []string
	for _, f := range req.Hidden {
		if allowed[f] {
			hidden = append(hidden, f)
		}
	}
	sort.Strings(hidden)
	db := DBFromContext(c)
	if err := UpdateSetting(db, nil, "smoke.weights", w.String()); err != nil {
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	if err := UpdateSetting(db, nil, "smoke.hidden", strings.Join(hidden, ",")); err != nil {
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	c.JSON(http.StatusOK, gin.H{"weights": w, "hidden": hidden})
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

const smokeCols = `r.id, r.plant_id, COALESCE(p.name, ''), r.strain_id, r.source, r.strain_name, r.breeder_name, r.smoked_on,
	r.thc, r.cbd, r.harvested_on, r.grow_space, r.lights, r.container, r.medium, r.nutrients,
	r.days_to_harvest, r.dry_days, r.cure_days, r.dispensary, r.price, r.amount_g, r.batch, r.method, r.again,
	r.score_bud, r.score_taste, r.score_smell, r.score_effects, r.notes`

func scanSmoke(row interface{ Scan(...any) error }) (SmokeReport, error) {
	var r SmokeReport
	var plantID, strainID, dth, dry, cure, sb, st, ss, se sql.NullInt64
	var price, amount sql.NullFloat64
	err := row.Scan(&r.ID, &plantID, &r.PlantName, &strainID, &r.Source, &r.StrainName, &r.BreederName, &r.SmokedOn,
		&r.THC, &r.CBD, &r.HarvestedOn, &r.GrowSpace, &r.Lights, &r.Container, &r.Medium, &r.Nutrients,
		&dth, &dry, &cure, &r.Dispensary, &price, &amount, &r.Batch, &r.Method, &r.Again,
		&sb, &st, &ss, &se, &r.Notes)
	if err != nil {
		return r, err
	}
	ip := func(n sql.NullInt64) *int {
		if !n.Valid {
			return nil
		}
		v := int(n.Int64)
		return &v
	}
	fp := func(n sql.NullFloat64) *float64 {
		if !n.Valid {
			return nil
		}
		v := n.Float64
		return &v
	}
	r.PlantID, r.StrainID = ip(plantID), ip(strainID)
	r.DaysToHarvest, r.DryDays, r.CureDays = ip(dth), ip(dry), ip(cure)
	r.ScoreBud, r.ScoreTaste, r.ScoreSmell, r.ScoreEffects = ip(sb), ip(st), ip(ss), ip(se)
	r.Price, r.AmountG = fp(price), fp(amount)
	r.Tags, r.Photos = []SmokeTag{}, []SmokePhoto{}
	return r, nil
}

func finishSmoke(r *SmokeReport, w SmokeWeights) {
	r.Overall = SmokeOverall(*r, w)
	if r.Price != nil && r.AmountG != nil && *r.AmountG > 0 {
		v := math.Round(*r.Price / *r.AmountG * 100) / 100
		r.PricePerGram = &v
	}
}

// LoadSmokeReports returns reports matching where (e.g. "r.plant_id = $1"),
// newest smoked first, with tags, photos and the overall score.
func LoadSmokeReports(db *sql.DB, w SmokeWeights, where string, args ...any) ([]SmokeReport, error) {
	q := `SELECT ` + smokeCols + ` FROM smoke_report r LEFT JOIN plant p ON p.id = r.plant_id`
	if where != "" {
		q += " WHERE " + where
	}
	q += " ORDER BY r.smoked_on DESC, r.id DESC"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []SmokeReport{}
	byID := map[int]int{}
	for rows.Next() {
		r, err := scanSmoke(rows)
		if err != nil {
			return nil, err
		}
		byID[r.ID] = len(list)
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return list, nil
	}
	tagRows, err := db.Query(`SELECT report_id, kind, name, custom FROM smoke_report_tag ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for tagRows.Next() {
		var id, custom int
		var t SmokeTag
		if err := tagRows.Scan(&id, &t.Kind, &t.Name, &custom); err == nil {
			if i, ok := byID[id]; ok {
				t.Custom = custom != 0
				list[i].Tags = append(list[i].Tags, t)
			}
		}
	}
	tagRows.Close()
	photoRows, err := db.Query(`SELECT id, report_id, path FROM smoke_report_photo ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	for photoRows.Next() {
		var id, rid int
		var path string
		if err := photoRows.Scan(&id, &rid, &path); err == nil {
			if i, ok := byID[rid]; ok {
				list[i].Photos = append(list[i].Photos, SmokePhoto{ID: id, URL: webPath(path)})
			}
		}
	}
	photoRows.Close()
	for i := range list {
		finishSmoke(&list[i], w)
	}
	return list, nil
}

// GetSmokeReport loads one report, or sql.ErrNoRows.
func GetSmokeReport(db *sql.DB, id int) (SmokeReport, error) {
	list, err := LoadSmokeReports(db, LoadSmokeSettings(db).Weights, "r.id = $1", id)
	if err != nil {
		return SmokeReport{}, err
	}
	if len(list) == 0 {
		return SmokeReport{}, sql.ErrNoRows
	}
	return list[0], nil
}

// SmokeAllHandler: GET /smoke/all, every report (for the Smoke Reports page).
func SmokeAllHandler(c *gin.Context) {
	db := DBFromContext(c)
	set := LoadSmokeSettings(db)
	list, err := LoadSmokeReports(db, set.Weights, "")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load smoke reports")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"reports": list, "weights": set.Weights})
}

// SmokeSummary is what the Plants page and the plant/strain pages show.
type SmokeSummary struct {
	Count  int      `json:"count"`
	Latest *float64 `json:"latest"` // overall score of the most recently smoked report
	LastID int      `json:"last_id"`
}

// SmokeSummariesByPlant maps plant id → its reports' count and latest score.
func SmokeSummariesByPlant(db *sql.DB) map[int]SmokeSummary {
	out := map[int]SmokeSummary{}
	list, err := LoadSmokeReports(db, LoadSmokeSettings(db).Weights, "r.plant_id IS NOT NULL")
	if err != nil {
		return out
	}
	for _, r := range list { // newest first
		s := out[*r.PlantID]
		if s.Count == 0 {
			s.Latest, s.LastID = r.Overall, r.ID
		}
		s.Count++
		out[*r.PlantID] = s
	}
	return out
}

// ---------------------------------------------------------------------------
// Saving
// ---------------------------------------------------------------------------

type smokeInput struct {
	PlantID       *int       `json:"plant_id"`
	StrainID      *int       `json:"strain_id"`
	Source        string     `json:"source"`
	StrainName    string     `json:"strain_name"`
	BreederName   string     `json:"breeder_name"`
	SmokedOn      string     `json:"smoked_on"`
	THC           string     `json:"thc"`
	CBD           string     `json:"cbd"`
	HarvestedOn   string     `json:"harvested_on"`
	GrowSpace     string     `json:"grow_space"`
	Lights        string     `json:"lights"`
	Container     string     `json:"container"`
	Medium        string     `json:"medium"`
	Nutrients     string     `json:"nutrients"`
	DaysToHarvest *int       `json:"days_to_harvest"`
	DryDays       *int       `json:"dry_days"`
	CureDays      *int       `json:"cure_days"`
	Dispensary    string     `json:"dispensary"`
	Price         *float64   `json:"price"`
	AmountG       *float64   `json:"amount_g"`
	Batch         string     `json:"batch"`
	Method        string     `json:"method"`
	Again         string     `json:"again"`
	ScoreBud      *int       `json:"score_bud"`
	ScoreTaste    *int       `json:"score_taste"`
	ScoreSmell    *int       `json:"score_smell"`
	ScoreEffects  *int       `json:"score_effects"`
	Notes         string     `json:"notes"`
	Tags          []SmokeTag `json:"tags"`
	// "Also add to my Strains" (bought flower with no strain_id).
	AddToStrains bool `json:"add_to_strains"`
	// The StrainCompass listing the strain was looked up from, if any.
	SCSlug           string `json:"sc_slug"`
	SCListingBreeder string `json:"sc_listing_breeder"`
}

var errSmokeInvalid = errors.New("invalid")

func (in *smokeInput) clean() error {
	trim := func(p *string, max int) error {
		*p = strings.TrimSpace(*p)
		if len(*p) > max {
			return errSmokeInvalid
		}
		return nil
	}
	for _, f := range []*string{&in.StrainName, &in.BreederName, &in.THC, &in.CBD, &in.GrowSpace, &in.Lights, &in.Container,
		&in.Medium, &in.Nutrients, &in.Dispensary, &in.Batch, &in.Method} {
		if err := trim(f, maxSmokeText); err != nil {
			return err
		}
	}
	if err := trim(&in.Notes, utils.MaxDescriptionLength); err != nil {
		return err
	}
	if in.StrainName == "" {
		return errSmokeInvalid
	}
	if in.Source != smokeSourceBought {
		in.Source = smokeSourceGrown
	}
	for _, d := range []*string{&in.SmokedOn, &in.HarvestedOn} {
		*d = strings.TrimSpace(*d)
		if *d != "" {
			if _, err := time.Parse(utils.LayoutDate, *d); err != nil {
				return errSmokeInvalid
			}
		}
	}
	switch in.Again {
	case "", "yes", "no", "maybe":
	default:
		return errSmokeInvalid
	}
	for _, s := range []**int{&in.ScoreBud, &in.ScoreTaste, &in.ScoreSmell, &in.ScoreEffects} {
		if *s != nil && (**s < 1 || **s > 10) {
			return errSmokeInvalid
		}
	}
	for _, n := range []**int{&in.DaysToHarvest, &in.DryDays, &in.CureDays} {
		if *n != nil && (**n < 0 || **n > 3650) {
			return errSmokeInvalid
		}
	}
	for _, f := range []**float64{&in.Price, &in.AmountG} {
		if *f != nil && (**f < 0 || math.IsNaN(**f) || math.IsInf(**f, 0) || **f > 1e7) {
			return errSmokeInvalid
		}
	}
	if len(in.Tags) > maxSmokeTags {
		return errSmokeInvalid
	}
	var tags []SmokeTag
	seen := map[string]bool{}
	for _, t := range in.Tags {
		t.Name = strings.TrimSpace(t.Name)
		list, ok := smokeTagKinds[t.Kind]
		if !ok || t.Name == "" || len(t.Name) > 60 {
			return errSmokeInvalid
		}
		if !t.Custom && !contains(list, t.Name) {
			return errSmokeInvalid
		}
		key := t.Kind + "\x00" + strings.ToLower(t.Name)
		if !seen[key] {
			seen[key] = true
			tags = append(tags, t)
		}
	}
	in.Tags = tags
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func nullIntPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullFloatPtr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func smokeArgs(in smokeInput) []any {
	return []any{nullIntPtr(in.PlantID), nullIntPtr(in.StrainID), in.Source, in.StrainName, in.BreederName, in.SmokedOn,
		in.THC, in.CBD, in.HarvestedOn, in.GrowSpace, in.Lights, in.Container, in.Medium, in.Nutrients,
		nullIntPtr(in.DaysToHarvest), nullIntPtr(in.DryDays), nullIntPtr(in.CureDays), in.Dispensary,
		nullFloatPtr(in.Price), nullFloatPtr(in.AmountG), in.Batch, in.Method, in.Again,
		nullIntPtr(in.ScoreBud), nullIntPtr(in.ScoreTaste), nullIntPtr(in.ScoreSmell), nullIntPtr(in.ScoreEffects), in.Notes}
}

func replaceSmokeTags(tx *sql.Tx, id int, tags []SmokeTag) error {
	if _, err := tx.Exec(`DELETE FROM smoke_report_tag WHERE report_id = $1`, id); err != nil {
		return err
	}
	for _, t := range tags {
		custom := 0
		if t.Custom {
			custom = 1
		}
		if _, err := tx.Exec(`INSERT INTO smoke_report_tag (report_id, kind, name, custom) VALUES ($1, $2, $3, $4)`, id, t.Kind, t.Name, custom); err != nil {
			return err
		}
	}
	return nil
}

func bindSmoke(c *gin.Context) (smokeInput, bool) {
	var in smokeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return in, false
	}
	if err := in.clean(); err != nil {
		apiBadRequest(c, "api_smoke_invalid")
		return in, false
	}
	db := DBFromContext(c)
	if in.PlantID != nil {
		var n int
		if db.QueryRow(`SELECT COUNT(*) FROM plant WHERE id = $1`, *in.PlantID).Scan(&n); n == 0 {
			apiBadRequest(c, "api_smoke_invalid")
			return in, false
		}
	}
	if in.StrainID != nil {
		var n int
		if db.QueryRow(`SELECT COUNT(*) FROM strain WHERE id = $1`, *in.StrainID).Scan(&n); n == 0 {
			in.StrainID = nil
		}
	}
	if in.StrainID == nil && in.AddToStrains {
		id, err := addSmokeStrain(c, in)
		if err != nil {
			logger.Log.WithError(err).Warn("Couldn't add the smoke report's strain to Strains")
			apiInternalError(c, "api_smoke_strain_failed")
			return in, false
		}
		in.StrainID = &id
	}
	return in, true
}

// CreateSmokeReportHandler: POST /smoke.
func CreateSmokeReportHandler(c *gin.Context) {
	in, ok := bindSmoke(c)
	if !ok {
		return
	}
	db := DBFromContext(c)
	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	defer func() { _ = tx.Rollback() }()
	var id int
	err = tx.QueryRow(`INSERT INTO smoke_report (plant_id, strain_id, source, strain_name, breeder_name, smoked_on,
		thc, cbd, harvested_on, grow_space, lights, container, medium, nutrients,
		days_to_harvest, dry_days, cure_days, dispensary, price, amount_g, batch, method, again,
		score_bud, score_taste, score_smell, score_effects, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
		RETURNING id`, smokeArgs(in)...).Scan(&id)
	if err == nil {
		err = replaceSmokeTags(tx, id, in.Tags)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		logger.Log.WithError(err).Error("Failed to save smoke report")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "strain_id": in.StrainID})
}

func smokeID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_request")
		return 0, false
	}
	var n int
	if DBFromContext(c).QueryRow(`SELECT COUNT(*) FROM smoke_report WHERE id = $1`, id).Scan(&n); n == 0 {
		apiNotFound(c, "api_smoke_not_found")
		return 0, false
	}
	return id, true
}

// UpdateSmokeReportHandler: PUT /smoke/:id.
func UpdateSmokeReportHandler(c *gin.Context) {
	id, ok := smokeID(c)
	if !ok {
		return
	}
	in, ok := bindSmoke(c)
	if !ok {
		return
	}
	db := DBFromContext(c)
	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	defer func() { _ = tx.Rollback() }()
	args := append(smokeArgs(in), id)
	_, err = tx.Exec(`UPDATE smoke_report SET plant_id = $1, strain_id = $2, source = $3, strain_name = $4, breeder_name = $5,
		smoked_on = $6, thc = $7, cbd = $8, harvested_on = $9, grow_space = $10, lights = $11, container = $12, medium = $13,
		nutrients = $14, days_to_harvest = $15, dry_days = $16, cure_days = $17, dispensary = $18, price = $19, amount_g = $20,
		batch = $21, method = $22, again = $23, score_bud = $24, score_taste = $25, score_smell = $26, score_effects = $27,
		notes = $28, updated_at = CURRENT_TIMESTAMP WHERE id = $29`, args...)
	if err == nil {
		err = replaceSmokeTags(tx, id, in.Tags)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		logger.Log.WithError(err).Error("Failed to update smoke report")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "strain_id": in.StrainID})
}

// DeleteSmokeReportHandler: DELETE /smoke/:id (and its photo files).
func DeleteSmokeReportHandler(c *gin.Context) {
	id, ok := smokeID(c)
	if !ok {
		return
	}
	db := DBFromContext(c)
	var paths []string
	if rows, err := db.Query(`SELECT path FROM smoke_report_photo WHERE report_id = $1`, id); err == nil {
		for rows.Next() {
			var p string
			if rows.Scan(&p) == nil {
				paths = append(paths, p)
			}
		}
		rows.Close()
	}
	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{`DELETE FROM smoke_report_photo WHERE report_id = $1`, `DELETE FROM smoke_report_tag WHERE report_id = $1`, `DELETE FROM smoke_report WHERE id = $1`} {
		if _, err := tx.Exec(q, id); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	for _, p := range paths {
		removeSmokePhotoFile(UploadDirFromContext(c), p)
	}
	apiOK(c, "api_smoke_deleted")
}
