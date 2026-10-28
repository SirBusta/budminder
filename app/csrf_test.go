package app_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/app"
	"isley/tests/testutil"
)

// countingReader records how much of a request body the server read.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// TestCSRF_MultipartRefusedBeforeReadingBody: an upload without the
// X-CSRF-Token header is refused without reading (and storing) the file.
func TestCSRF_MultipartRefusedBeforeReadingBody(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	engine, err := app.NewEngine(app.Config{
		DB:            db,
		Assets:        testutil.RepoFS(t),
		Version:       "test",
		SessionSecret: []byte("isley-test-session-secret-32-by!"),
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.WriteField("csrf_token", "made-up"))
	fw, err := mw.CreateFormFile("images", "big.jpg")
	require.NoError(t, err)
	_, err = fw.Write(bytes.Repeat([]byte{0xAB}, 8<<20))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	total := buf.Len()

	body := &countingReader{r: &buf}
	req := httptest.NewRequest(http.MethodPost, "/plant/1/images/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Less(t, body.n, total/100, "read %d of %d bytes before refusing", body.n, total)
}
