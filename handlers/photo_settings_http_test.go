package handlers_test

// Settings > Photos routes and the compare page, through the real engine
// with a signed-in session (handlers/photo_settings_http.go).

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"image/jpeg"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type photoRoutes struct {
	server *testutil.TestServer
	c      *testutil.Client
	csrf   string
	plant  int
}

func newPhotoRoutes(t *testing.T) *photoRoutes {
	t.Helper()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, "photos-pass-123")
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(t.TempDir()), testutil.WithDataDir(t.TempDir()))
	c, csrf := server.LoginAndFetchCSRF(t, "photos-pass-123", "/settings")
	plant := testutil.SeedPlant(t, db, "Grape OG #1", testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"), testutil.SeedZone(t, db, "Z"))
	return &photoRoutes{server: server, c: c, csrf: csrf, plant: plant}
}

func (r *photoRoutes) addPhoto(t *testing.T, name string) int {
	t.Helper()
	return testutil.SeedPlantImage(t, r.server.DB, r.plant, filepath.Join(r.server.UploadDir, "plants", name), testutil.DetailedPhotoJPEG(t))
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer testutil.DrainAndClose(resp)
	out := map[string]any{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func TestPhotoSettingsHTTP_Save(t *testing.T) {
	t.Parallel()
	r := newPhotoRoutes(t)

	resp := r.c.SessionPostJSON(t, "/settings/photos", r.csrf, map[string]any{"compress": true, "quality": 70})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	assert.True(t, r.server.ConfigStore.PhotoCompress())
	assert.Equal(t, 70, r.server.ConfigStore.PhotoQuality())
	var saved string
	require.NoError(t, r.server.DB.QueryRow(`SELECT value FROM settings WHERE name = 'photo_quality'`).Scan(&saved))
	assert.Equal(t, "70", saved)

	for _, bad := range []any{47, 95, 100, 0} {
		resp = r.c.SessionPostJSON(t, "/settings/photos", r.csrf, map[string]any{"quality": bad})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "quality %v", bad)
		testutil.DrainAndClose(resp)
	}
	resp = r.c.SessionPostJSON(t, "/settings/photos", r.csrf, map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)

	// The Settings page shows the Photos section with the saved choice.
	page := r.c.Get("/settings")
	body, _ := io.ReadAll(page.Body)
	testutil.DrainAndClose(page)
	assert.Contains(t, string(body), `id="pcCard" data-quality="70" data-compress="true"`)
}

func TestPhotoSettingsHTTP_RunAndLock(t *testing.T) {
	t.Parallel()
	r := newPhotoRoutes(t)
	r.addPhoto(t, "a.jpg")
	r.addPhoto(t, "b.jpg")

	// Keep originals: no run.
	resp := r.c.SessionPostJSON(t, "/settings/photos/existing/start", r.csrf, map[string]any{"backup": true})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)

	resp = r.c.SessionPostJSON(t, "/settings/photos", r.csrf, map[string]any{"compress": true, "quality": 75})
	testutil.DrainAndClose(resp)

	est := decodeJSON(t, r.c.Get("/settings/photos/existing?quality=75"))
	assert.Equal(t, float64(2), est["count"])
	assert.Positive(t, est["estimated_saving"])

	resp = r.c.SessionPostJSON(t, "/settings/photos/existing/start", r.csrf, map[string]any{"backup": true})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	r.server.PhotoCompress.Wait()

	st := decodeJSON(t, r.c.Get("/settings/photos/existing/status"))
	assert.Equal(t, "finished", st["state"])
	assert.Equal(t, float64(2), st["compressed"])
	assert.NotEmpty(t, st["backup_path"])
	assert.Equal(t, false, st["can_open"], "Open folder: desktop programs only")

	resp = r.c.SessionPostJSON(t, "/settings/photos/existing/backup/open", r.csrf, nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	testutil.DrainAndClose(resp)

	resp = r.c.SessionDeleteJSON(t, "/settings/photos/existing/backup", r.csrf, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, true, decodeJSON(t, resp)["backup_gone"])

	// A paused run locks the photo settings.
	r.addPhoto(t, "c.jpg")
	resp = r.c.SessionPostJSON(t, "/settings/photos/existing/start", r.csrf, map[string]any{"backup": false})
	testutil.DrainAndClose(resp)
	resp = r.c.SessionPostJSON(t, "/settings/photos/existing/stop", r.csrf, nil)
	testutil.DrainAndClose(resp)
	if r.server.PhotoCompress.Locked() {
		resp = r.c.SessionPostJSON(t, "/settings/photos", r.csrf, map[string]any{"quality": 60})
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		testutil.DrainAndClose(resp)
		resp = r.c.SessionPostJSON(t, "/settings/photos/existing/finish", r.csrf, nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		testutil.DrainAndClose(resp)
	}
	resp = r.c.SessionPostJSON(t, "/settings/photos/existing/resume", r.csrf, nil)
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "nothing paused")
	testutil.DrainAndClose(resp)
}

func TestPhotoSettingsHTTP_RestoreBackup(t *testing.T) {
	t.Parallel()
	r := newPhotoRoutes(t)
	id := r.addPhoto(t, "a.jpg")
	resp := r.c.SessionPostJSON(t, "/settings/photos", r.csrf, map[string]any{"compress": true, "quality": 75})
	testutil.DrainAndClose(resp)
	resp = r.c.SessionPostJSON(t, "/settings/photos/existing/start", r.csrf, map[string]any{"backup": true})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	r.server.PhotoCompress.Wait()

	st := decodeJSON(t, r.c.Get("/settings/photos/existing/status"))
	assert.Equal(t, float64(len(testutil.DetailedPhotoJPEG(t))), st["backup_bytes"])

	resp = r.c.SessionPostJSON(t, "/settings/photos/existing/backup/restore", r.csrf, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	st = decodeJSON(t, resp)
	assert.Equal(t, true, st["restored"])
	assert.Equal(t, float64(1), st["restored_n"])
	assert.Equal(t, true, st["backup_gone"])
	var result sql.NullString
	require.NoError(t, r.server.DB.QueryRow(`SELECT compress_result FROM plant_images WHERE id = $1`, id).Scan(&result))
	assert.False(t, result.Valid)

	resp = r.c.SessionPostJSON(t, "/settings/photos/existing/backup/restore", r.csrf, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "the backup is gone")
	testutil.DrainAndClose(resp)
}

func TestPhotoCompareHTTP(t *testing.T) {
	t.Parallel()
	r := newPhotoRoutes(t)
	id := r.addPhoto(t, "a.jpg")

	page := r.c.Get("/settings/photos/compare")
	assert.Equal(t, http.StatusOK, page.StatusCode)
	testutil.DrainAndClose(page)

	list := decodeJSON(t, r.c.Get("/settings/photos/compare/photos?page=0"))
	photos := list["photos"].([]any)
	require.Len(t, photos, 1)
	assert.Equal(t, "Grape OG #1", photos[0].(map[string]any)["plant"])
	assert.Equal(t, false, list["more"])

	for _, src := range []string{"sample:1", "plant:" + strconv.Itoa(id)} {
		sizes := decodeJSON(t, r.c.Get("/settings/photos/compare/sizes?src="+src))
		assert.Positive(t, sizes["original"], src)
		assert.Len(t, sizes["sizes"], 9, src)

		img := r.c.Get("/settings/photos/compare/image?src=" + src + "&q=60")
		require.Equal(t, http.StatusOK, img.StatusCode, src)
		b, _ := io.ReadAll(img.Body)
		testutil.DrainAndClose(img)
		_, err := jpeg.DecodeConfig(bytes.NewReader(b))
		assert.NoError(t, err, src)
	}
	thumb := r.c.Get("/settings/photos/compare/thumb?src=plant:" + strconv.Itoa(id))
	assert.Equal(t, http.StatusOK, thumb.StatusCode)
	testutil.DrainAndClose(thumb)

	outside := testutil.SeedPlantImage(t, r.server.DB, r.plant, filepath.Join(t.TempDir(), "x.jpg"), testutil.DetailedPhotoJPEG(t))
	for _, path := range []string{
		"/settings/photos/compare/sizes?src=sample:0",
		"/settings/photos/compare/sizes?src=sample:4",
		"/settings/photos/compare/sizes?src=plant:99999",
		"/settings/photos/compare/sizes?src=plant:" + strconv.Itoa(outside),
		"/settings/photos/compare/sizes?src=../../go.mod",
		"/settings/photos/compare/thumb?src=sample:1",
	} {
		resp := r.c.Get(path)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, path)
		testutil.DrainAndClose(resp)
	}
	resp := r.c.Get("/settings/photos/compare/image?src=sample:1&q=42")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	testutil.DrainAndClose(resp)
}

func TestPhotoSettingsHTTP_SignedOut(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, "photos-pass-123")
	server := testutil.NewTestServer(t, db)
	c := server.NewClient(t)
	for _, path := range []string{"/settings/photos/existing", "/settings/photos/compare/sizes?src=sample:1", "/settings/photos/existing/status"} {
		resp := c.Get(path)
		assert.NotEqual(t, http.StatusOK, resp.StatusCode, path)
		testutil.DrainAndClose(resp)
	}
}
