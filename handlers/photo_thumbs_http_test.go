package handlers_test

// Small photo copies (handlers/photo_thumbs.go): what's served at /thumbs, and
// that camera snapshots and other files stay out of it.

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

const thumbPassword = "thumb-pass"

// bigJPEG is a noisy photo, so it can't compress below the small-file limit.
func bigJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
		}
	}
	var out bytes.Buffer
	require.NoError(t, jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}))
	return out.Bytes()
}

func thumbServer(t *testing.T) (*testutil.Client, string, string) {
	t.Helper()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, thumbPassword)
	uploads, data := t.TempDir(), t.TempDir()
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(uploads), testutil.WithDataDir(data))
	c, _ := server.LoginAndFetchCSRF(t, thumbPassword, "/")
	return c, uploads, data
}

func saveUpload(t *testing.T, uploads, rel string, body []byte) {
	t.Helper()
	file := filepath.Join(uploads, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, body, 0o644))
}

func fetchBody(t *testing.T, c *testutil.Client, path string) (int, []byte) {
	t.Helper()
	resp := c.Get(path)
	defer testutil.DrainAndClose(resp)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

func TestPhotoThumb_BigPhotoGetsASmallCopyThatIsKept(t *testing.T) {
	t.Parallel()
	c, uploads, data := thumbServer(t)
	original := bigJPEG(t, 1600, 1200)
	saveUpload(t, uploads, "plants/a.jpg", original)

	status, body := fetchBody(t, c, "/thumbs/plants/a.jpg")
	require.Equal(t, http.StatusOK, status)
	assert.Less(t, len(body), len(original)/2)
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, 640, cfg.Width)
	assert.Equal(t, 480, cfg.Height)

	kept, err := os.ReadDir(filepath.Join(data, "thumbs"))
	require.NoError(t, err)
	require.Len(t, kept, 1)

	_, again := fetchBody(t, c, "/thumbs/plants/a.jpg")
	assert.Equal(t, body, again)
	after, err := os.ReadDir(filepath.Join(data, "thumbs"))
	require.NoError(t, err)
	assert.Len(t, after, 1, "the copy is reused")
}

func TestPhotoThumb_ServesTheOriginalWhenACopyWouldNotHelp(t *testing.T) {
	t.Parallel()
	c, uploads, _ := thumbServer(t)
	cases := map[string][]byte{
		"plants/small.jpg": bigJPEG(t, 100, 100),
		"smoke/narrow.jpg": bigJPEG(t, 640, 640),
		"plants/heic.jpg":  bytes.Repeat([]byte("not a photo "), 30000),
	}
	for rel, want := range cases {
		saveUpload(t, uploads, rel, want)
		status, body := fetchBody(t, c, "/thumbs/"+rel)
		assert.Equal(t, http.StatusOK, status, rel)
		assert.Equal(t, want, body, rel)
	}
}

func TestPhotoThumb_OnlyPhotoFolders(t *testing.T) {
	t.Parallel()
	c, uploads, _ := thumbServer(t)
	saveUpload(t, uploads, "streams/cam.jpg", bigJPEG(t, 800, 600))
	saveUpload(t, uploads, "plants/a.jpg", bigJPEG(t, 800, 600))
	for _, p := range []string{
		"/thumbs/streams/cam.jpg",
		"/thumbs/plants/../streams/cam.jpg",
		"/thumbs/plants/..%5Cstreams%5Ccam.jpg",
		"/thumbs/plants/missing.jpg",
		"/thumbs/plants",
		"/thumbs/",
	} {
		status, _ := fetchBody(t, c, p)
		assert.Equal(t, http.StatusNotFound, status, p)
	}
}

func TestPhotoThumb_SignedOutNeedsGuestAccess(t *testing.T) {
	t.Parallel()
	for _, guest := range []bool{false, true} {
		db := testutil.NewTestDB(t)
		testutil.SeedAdmin(t, db, thumbPassword)
		uploads := t.TempDir()
		opts := []testutil.ServerOption{testutil.WithUploadDir(uploads), testutil.WithDataDir(t.TempDir())}
		want := http.StatusFound
		if guest {
			opts = append(opts, testutil.WithGuestMode())
			want = http.StatusOK
		}
		server := testutil.NewTestServer(t, db, opts...)
		saveUpload(t, uploads, "plants/a.jpg", bigJPEG(t, 800, 600))
		status, _ := fetchBody(t, server.NewClient(t), "/thumbs/plants/a.jpg")
		assert.Equal(t, want, status, "guest %v", guest)
	}
}

func TestPhotoThumbURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/thumbs/plants/a.jpg", handlers.PhotoThumbURL("/uploads/plants/a.jpg"))
	assert.Equal(t, "/thumbs/smoke/a.jpg", handlers.PhotoThumbURL("/uploads/smoke/a.jpg"))
	assert.Equal(t, "/uploads/strains/a.jpg", handlers.PhotoThumbURL("/uploads/strains/a.jpg"))
	assert.Equal(t, "", handlers.PhotoThumbURL(""))
}
