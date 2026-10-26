package handlers_test

// Sign-in through the real engine.

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// Signing in goes back to the page that sent you to sign in; a background
// request doesn't count.
func TestLogin_ReturnsToWantedPage(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "return-pass-123")

	signIn := func(c *testutil.Client) string {
		form := url.Values{}
		form.Set("username", "admin")
		form.Set("password", "return-pass-123")
		form.Set("csrf_token", c.FetchCSRFToken("/login"))
		resp := c.PostForm("/login", form)
		resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode)
		return resp.Header.Get("Location")
	}
	open := func(c *testutil.Client, path, accept string) {
		req, _ := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
		req.Header.Set("Accept", accept)
		resp, err := c.Client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode)
	}

	browser := server.NewClient(t)
	open(browser, "/settings", "text/html,application/xhtml+xml,*/*;q=0.8")
	assert.Equal(t, "/settings", signIn(browser))

	background := server.NewClient(t)
	open(background, "/home/data", "*/*")
	assert.Equal(t, "/", signIn(background))
}
