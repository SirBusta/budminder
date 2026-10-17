package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchImageURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			_, _ = w.Write(make([]byte, 2048))
		case "/missing":
			http.NotFound(w, r)
		default:
			_, _ = w.Write([]byte("picture-bytes"))
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	b, err := fetchImageURL(ctx, srv.Client(), srv.URL+"/pic.jpg", 1024)
	require.NoError(t, err)
	assert.Equal(t, "picture-bytes", string(b))

	_, err = fetchImageURL(ctx, srv.Client(), srv.URL+"/big", 1024)
	assert.ErrorIs(t, err, errSmokeTooBig)
	_, err = fetchImageURL(ctx, srv.Client(), srv.URL+"/missing", 1024)
	assert.Error(t, err)
	_, err = fetchImageURL(ctx, srv.Client(), "ftp://example.com/x.jpg", 1024)
	assert.ErrorIs(t, err, errSmokeNotImage, "only http(s) addresses")

	// The app's client won't reach addresses on this computer or the home network.
	_, err = fetchImageURL(ctx, publicOnlyClient, srv.URL+"/pic.jpg", 1024)
	assert.Error(t, err)
}
