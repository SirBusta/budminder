package handlers_test

// Settings > Photos: with Compression Options on, a new JPEG plant photo is
// saved compressed (when that saves at least 10%) and its row records the
// quality and result; with Keep originals it's stored as uploaded.

import (
	"bytes"
	"database/sql"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/tests/testutil"
	"isley/utils"
)

type uploadedPhoto struct {
	path    string
	size    int
	quality sql.NullInt64
	result  sql.NullString
}

func uploadPlantPhoto(t *testing.T, store *config.Store, name string, body []byte) uploadedPhoto {
	t.Helper()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(t.TempDir()), testutil.WithConfigStore(store))
	const apiKey = "photo-compress-key"
	testutil.SeedAPIKey(t, db, apiKey)
	plant := testutil.SeedPlant(t, db, "P", testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"), testutil.SeedZone(t, db, "Z"))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("images[]", name)
	require.NoError(t, err)
	_, _ = part.Write(body)
	require.NoError(t, w.Close())
	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plant/"+itoa(plant)+"/images/upload", apiKey, &buf, w.FormDataContentType()))
	require.NoError(t, err)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var p uploadedPhoto
	require.NoError(t, db.QueryRow(`SELECT image_path, compress_quality, compress_result FROM plant_images`).Scan(&p.path, &p.quality, &p.result))
	b, err := os.ReadFile(p.path)
	require.NoError(t, err)
	p.size = len(b)
	return p
}

func compressingStore(q int) *config.Store {
	s := config.NewStore()
	s.SetAPIIngestEnabled(1)
	s.SetPhotoCompress(true)
	s.SetPhotoQuality(q)
	return s
}

func TestPlantPhotoUpload_Compressed(t *testing.T) {
	t.Parallel()
	orig := testutil.DetailedPhotoJPEG(t)
	p := uploadPlantPhoto(t, compressingStore(75), "IMG_1.jpg", orig)
	assert.Less(t, p.size, len(orig)*9/10)
	assert.Equal(t, int64(75), p.quality.Int64)
	assert.Equal(t, "compressed", p.result.String)
	stored, _ := os.ReadFile(p.path)
	assert.Contains(t, string(stored), "Exif\x00\x00MM", "orientation kept")
}

func TestPlantPhotoUpload_KeepOriginals(t *testing.T) {
	t.Parallel()
	orig := testutil.DetailedPhotoJPEG(t)
	store := config.NewStore()
	store.SetAPIIngestEnabled(1)
	p := uploadPlantPhoto(t, store, "IMG_1.jpg", orig)
	stripped, err := utils.StripImageMetadata(orig)
	require.NoError(t, err)
	assert.Equal(t, len(stripped), p.size, "stored as uploaded (metadata stripped only)")
	assert.False(t, p.quality.Valid)
	assert.False(t, p.result.Valid)
}

func TestPlantPhotoUpload_SmallSavingKept(t *testing.T) {
	t.Parallel()
	// Saved at 50 already: compressing at 90 doesn't save 10%.
	low, err := utils.CompressJPEG(testutil.DetailedPhotoJPEG(t), 50)
	require.NoError(t, err)
	p := uploadPlantPhoto(t, compressingStore(90), "IMG_1.jpg", low)
	assert.Equal(t, len(low), p.size)
	assert.Equal(t, "kept", p.result.String)
	assert.Equal(t, int64(90), p.quality.Int64)
}

func TestPlantPhotoUpload_PNGNotCompressed(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))))
	p := uploadPlantPhoto(t, compressingStore(75), "IMG_1.png", b.Bytes())
	assert.False(t, p.result.Valid, "only JPEGs are compressed")
}
