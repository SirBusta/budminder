package handlers

// Routes for Settings > Photos and the compare page (handlers for
// photo_compress_service.go).

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"isley/launcher"
	"isley/logger"
	"isley/utils"
)

// PhotoSamples are the compare page's sample photos (web/static/img/
// compare-samples), so it works before any photo is added.
var PhotoSamples = []struct {
	Name, Date string // Date: YYYY-MM-DD, shown in the visitor's language
}{
	{"Grape OG #1", "2019-12-29"},
	{"Grape OG #2", "2019-11-24"},
	{"Grape OG #1", "2019-11-01"},
}

func photoSampleFile(n int) string {
	return "web/static/img/compare-samples/sample-" + strconv.Itoa(n) + ".jpg"
}

// PhotoSettingsData is what the Photos section of Settings renders with.
type PhotoSettingsData struct {
	Compress  bool
	Quality   int
	Qualities []int
	Locked    bool
	Status    PhotoJobStatus
}

// PhotoSettingsPageData builds the Settings > Photos data for a request.
func PhotoSettingsPageData(c *gin.Context) PhotoSettingsData {
	store := ConfigStoreFromContext(c)
	d := PhotoSettingsData{Compress: store.PhotoCompress(), Quality: store.PhotoQuality(), Qualities: PhotoQualities}
	if svc := PhotoCompressFromContext(c); svc != nil {
		d.Status = svc.Status()
		d.Locked = svc.Locked()
	}
	return d
}

// SavePhotoSettingsHandler: POST /settings/photos {compress, quality}.
func SavePhotoSettingsHandler(c *gin.Context) {
	var req struct {
		Compress *bool `json:"compress"`
		Quality  *int  `json:"quality"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Compress == nil && req.Quality == nil) {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	if req.Quality != nil && !ValidPhotoQuality(*req.Quality) {
		apiBadRequest(c, "photo_quality_invalid")
		return
	}
	if svc := PhotoCompressFromContext(c); svc != nil && svc.Locked() {
		apiError(c, http.StatusConflict, "photo_settings_locked")
		return
	}
	db, store := DBFromContext(c), ConfigStoreFromContext(c)
	if req.Compress != nil {
		v := "0"
		if *req.Compress {
			v = "1"
		}
		if err := UpdateSetting(db, nil, PhotoCompressSetting, v); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
		store.SetPhotoCompress(*req.Compress)
	}
	if req.Quality != nil {
		if err := UpdateSetting(db, nil, PhotoQualitySetting, strconv.Itoa(*req.Quality)); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
		store.SetPhotoQuality(*req.Quality)
	}
	c.JSON(http.StatusOK, gin.H{"compress": store.PhotoCompress(), "quality": store.PhotoQuality()})
}

func photoQualityParam(c *gin.Context) (int, bool) {
	q, err := strconv.Atoi(c.DefaultQuery("quality", strconv.Itoa(ConfigStoreFromContext(c).PhotoQuality())))
	if err != nil || !ValidPhotoQuality(q) {
		apiBadRequest(c, "photo_quality_invalid")
		return 0, false
	}
	return q, true
}

// ExistingPhotosHandler: GET /settings/photos/existing?quality=75.
func ExistingPhotosHandler(c *gin.Context) {
	q, ok := photoQualityParam(c)
	if !ok {
		return
	}
	e, err := PhotoCompressFromContext(c).Estimate(DBFromContext(c), q)
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, e)
}

// PhotoJobStatusHandler: GET /settings/photos/existing/status.
func PhotoJobStatusHandler(c *gin.Context) {
	c.JSON(http.StatusOK, PhotoCompressFromContext(c).Status())
}

func photoJobAnswer(c *gin.Context, err error) {
	svc := PhotoCompressFromContext(c)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, svc.Status())
	case errors.Is(err, ErrPhotoJobExists):
		apiError(c, http.StatusConflict, "photo_job_exists")
	case errors.Is(err, ErrPhotoJobNotPaused):
		apiError(c, http.StatusConflict, "photo_job_not_paused")
	case errors.Is(err, ErrPhotoJobRestore):
		apiError(c, http.StatusConflict, "photo_job_restore")
	case errors.Is(err, ErrPhotoNoBackup):
		apiNotFound(c, "photo_no_backup")
	default:
		logger.Log.WithError(err).Error("Photo compression request failed")
		apiInternalError(c, "photo_job_failed")
	}
}

// StartPhotoJobHandler: POST /settings/photos/existing/start {backup}.
// Uses the saved quality; Compression Options must be on.
func StartPhotoJobHandler(c *gin.Context) {
	var req struct {
		Backup bool `json:"backup"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	store := ConfigStoreFromContext(c)
	if !store.PhotoCompress() {
		apiBadRequest(c, "photo_compress_off")
		return
	}
	photoJobAnswer(c, PhotoCompressFromContext(c).Start(store.PhotoQuality(), req.Backup))
}

// StopPhotoJobHandler: POST /settings/photos/existing/stop.
func StopPhotoJobHandler(c *gin.Context) {
	PhotoCompressFromContext(c).Stop()
	photoJobAnswer(c, nil)
}

// ResumePhotoJobHandler: POST /settings/photos/existing/resume.
func ResumePhotoJobHandler(c *gin.Context) {
	photoJobAnswer(c, PhotoCompressFromContext(c).Resume())
}

// FinishPhotoJobHandler: POST /settings/photos/existing/finish.
func FinishPhotoJobHandler(c *gin.Context) {
	photoJobAnswer(c, PhotoCompressFromContext(c).Finish())
}

// DeletePhotoBackupHandler: DELETE /settings/photos/existing/backup.
func DeletePhotoBackupHandler(c *gin.Context) {
	photoJobAnswer(c, PhotoCompressFromContext(c).DeleteBackup())
}

// OpenPhotoBackupHandler: POST /settings/photos/existing/backup/open. Only
// for the downloadable programs, asked from the same computer.
func OpenPhotoBackupHandler(c *gin.Context) {
	svc := PhotoCompressFromContext(c)
	ip := net.ParseIP(c.RemoteIP())
	if !svc.Desktop || ip == nil || !ip.IsLoopback() {
		apiForbidden(c, "photo_open_unavailable")
		return
	}
	dir, err := svc.BackupDirToOpen()
	if err != nil {
		photoJobAnswer(c, err)
		return
	}
	if err := launcher.OpenFolder(dir); err != nil {
		logger.Log.WithError(err).Warn("Couldn't open the photo backup folder")
		apiInternalError(c, "photo_open_failed")
		return
	}
	apiOK(c, "photo_open_done")
}

// ---------------------------------------------------------------- compare page

// photoComparer caches what the compare page asks for: the sizes per photo
// and quality, and the encoded copies of the photo being looked at.
type photoComparer struct {
	mu     sync.Mutex
	sizes  map[string]map[int]int // src -> quality (0 = original) -> bytes
	src    string                 // the photo whose copies are kept
	copies map[int][]byte
	thumbs map[string][]byte
}

func newPhotoComparer() *photoComparer {
	return &photoComparer{sizes: map[string]map[int]int{}, thumbs: map[string][]byte{}}
}

// photoSource reads a compare page photo: "plant:<image id>" (a JPEG plant
// photo in the upload folder) or "sample:<1-3>". The key includes the
// database, so demo visitors' copies don't mix.
func photoSource(c *gin.Context, src string) (key string, body []byte, ok bool) {
	kind, arg, _ := strings.Cut(src, ":")
	n, err := strconv.Atoi(arg)
	if err != nil {
		return "", nil, false
	}
	switch kind {
	case "sample":
		if n < 1 || n > len(PhotoSamples) || AssetsFromContext(c) == nil {
			return "", nil, false
		}
		b, err := fs.ReadFile(AssetsFromContext(c), photoSampleFile(n))
		return src, b, err == nil
	case "plant":
		db := DBFromContext(c)
		var stored string
		if err := db.QueryRow(`SELECT image_path FROM plant_images WHERE id = $1`, n).Scan(&stored); err != nil {
			return "", nil, false
		}
		f := photoFile(UploadDirFromContext(c), stored)
		if f == "" {
			return "", nil, false
		}
		if st, err := os.Stat(f); err != nil || st.Size() > maxCompressPhotoSize {
			return "", nil, false
		}
		b, err := os.ReadFile(f)
		if err != nil || !utils.IsJPEG(b) {
			return "", nil, false
		}
		return src + "@" + fmt.Sprintf("%p", db) + "@" + strconv.Itoa(len(b)), b, true
	}
	return "", nil, false
}

// copiesFor returns the photo encoded at every quality, made once.
func (pc *photoComparer) copiesFor(key string, body []byte) (map[int][]byte, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.src == key && pc.copies != nil {
		return pc.copies, nil
	}
	copies := map[int][]byte{0: body}
	sizes := map[int]int{0: len(body)}
	for _, q := range PhotoQualities {
		comp, err := utils.CompressJPEG(body, q)
		if err != nil {
			return nil, err
		}
		copies[q], sizes[q] = comp, len(comp)
	}
	pc.src, pc.copies = key, copies
	if len(pc.sizes) > 500 {
		pc.sizes = map[string]map[int]int{}
	}
	pc.sizes[key] = sizes
	return copies, nil
}

// ComparePhotoSizesHandler: GET /settings/photos/compare/sizes?src=…
// → {"original": bytes, "sizes": {"90": bytes, …}}.
func ComparePhotoSizesHandler(c *gin.Context) {
	photoCompare := PhotoCompressFromContext(c).compare
	key, body, ok := photoSource(c, c.Query("src"))
	if !ok {
		apiNotFound(c, "api_image_not_found")
		return
	}
	photoCompare.mu.Lock()
	sizes, cached := photoCompare.sizes[key]
	photoCompare.mu.Unlock()
	if !cached {
		copies, err := photoCompare.copiesFor(key, body)
		if err != nil {
			apiBadRequest(c, "photo_not_compressible")
			return
		}
		sizes = map[int]int{}
		for q, b := range copies {
			sizes[q] = len(b)
		}
	}
	out := map[string]int{}
	for _, q := range PhotoQualities {
		out[strconv.Itoa(q)] = sizes[q]
	}
	c.JSON(http.StatusOK, gin.H{"original": sizes[0], "sizes": out})
}

// ComparePhotoImageHandler: GET /settings/photos/compare/image?src=…&q=75
// (q=0 is the original).
func ComparePhotoImageHandler(c *gin.Context) {
	photoCompare := PhotoCompressFromContext(c).compare
	q, err := strconv.Atoi(c.Query("q"))
	if err != nil || (q != 0 && !ValidPhotoQuality(q)) {
		apiBadRequest(c, "photo_quality_invalid")
		return
	}
	key, body, ok := photoSource(c, c.Query("src"))
	if !ok {
		apiNotFound(c, "api_image_not_found")
		return
	}
	out := body
	if q != 0 {
		copies, err := photoCompare.copiesFor(key, body)
		if err != nil {
			apiBadRequest(c, "photo_not_compressible")
			return
		}
		out = copies[q]
	}
	c.Header("Cache-Control", "private, max-age=600")
	c.Data(http.StatusOK, "image/jpeg", out)
}

// ComparePhotoThumbHandler: GET /settings/photos/compare/thumb?src=plant:ID
// → a small JPEG for the photo picker.
func ComparePhotoThumbHandler(c *gin.Context) {
	photoCompare := PhotoCompressFromContext(c).compare
	key, body, ok := photoSource(c, c.Query("src"))
	if !ok || !strings.HasPrefix(key, "plant:") {
		apiNotFound(c, "api_image_not_found")
		return
	}
	photoCompare.mu.Lock()
	t, cached := photoCompare.thumbs[key]
	photoCompare.mu.Unlock()
	if !cached {
		var err error
		if t, err = utils.ThumbnailJPEG(body, 320); err != nil {
			apiNotFound(c, "api_image_not_found")
			return
		}
		photoCompare.mu.Lock()
		if len(photoCompare.thumbs) > 300 {
			photoCompare.thumbs = map[string][]byte{}
		}
		photoCompare.thumbs[key] = t
		photoCompare.mu.Unlock()
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Data(http.StatusOK, "image/jpeg", t)
}

// ComparePhoto is one of the user's photos in the compare page's picker.
type ComparePhoto struct {
	ID    int    `json:"id"`
	Plant string `json:"plant"`
	Date  string `json:"date"`
	Bytes int64  `json:"bytes"`
}

const comparePhotosPerPage = 12

// ComparePhotosHandler: GET /settings/photos/compare/photos?page=N → the
// user's JPEG plant photos, newest first.
func ComparePhotosHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "0"))
	page = max(0, page)
	db := DBFromContext(c)
	rows, err := db.Query(`SELECT i.id, i.image_path, COALESCE(p.name, ''), i.image_date
		FROM plant_images i LEFT JOIN plant p ON p.id = i.plant_id
		WHERE LOWER(i.image_path) LIKE '%.jpg' OR LOWER(i.image_path) LIKE '%.jpeg'
		ORDER BY i.image_date DESC, i.id DESC LIMIT $1 OFFSET $2`, comparePhotosPerPage+1, page*comparePhotosPerPage)
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	defer rows.Close()
	out := []ComparePhoto{}
	uploads := UploadDirFromContext(c)
	more := false
	for rows.Next() {
		var p ComparePhoto
		var stored string
		var date time.Time
		if err := rows.Scan(&p.ID, &stored, &p.Plant, &date); err != nil {
			continue
		}
		if len(out) == comparePhotosPerPage {
			more = true
			break
		}
		f := photoFile(uploads, stored)
		if f == "" {
			continue
		}
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		p.Bytes, p.Date = st.Size(), utils.AsLocal(date).Format("2006-01-02")
		out = append(out, p)
	}
	c.JSON(http.StatusOK, gin.H{"photos": out, "more": more})
}
