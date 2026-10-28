package handlers

// Smoke reports, part 2: the form's prefill from a plant, photos (upload,
// image URL, a journal photo), "Also add to my Strains", and page data.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/utils"
)

const maxSmokePhotoSize = 10 << 20

// ---------------------------------------------------------------------------
// Prefill from a plant
// ---------------------------------------------------------------------------

// SmokePlantOption is a plant in the form's plant picker.
type SmokePlantOption struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Strain    string `json:"strain"`
	Breeder   string `json:"breeder"`
	Zone      string `json:"zone"`
	Status    string `json:"status"`
	Harvested string `json:"harvested"` // YYYY-MM-DD, "" while still growing
}

// SmokePrefill is what BudMinder knows about a plant for a new report.
type SmokePrefill struct {
	PlantID       int    `json:"plant_id"`
	PlantName     string `json:"plant_name"`
	StrainID      *int   `json:"strain_id"`
	StrainName    string `json:"strain_name"`
	BreederName   string `json:"breeder_name"`
	GrowSpace     string `json:"grow_space"`
	HarvestedOn   string `json:"harvested_on"`
	CuringOn      string `json:"curing_on"` // cure time = date smoked − this
	DaysToHarvest *int   `json:"days_to_harvest"`
	DryDays       *int   `json:"dry_days"`
	THC           string `json:"thc"`
	CBD           string `json:"cbd"`
	// Suggestions: a light port in the plant's tent, and what the last report
	// for that tent said.
	LightPort string `json:"light_port"`
	Lights    string `json:"lights"`
	Container string `json:"container"`
	Medium    string `json:"medium"`
	Nutrients string `json:"nutrients"`
}

// Same rule as the light icon in Manage sensors (sensor-kinds.js portIcon).
var lightPortRe = regexp.MustCompile(`(?i)light|led|lamp|sun|bloom|veg|fc\d`)

func stageDate(h []NotebookStage, names ...string) string {
	for _, s := range h { // oldest first: the first time any of these was reached
		for _, n := range names {
			if strings.EqualFold(s.Status, n) {
				return s.Date
			}
		}
	}
	return ""
}

func smokeDays(a, b string) *int {
	ta, err1 := time.Parse(utils.LayoutDate, a)
	tb, err2 := time.Parse(utils.LayoutDate, b)
	if err1 != nil || err2 != nil || tb.Before(ta) {
		return nil
	}
	d := int(tb.Sub(ta).Hours()/24 + 0.5)
	return &d
}

func rangeText(lo, hi sql.NullFloat64) string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	switch {
	case lo.Valid && hi.Valid && lo.Float64 != hi.Float64:
		return f(lo.Float64) + "–" + f(hi.Float64)
	case hi.Valid:
		return f(hi.Float64)
	case lo.Valid:
		return f(lo.Float64)
	}
	return ""
}

// SmokePlantOptions lists every plant for the picker, harvested ones first.
func SmokePlantOptions(db *sql.DB) []SmokePlantOption {
	plants, err := LoadNotebookPlants(db)
	if err != nil {
		return []SmokePlantOption{}
	}
	breeders := map[int]string{}
	if rows, err := db.Query(`SELECT p.id, COALESCE(b.name, '') FROM plant p LEFT JOIN strain s ON s.id = p.strain_id LEFT JOIN breeder b ON b.id = s.breeder_id`); err == nil {
		for rows.Next() {
			var id int
			var b string
			if rows.Scan(&id, &b) == nil {
				breeders[id] = b
			}
		}
		rows.Close()
	}
	var harvested, growing []SmokePlantOption
	for _, p := range plants {
		o := SmokePlantOption{ID: p.ID, Name: p.Name, Strain: p.Strain, Breeder: breeders[p.ID], Zone: p.Zone, Status: p.Status,
			Harvested: stageDate(p.History, "drying", "curing", "success")}
		if strings.EqualFold(p.Status, "dead") {
			continue
		}
		if o.Harvested != "" {
			harvested = append(harvested, o)
		} else {
			growing = append(growing, o)
		}
	}
	return append(append([]SmokePlantOption{}, harvested...), growing...)
}

// SmokePrefillFor fills a new report from what BudMinder knows about a plant.
func SmokePrefillFor(db *sql.DB, plantID int) (SmokePrefill, error) {
	plants, err := LoadNotebookPlants(db)
	if err != nil {
		return SmokePrefill{}, err
	}
	var np *NotebookPlant
	for i := range plants {
		if plants[i].ID == plantID {
			np = &plants[i]
		}
	}
	if np == nil {
		return SmokePrefill{}, sql.ErrNoRows
	}
	p := SmokePrefill{PlantID: np.ID, PlantName: np.Name, StrainName: np.Strain, GrowSpace: np.Zone}
	if np.StrainID > 0 {
		id := np.StrainID
		p.StrainID = &id
		var thcLo, thcHi, cbdLo, cbdHi sql.NullFloat64
		_ = db.QueryRow(`SELECT COALESCE(b.name, ''), s.thc_min, s.thc_max, s.cbd_min, s.cbd_max FROM strain s
			LEFT JOIN breeder b ON b.id = s.breeder_id WHERE s.id = $1`, id).Scan(&p.BreederName, &thcLo, &thcHi, &cbdLo, &cbdHi)
		p.THC, p.CBD = rangeText(thcLo, thcHi), rangeText(cbdLo, cbdHi)
	}
	p.HarvestedOn = stageDate(np.History, "drying", "curing", "success")
	if np.Start != "" && p.HarvestedOn != "" {
		p.DaysToHarvest = smokeDays(np.Start, p.HarvestedOn)
	}
	drying, curing := stageDate(np.History, "drying"), stageDate(np.History, "curing")
	p.CuringOn = curing
	if drying != "" && curing != "" {
		p.DryDays = smokeDays(drying, curing)
	}
	if np.ZoneID > 0 {
		if rows, err := db.Query(`SELECT name FROM sensors WHERE zone_id = $1 AND type LIKE 'ACIP.%' ORDER BY id`, np.ZoneID); err == nil {
			for rows.Next() {
				var n string
				if rows.Scan(&n) == nil && p.LightPort == "" && lightPortRe.MatchString(n) {
					p.LightPort = n
				}
			}
			rows.Close()
		}
	}
	if p.GrowSpace != "" {
		_ = db.QueryRow(`SELECT lights, container, medium, nutrients FROM smoke_report WHERE grow_space = $1
			ORDER BY smoked_on DESC, id DESC LIMIT 1`, p.GrowSpace).Scan(&p.Lights, &p.Container, &p.Medium, &p.Nutrients)
	}
	return p, nil
}

// SmokePrefillHandler: GET /smoke/prefill?plant_id=.
func SmokePrefillHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("plant_id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	p, err := SmokePrefillFor(DBFromContext(c), id)
	if errors.Is(err, sql.ErrNoRows) {
		apiNotFound(c, "api_plant_not_found")
		return
	}
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, p)
}

// ---------------------------------------------------------------------------
// "Also add to my Strains"
// ---------------------------------------------------------------------------

// addSmokeStrain adds the report's strain to Strains (library only: no seeds)
// and returns its id. From a StrainCompass listing it's imported with the
// listing's details (effects, flavors, lineage); otherwise just the name and
// breeder. An existing strain with the same name and breeder is reused.
func addSmokeStrain(c *gin.Context, in smokeInput) (int, error) {
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)
	breeder := in.BreederName
	if breeder == "" {
		breeder = "Unknown Breeder"
	}
	var existing int
	if err := db.QueryRow(`SELECT s.id FROM strain s JOIN breeder b ON b.id = s.breeder_id
		WHERE LOWER(s.name) = LOWER($1) AND LOWER(b.name) = LOWER($2) ORDER BY s.id LIMIT 1`, in.StrainName, breeder).Scan(&existing); err == nil {
		return existing, nil
	}
	breederID, _, err := findOrCreateBreeder(db, breeder)
	if err != nil {
		return 0, err
	}
	// The picked StrainCompass listing is already in Strains: link to it
	// as it is (a re-import would rename it and overwrite its text).
	if slug := strings.TrimSpace(in.SCSlug); slug != "" {
		if err := db.QueryRow(`SELECT id FROM strain WHERE straincompass_slug = $1 AND breeder_id = $2 ORDER BY id LIMIT 1`,
			slug, breederID).Scan(&existing); err == nil {
			return existing, nil
		}
	}
	var id int
	if slug := strings.TrimSpace(in.SCSlug); slug != "" && store.StraincompassEnabled() == 1 {
		rec, err := straincompassGetStrainBySlug(store.StraincompassBaseURL(), store.StraincompassAPIKey(), slug, in.StrainName, in.SCListingBreeder)
		if err == nil && rec.Name != "" {
			s := mapStraincompassStrain(rec)
			s.Name = in.StrainName // the user's spelling wins
			if id, err = upsertStraincompassStrain(db, breederID, s); err != nil {
				return 0, err
			}
			if err := replaceStraincompassAttributes(db, id, rec); err != nil {
				logger.Log.WithError(err).Warn("Smoke report strain: attributes not imported")
			}
			if _, err := seedLineageFromNote(db, id, rec.Lineage); err != nil {
				logger.Log.WithError(err).Warn("Smoke report strain: lineage not recorded")
			}
		} else if err != nil {
			logger.Log.WithError(err).Warn("Smoke report strain: StrainCompass lookup failed; adding name and breeder only")
		}
	}
	if id == 0 {
		if id, err = insertBareStrain(db, in.StrainName, breederID); err != nil {
			return 0, err
		}
	}
	store.SetBreeders(GetBreeders(db))
	store.SetStrains(GetStrains(db))
	return id, nil
}

// insertBareStrain adds a strain with only a name and breeder.
// autoflower is an INTEGER column: 0, not false (Postgres refuses a boolean).
func insertBareStrain(db *sql.DB, name string, breederID int) (int, error) {
	var id int
	err := db.QueryRow(`INSERT INTO strain (name, breeder_id, indica, sativa, autoflower, seed_count, description, short_desc, cycle_time)
		VALUES ($1, $2, 50, 50, 0, 0, '', '', 0) RETURNING id`, name, breederID).Scan(&id)
	return id, err
}

// ---------------------------------------------------------------------------
// Photos
// ---------------------------------------------------------------------------

func smokeUploadDir(uploadDir string) string { return filepath.Join(uploadDir, "smoke") }

// removeSmokePhotoFile deletes a stored photo, refusing anything outside
// uploads/smoke.
func removeSmokePhotoFile(uploadDir, stored string) {
	root, err1 := filepath.Abs(smokeUploadDir(uploadDir))
	target, err2 := filepath.Abs(filepath.FromSlash(stored))
	if stored == "" || err1 != nil || err2 != nil || !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return
	}
	_ = os.Remove(target)
}

// saveSmokePhoto checks the bytes are a real image and adds it to the report
// as its last photo.
func saveSmokePhoto(db *sql.DB, uploadDir string, reportID int, body []byte) (SmokePhoto, error) {
	ext, err := validateImageBytes(body)
	if err != nil {
		return SmokePhoto{}, errSmokeNotImage
	}
	if body, err = utils.StripImageMetadata(body); err != nil {
		return SmokePhoto{}, errSmokeNotImage
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM smoke_report_photo WHERE report_id = $1`, reportID).Scan(&n)
	if n >= maxSmokePhotos {
		return SmokePhoto{}, errSmokeTooMany
	}
	if err := os.MkdirAll(smokeUploadDir(uploadDir), 0o755); err != nil {
		return SmokePhoto{}, err
	}
	path := filepath.Join(smokeUploadDir(uploadDir), fmt.Sprintf("smoke_%d_%d%s", reportID, time.Now().UnixNano(), ext))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return SmokePhoto{}, err
	}
	var id int
	if err := db.QueryRow(`INSERT INTO smoke_report_photo (report_id, path, sort_order) VALUES ($1, $2, $3) RETURNING id`,
		reportID, path, n).Scan(&id); err != nil {
		_ = os.Remove(path)
		return SmokePhoto{}, err
	}
	return SmokePhoto{ID: id, URL: webPath(path)}, nil
}

var (
	errSmokeNotImage = errors.New("not an image")
	errSmokeTooMany  = errors.New("too many photos")
	errSmokeTooBig   = errors.New("too large")
)

func smokePhotoError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errSmokeNotImage):
		apiBadRequest(c, "api_invalid_file_type")
	case errors.Is(err, errSmokeTooMany):
		apiBadRequest(c, "api_smoke_too_many_photos")
	case errors.Is(err, errSmokeTooBig):
		apiError(c, http.StatusRequestEntityTooLarge, "api_image_too_large")
	default:
		logger.Log.WithError(err).Error("Failed to save smoke report photo")
		apiInternalError(c, "api_failed_to_save_file")
	}
}

// UploadSmokePhotosHandler: POST /smoke/:id/photos (multipart "photos").
func UploadSmokePhotosHandler(c *gin.Context) {
	id, ok := smokeID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxSmokePhotos)*maxSmokePhotoSize+(1<<20))
	form, err := c.MultipartForm()
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			apiError(c, http.StatusRequestEntityTooLarge, "api_image_too_large")
		} else {
			apiBadRequest(c, "api_failed_to_parse_form")
		}
		return
	}
	var saved []SmokePhoto
	for _, fh := range form.File["photos"] {
		if fh.Size > maxSmokePhotoSize {
			smokePhotoError(c, errSmokeTooBig)
			return
		}
		f, err := fh.Open()
		if err != nil {
			apiBadRequest(c, "api_failed_to_parse_form")
			return
		}
		body, err := io.ReadAll(io.LimitReader(f, maxSmokePhotoSize+1))
		f.Close()
		if err != nil || int64(len(body)) > maxSmokePhotoSize {
			smokePhotoError(c, errSmokeTooBig)
			return
		}
		p, err := saveSmokePhoto(DBFromContext(c), UploadDirFromContext(c), id, body)
		if err != nil {
			smokePhotoError(c, err)
			return
		}
		saved = append(saved, p)
	}
	c.JSON(http.StatusOK, gin.H{"photos": saved})
}

// fetchImageURL downloads an image for a report. client is publicOnlyClient
// in the app (public addresses only, at most 3 https redirects).
func fetchImageURL(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errSmokeNotImage
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", straincompassUserAgent())
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errSmokeTooBig
	}
	return body, nil
}

// SmokePhotoFromURLHandler: POST /smoke/:id/photos/url {url}. BudMinder
// downloads the picture and keeps its own copy.
func SmokePhotoFromURLHandler(c *gin.Context) {
	id, ok := smokeID(c)
	if !ok {
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.URL) > 2048 {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	body, err := fetchImageURL(ctx, publicOnlyClient, req.URL, maxSmokePhotoSize)
	if err != nil {
		if errors.Is(err, errSmokeTooBig) || errors.Is(err, errSmokeNotImage) {
			smokePhotoError(c, err)
			return
		}
		logger.Log.WithError(err).Warn("Couldn't download smoke report photo")
		apiBadRequest(c, "api_smoke_photo_download_failed")
		return
	}
	p, err := saveSmokePhoto(DBFromContext(c), UploadDirFromContext(c), id, body)
	if err != nil {
		smokePhotoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"photos": []SmokePhoto{p}})
}

// SmokePhotoFromJournalHandler: POST /smoke/:id/photos/journal {image_id}.
// The plant photo is copied, so deleting it from the journal later doesn't
// take it off the report.
func SmokePhotoFromJournalHandler(c *gin.Context) {
	id, ok := smokeID(c)
	if !ok {
		return
	}
	var req struct {
		ImageID int `json:"image_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	db := DBFromContext(c)
	var stored string
	if err := db.QueryRow(`SELECT image_path FROM plant_images WHERE id = $1`, req.ImageID).Scan(&stored); err != nil {
		apiNotFound(c, "api_image_not_found")
		return
	}
	uploadDir := UploadDirFromContext(c)
	root, err1 := filepath.Abs(uploadDir)
	src, err2 := filepath.Abs(filepath.FromSlash(stored))
	if err1 != nil || err2 != nil || !strings.HasPrefix(src, root+string(filepath.Separator)) {
		apiNotFound(c, "api_image_not_found")
		return
	}
	body, err := os.ReadFile(src)
	if err != nil || int64(len(body)) > 60<<20 {
		apiNotFound(c, "api_image_not_found")
		return
	}
	p, err := saveSmokePhoto(db, uploadDir, id, body)
	if err != nil {
		smokePhotoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"photos": []SmokePhoto{p}})
}

// DeleteSmokePhotoHandler: DELETE /smoke/:id/photos/:photoID.
func DeleteSmokePhotoHandler(c *gin.Context) {
	id, ok := smokeID(c)
	if !ok {
		return
	}
	pid, _ := strconv.Atoi(c.Param("photoID"))
	db := DBFromContext(c)
	var path string
	if err := db.QueryRow(`SELECT path FROM smoke_report_photo WHERE id = $1 AND report_id = $2`, pid, id).Scan(&path); err != nil {
		apiNotFound(c, "api_image_not_found")
		return
	}
	if _, err := db.Exec(`DELETE FROM smoke_report_photo WHERE id = $1`, pid); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	removeSmokePhotoFile(UploadDirFromContext(c), path)
	apiOK(c, "api_image_deleted")
}

// OrderSmokePhotosHandler: PUT /smoke/:id/photos/order {ids: [...]}; the
// first is the main photo.
func OrderSmokePhotosHandler(c *gin.Context) {
	id, ok := smokeID(c)
	if !ok {
		return
	}
	var req struct {
		IDs []int `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) > maxSmokePhotos {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	db := DBFromContext(c)
	for i, pid := range req.IDs {
		if _, err := db.Exec(`UPDATE smoke_report_photo SET sort_order = $1 WHERE id = $2 AND report_id = $3`, i, pid, id); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
	}
	apiOK(c, "api_smoke_saved")
}

// PlantJournalPhotos lists a plant's journal photos (newest first) for the
// form's "From the journal" tab.
func PlantJournalPhotos(db *sql.DB, plantID int) []gin.H {
	out := []gin.H{}
	rows, err := db.Query(`SELECT id, image_path FROM plant_images WHERE plant_id = $1 ORDER BY image_date DESC, id DESC`, plantID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var p string
		if rows.Scan(&id, &p) == nil {
			out = append(out, gin.H{"id": id, "url": webPath(p)})
		}
	}
	return out
}

// SmokeJournalPhotosHandler: GET /smoke/journal-photos?plant_id=.
func SmokeJournalPhotosHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("plant_id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	c.JSON(http.StatusOK, gin.H{"photos": PlantJournalPhotos(DBFromContext(c), id)})
}

// ---------------------------------------------------------------------------
// Page data
// ---------------------------------------------------------------------------

func smokeTranslations(translations map[string]string) map[string]string {
	t := map[string]string{}
	for k, v := range translations {
		if strings.HasPrefix(k, "sr_") {
			t[k] = v
		}
	}
	return t
}

// SmokeVocab is the choice lists the form and pages use.
func SmokeVocab() gin.H {
	return gin.H{"effect": SmokeEffects, "side": SmokeSideEffects, "smell": SmokeSmells, "taste": SmokeTastes, "bud": SmokeBud, "method": SmokeMethods}
}

// SmokePageData is the Smoke Reports page's data-page JSON.
func SmokePageData(c *gin.Context, loggedIn bool, lang string, translations map[string]string) gin.H {
	return gin.H{"logged_in": loggedIn, "lang": lang, "t": smokeTranslations(translations), "vocab": SmokeVocab(),
		"settings": LoadSmokeSettings(DBFromContext(c))}
}

// SmokeFormData is the New/Edit form's data-page JSON.
func SmokeFormData(c *gin.Context, lang string, translations map[string]string, report *SmokeReport) gin.H {
	db := DBFromContext(c)
	strains := []gin.H{}
	if rows, err := db.Query(`SELECT s.id, s.name, COALESCE(b.name, ''), s.thc_min, s.thc_max, s.cbd_min, s.cbd_max
		FROM strain s LEFT JOIN breeder b ON b.id = s.breeder_id ORDER BY LOWER(s.name)`); err == nil {
		for rows.Next() {
			var id int
			var n, b string
			var tl, th, cl, ch sql.NullFloat64
			if rows.Scan(&id, &n, &b, &tl, &th, &cl, &ch) == nil {
				strains = append(strains, gin.H{"id": id, "name": n, "breeder": b, "thc": rangeText(tl, th), "cbd": rangeText(cl, ch)})
			}
		}
		rows.Close()
	}
	data := gin.H{"lang": lang, "t": smokeTranslations(translations), "vocab": SmokeVocab(),
		"settings": LoadSmokeSettings(db), "plants": SmokePlantOptions(db), "strains": strains,
		"today":         time.Now().Format(utils.LayoutDate),
		"straincompass": ConfigStoreFromContext(c).StraincompassEnabled() == 1}
	if report != nil {
		data["report"] = report
	}
	if id, err := strconv.Atoi(c.Query("plant_id")); err == nil && report == nil {
		data["plant_id"] = id
	}
	if id, err := strconv.Atoi(c.Query("strain_id")); err == nil && report == nil {
		data["strain_id"] = id
	}
	return data
}

// SmokeWord is the text for a choice-list key (or the user's own words).
func SmokeWord(lcl map[string]string, t SmokeTag) string {
	if t.Custom {
		return t.Name
	}
	if s := lcl["sr_w_"+t.Name]; s != "" {
		return s
	}
	return t.Name
}

// SmokeMethodText is the text for a stored method (a key or typed words).
func SmokeMethodText(lcl map[string]string, m string) string {
	if s := lcl["sr_m_"+m]; s != "" {
		return s
	}
	return m
}

// SmokeReportView is what views/smoke-report.html shows for one report.
type SmokeReportView struct {
	R        SmokeReport
	Weights  SmokeWeights
	Hidden   map[string]bool
	Words    map[string][]string // kind → display texts
	Method   string
	Timeline []SmokeReport // the same plant's reports, oldest first
	More     []SmokeCard   // other reports of the same strain
	Strain   gin.H         // the linked strain's type info, if any

	OverallText  string   // "8.8", "" when nothing is scored
	OverallClass string   // "" (8+), "mid" (6+), "low"
	Dots         []string // ten dots: "on", "half" or ""
	Scores       []SmokeScoreRow
	Points       []SmokePoint // the plant-over-time chart
	PriceText    string
	PPGText      string
	Grown        bool
}

// SmokeScoreRow is one score bar.
type SmokeScoreRow struct {
	Key    string // sr_bud, sr_taste, …
	Value  int    // 0 = not scored
	Pct    int
	Weight int
}

// SmokePoint is one report on the plant-over-time chart (viewBox 300×110).
type SmokePoint struct {
	ID      int
	X, Y    float64
	Label   string
	Date    string
	Cure    string
	Current bool
}

// SmokeCard is a small link to another report.
type SmokeCard struct {
	ID           int
	Title        string
	Photo        string
	OverallText  string
	OverallClass string
	Date         string
}

func smokeScoreText(v *float64) (string, string) {
	if v == nil {
		return "", "low"
	}
	cls := "low"
	if *v >= 8 {
		cls = ""
	} else if *v >= 6 {
		cls = "mid"
	}
	return strconv.FormatFloat(*v, 'f', 1, 64), cls
}

func smokeMoney(lcl map[string]string, v float64) string {
	return lcl["sr_currency"] + strconv.FormatFloat(v, 'f', 2, 64)
}

// BuildSmokeReportView loads a report and everything around it.
func BuildSmokeReportView(db *sql.DB, lcl map[string]string, id int) (SmokeReportView, error) {
	set := LoadSmokeSettings(db)
	r, err := GetSmokeReport(db, id)
	if err != nil {
		return SmokeReportView{}, err
	}
	v := SmokeReportView{R: r, Weights: set.Weights, Hidden: set.Hidden, Words: map[string][]string{}, Method: SmokeMethodText(lcl, r.Method)}
	for _, t := range r.Tags {
		v.Words[t.Kind] = append(v.Words[t.Kind], SmokeWord(lcl, t))
	}
	if r.PlantID != nil {
		list, _ := LoadSmokeReports(db, set.Weights, "r.plant_id = $1", *r.PlantID)
		for i := len(list) - 1; i >= 0; i-- {
			v.Timeline = append(v.Timeline, list[i])
		}
	}
	others, _ := LoadSmokeReports(db, set.Weights, "LOWER(r.strain_name) = LOWER($1) AND r.id <> $2", r.StrainName, r.ID)
	for _, o := range others {
		c := SmokeCard{ID: o.ID, Title: o.PlantName, Date: o.SmokedOn}
		if c.Title == "" {
			c.Title = o.Dispensary
		}
		if c.Title == "" {
			c.Title = o.StrainName
		}
		if len(o.Photos) > 0 {
			c.Photo = o.Photos[0].URL
		}
		c.OverallText, c.OverallClass = smokeScoreText(o.Overall)
		v.More = append(v.More, c)
	}

	v.Grown = r.Source != smokeSourceBought
	v.OverallText, v.OverallClass = smokeScoreText(r.Overall)
	for i := 1; i <= 10; i++ {
		d := ""
		if r.Overall != nil && *r.Overall >= float64(i) {
			d = "on"
		} else if r.Overall != nil && *r.Overall >= float64(i)-0.5 {
			d = "half"
		}
		v.Dots = append(v.Dots, d)
	}
	for _, row := range []struct {
		key string
		val *int
		w   int
	}{{"sr_bud", r.ScoreBud, set.Weights.Bud}, {"sr_taste", r.ScoreTaste, set.Weights.Taste}, {"sr_smell", r.ScoreSmell, set.Weights.Smell}, {"sr_effects", r.ScoreEffects, set.Weights.Effects}} {
		sr := SmokeScoreRow{Key: row.key, Weight: row.w}
		if row.val != nil {
			sr.Value, sr.Pct = *row.val, *row.val*10
		}
		v.Scores = append(v.Scores, sr)
	}
	if r.Price != nil {
		v.PriceText = smokeMoney(lcl, *r.Price)
	}
	if r.PricePerGram != nil {
		v.PPGText = smokeMoney(lcl, *r.PricePerGram) + "/g"
	}
	if n := len(v.Timeline); n > 1 {
		for i, t := range v.Timeline {
			if t.Overall == nil {
				continue
			}
			p := SmokePoint{ID: t.ID, X: 40 + 220*float64(i)/float64(n-1), Y: 100 - *t.Overall*9, Date: t.SmokedOn, Current: t.ID == r.ID}
			p.Label, _ = smokeScoreText(t.Overall)
			if t.CureDays != nil {
				p.Cure = strconv.Itoa(*t.CureDays)
			}
			v.Points = append(v.Points, p)
		}
	}
	if r.StrainID != nil {
		var indica, sativa int
		var auto bool
		if db.QueryRow(`SELECT indica, sativa, COALESCE(autoflower, 0) FROM strain WHERE id = $1`, *r.StrainID).Scan(&indica, &sativa, &auto) == nil {
			v.Strain = gin.H{"indica": indica, "sativa": sativa, "autoflower": auto}
		}
	}
	return v, nil
}

// StrainSmoke is the strain page's "Smoke reports" card.
type StrainSmoke struct {
	Cards    []SmokeCard
	Count    int
	AvgText  string
	AvgClass string
}

// StrainSmokeSummary lists a strain's smoke reports (newest first) with
// their average overall score.
func StrainSmokeSummary(db *sql.DB, strainID int) StrainSmoke {
	out := StrainSmoke{}
	list, err := LoadSmokeReports(db, LoadSmokeSettings(db).Weights, "r.strain_id = $1", strainID)
	if err != nil {
		return out
	}
	sum, n := 0.0, 0
	for _, r := range list {
		c := SmokeCard{ID: r.ID, Title: r.PlantName, Date: r.SmokedOn}
		if c.Title == "" {
			c.Title = r.Dispensary
		}
		if c.Title == "" {
			c.Title = r.StrainName
		}
		if len(r.Photos) > 0 {
			c.Photo = r.Photos[0].URL
		}
		c.OverallText, c.OverallClass = smokeScoreText(r.Overall)
		if r.Overall != nil {
			sum += *r.Overall
			n++
		}
		out.Cards = append(out.Cards, c)
	}
	out.Count = len(list)
	if n > 0 {
		avg := math.Round(sum/float64(n)*10) / 10
		out.AvgText, out.AvgClass = smokeScoreText(&avg)
	}
	return out
}

// ---------------------------------------------------------------------------
// Draft photos: a picture from a web address is downloaded as soon as it's
// added on the form, so it can be shown before the report is saved (the
// page may only show images from BudMinder itself). Saving moves it in.
// ---------------------------------------------------------------------------

func smokeDraftDir(uploadDir string) string {
	return filepath.Join(smokeUploadDir(uploadDir), "drafts")
}

var smokeDraftToken = regexp.MustCompile(`^[0-9a-f]{32}$`)

// cleanupSmokeDrafts removes drafts older than a day (forms left unsaved).
func cleanupSmokeDrafts(uploadDir string) {
	entries, err := os.ReadDir(smokeDraftDir(uploadDir))
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.Remove(filepath.Join(smokeDraftDir(uploadDir), e.Name()))
		}
	}
}

// SmokeDraftPhotoHandler: POST /smoke/draft-photo {url} → {token, url}.
func SmokeDraftPhotoHandler(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.URL) > 2048 {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	body, err := fetchImageURL(ctx, publicOnlyClient, req.URL, maxSmokePhotoSize)
	if err != nil {
		if errors.Is(err, errSmokeTooBig) || errors.Is(err, errSmokeNotImage) {
			smokePhotoError(c, err)
			return
		}
		logger.Log.WithError(err).Warn("Couldn't download smoke report photo")
		apiBadRequest(c, "api_smoke_photo_download_failed")
		return
	}
	ext, err := validateImageBytes(body)
	if err != nil {
		smokePhotoError(c, errSmokeNotImage)
		return
	}
	uploadDir := UploadDirFromContext(c)
	cleanupSmokeDrafts(uploadDir)
	if err := os.MkdirAll(smokeDraftDir(uploadDir), 0o755); err != nil {
		apiInternalError(c, "api_failed_to_create_directory")
		return
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		apiInternalError(c, "api_failed_to_save_file")
		return
	}
	token := hex.EncodeToString(b)
	path := filepath.Join(smokeDraftDir(uploadDir), token+ext)
	if body, err = utils.StripImageMetadata(body); err != nil {
		smokePhotoError(c, errSmokeNotImage)
		return
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		apiInternalError(c, "api_failed_to_save_file")
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "url": webPath(path)})
}

// SmokePhotoFromDraftHandler: POST /smoke/:id/photos/draft {token} moves a
// downloaded draft onto the report.
func SmokePhotoFromDraftHandler(c *gin.Context) {
	id, ok := smokeID(c)
	if !ok {
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !smokeDraftToken.MatchString(req.Token) {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	uploadDir := UploadDirFromContext(c)
	matches, _ := filepath.Glob(filepath.Join(smokeDraftDir(uploadDir), req.Token+".*"))
	if len(matches) != 1 {
		apiNotFound(c, "api_image_not_found")
		return
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		apiNotFound(c, "api_image_not_found")
		return
	}
	p, err := saveSmokePhoto(DBFromContext(c), uploadDir, id, body)
	if err != nil {
		smokePhotoError(c, err)
		return
	}
	_ = os.Remove(matches[0])
	c.JSON(http.StatusOK, gin.H{"photos": []SmokePhoto{p}})
}
