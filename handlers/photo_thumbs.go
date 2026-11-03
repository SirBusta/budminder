package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/utils"
)

// Small copies of plant and smoke report photos, for lists, grids and strips
// (the thumb template func, bmThumb in web/static/js/photo-thumb.js). A copy is
// made the first time it's asked for and kept in <data>/thumbs, named after the
// photo's path, size and time, so a compressed or restored photo gets a new one.
// The folder isn't in backups: everything in it can be made again.

const (
	photoThumbSize    = 640                 // longest side, in pixels
	photoThumbMinFile = 200 * 1024          // smaller files are served as they are
	photoThumbMaxArea = 100 * 1000 * 1000   // bigger photos aren't decoded here
	photoThumbMaxAge  = 90 * 24 * time.Hour // unused copies go; used ones are made again
	photoThumbWorkers = 2                   // decoding a phone photo takes ~50 MB
)

var photoThumbFolders = []string{"plants", "smoke"}

// PhotoThumbURL returns the small copy's address for a photo's address, or the
// address as it was when the photo has no small copy.
func PhotoThumbURL(u string) string {
	for _, folder := range photoThumbFolders {
		if strings.HasPrefix(u, "/uploads/"+folder+"/") {
			return "/thumbs" + strings.TrimPrefix(u, "/uploads")
		}
	}
	return u
}

// PhotoThumbHandler: GET /thumbs/plants/<file> → the small copy, or the photo
// itself when a copy can't be made or wouldn't help. Signed-out visitors need
// guest access. Only the photo folders count: camera snapshots stay private,
// and a backslash could step out of a folder on Windows.
func PhotoThumbHandler(c *gin.Context) {
	rel := strings.TrimPrefix(path.Clean("/"+c.Param("filepath")), "/")
	if strings.Contains(rel, `\`) {
		c.Status(http.StatusNotFound)
		return
	}
	folder, _, _ := strings.Cut(rel, "/")
	allowed := false
	for _, f := range photoThumbFolders {
		allowed = allowed || f == folder
	}
	if !allowed || !filepath.IsLocal(filepath.FromSlash(rel)) {
		c.Status(http.StatusNotFound)
		return
	}
	s := PhotoCompressFromContext(c)
	file := filepath.Join(s.uploadDir, filepath.FromSlash(rel))
	info, err := os.Stat(file)
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}
	if thumb, ok := s.photoThumb(c.Request.Context(), rel, file, info); ok {
		file = thumb
	}
	f, err := os.Open(file)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()
	served, err := f.Stat()
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	http.ServeContent(c.Writer, c.Request, file, served.ModTime(), f)
}

// photoThumb returns the path of the photo's small copy, making it if needed,
// or false when the photo should be served as it is.
func (s *PhotoCompressService) photoThumb(ctx context.Context, rel, file string, info os.FileInfo) (string, bool) {
	if info.Size() < photoThumbMinFile {
		return "", false
	}
	sum := sha256.Sum256([]byte(rel))
	thumb := filepath.Join(s.dataDir, "thumbs", fmt.Sprintf("%s-%x-%x.jpg", hex.EncodeToString(sum[:12]), info.ModTime().UnixNano(), info.Size()))
	if _, err := os.Stat(thumb); err == nil {
		return thumb, true
	}
	if !photoNeedsThumb(file) {
		return "", false
	}
	select {
	case s.thumbSlots <- struct{}{}:
		defer func() { <-s.thumbSlots }()
	case <-ctx.Done():
		return "", false
	}
	if _, err := os.Stat(thumb); err == nil { // made while this one waited
		return thumb, true
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return "", false
	}
	small, err := utils.PhotoThumbnail(body, photoThumbSize)
	if err != nil || len(small) >= len(body) {
		return "", false
	}
	if err := os.MkdirAll(filepath.Dir(thumb), 0o755); err != nil {
		return "", false
	}
	tmp, err := os.CreateTemp(filepath.Dir(thumb), "thumb-*.tmp")
	if err != nil {
		return "", false
	}
	_, werr := tmp.Write(small)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), thumb) != nil {
		_ = os.Remove(tmp.Name())
		return "", false
	}
	return thumb, true
}

// photoNeedsThumb reads only the photo's header: is it bigger than a small copy
// and small enough to decode?
func photoNeedsThumb(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return false
	}
	return max(cfg.Width, cfg.Height) > photoThumbSize && int64(cfg.Width)*int64(cfg.Height) <= photoThumbMaxArea
}

// CleanupPhotoThumbs deletes small copies older than photoThumbMaxAge, which
// also clears the ones whose photos were deleted or changed.
func (s *PhotoCompressService) CleanupPhotoThumbs() {
	dir := filepath.Join(s.dataDir, "thumbs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := s.now().Add(-photoThumbMaxAge)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
