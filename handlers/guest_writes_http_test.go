package handlers_test

// Guest view is read-only: a visitor who isn't signed in can't create files.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

func TestGuestView_CannotDecorateImages(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())
	guest := server.NewClient(t)
	token := guest.FetchMetaCSRFToken("/plants")
	require.NotEmpty(t, token)

	resp := guest.SessionPostJSON(t, "/decorateImage", token, map[string]any{"imagePath": "uploads/plants/x.jpg"})
	defer testutil.DrainAndClose(resp)
	assert.NotEqual(t, http.StatusBadRequest, resp.StatusCode, "the request must not reach the image handler")
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}
