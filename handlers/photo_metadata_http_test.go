package handlers_test

// Every photo BudMinder takes in is stored without its hidden metadata
// (utils.StripImageMetadata): no location, camera or comments, only the
// orientation so it still shows the right way up.

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

const photoSecret = "PHONE-MODEL-AND-GPS-37.77N"

func assertCleanPhoto(t *testing.T, path string) {
	t.Helper()
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(stored), photoSecret, "no location, camera, XMP, comment or trailing data")
	assert.Contains(t, string(stored), "Exif\x00\x00MM", "the orientation stays, in a minimal EXIF block")
}

func TestPlantPhotoUpload_StripsMetadata(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	uploads := t.TempDir()
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(uploads))
	const apiKey = "photo-meta-key"
	testutil.SeedAPIKey(t, db, apiKey)
	plant := testutil.SeedPlant(t, db, "P", testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"), testutil.SeedZone(t, db, "Z"))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("images[]", "IMG_0001.jpg")
	require.NoError(t, err)
	_, _ = part.Write(testutil.PhonePhotoJPEG(t, photoSecret))
	require.NoError(t, w.Close())

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plant/"+itoa(plant)+"/images/upload", apiKey, &buf, w.FormDataContentType()))
	require.NoError(t, err)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	saved, _ := filepath.Glob(filepath.Join(uploads, "plants", "*.jpg"))
	require.Len(t, saved, 1)
	assertCleanPhoto(t, saved[0])
}

func TestSmokePhotoUpload_StripsMetadata(t *testing.T) {
	t.Parallel()
	e := newSmokeEnv(t)
	code, out := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{"strain_name": "S"})
	require.Equal(t, http.StatusOK, code)
	id := int(out["id"].(float64))

	code, _ = e.uploadSmoke(t, id, "IMG_0002.jpg", testutil.PhonePhotoJPEG(t, photoSecret))
	require.Equal(t, http.StatusOK, code)

	var path string
	require.NoError(t, e.db.QueryRow(`SELECT path FROM smoke_report_photo WHERE report_id = $1`, id).Scan(&path))
	saved, _ := filepath.Glob(filepath.Join(e.server.UploadDir, "smoke", filepath.Base(path)))
	require.Len(t, saved, 1)
	assertCleanPhoto(t, saved[0])
}
