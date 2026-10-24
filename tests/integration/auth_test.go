package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// TestAuth_Logout verifies the full session-clear contract: after
// /logout the session cookie no longer authenticates against /, which
// goes back to redirecting to /login.
func TestAuth_Logout(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "logout-test-pw")

	c := server.LoginAsAdmin(t, "logout-test-pw")

	// Sanity: dashboard is reachable while logged in.
	resp := c.Get("/")
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Logout itself redirects to /login.
	logoutResp := c.Get("/logout")
	logoutResp.Body.Close()
	assert.Equal(t, http.StatusFound, logoutResp.StatusCode)
	assert.Equal(t, "/login", logoutResp.Header.Get("Location"))

	// Subsequent dashboard request must redirect — session was cleared.
	postResp := c.Get("/")
	postResp.Body.Close()
	assert.Equal(t, http.StatusFound, postResp.StatusCode)
	assert.Equal(t, "/login", postResp.Header.Get("Location"))
}

// TestAuth_CSRFRejectsPostWithoutToken verifies that a POST without a
// csrf_token field is rejected with 403, even on an unauthenticated
// route like /login. The session-bound CSRF token must be submitted
// with every POST/PUT/DELETE.
func TestAuth_CSRFRejectsPostWithoutToken(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "irrelevant")

	c := server.NewClient(t)

	// Prime the session by GETting /login first so the server-side
	// session has a CSRF token assigned. We deliberately do NOT submit
	// it back — that's the assertion.
	primer := c.Get("/login")
	primer.Body.Close()

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "irrelevant")
	// no csrf_token
	resp := c.PostForm("/login", form)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "POST without csrf_token must be 403")
}

// TestAuth_LoginRateLimiter verifies that after MaxLoginAttempts failed
// POSTs from one IP, the next attempt returns 429 rather than 401.
//
// The limiter's sliding window is widened to 10 minutes here so the
// test does not race the wallclock under heavy CI load. Production
// uses 1 minute (NewRateLimiterService(nil, nil) default), but a
// parallel test running under -race against bcrypt validation can
// take 60+ seconds to issue six POSTs on a contended runner —
// observed at 91s in CI on 2026-04-28 — and the early attempts
// would age out of a 1-minute window before the +1 attempt fires.
// The behavior under test (count attempts, trip at limit+1) is
// identical at any window > test runtime; only the wallclock
// dependency goes away.
func TestAuth_LoginRateLimiter(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	rls := handlers.NewRateLimiterService(
		nil,
		handlers.NewLoginRateLimiter(handlers.MaxLoginAttempts, 10*time.Minute),
	)
	server := testutil.NewTestServer(t, db, testutil.WithRateLimiterService(rls))
	testutil.SeedAdmin(t, db, "the-real-password")

	c := server.NewClient(t)
	// Get a CSRF token once; the cookie jar carries the session across
	// POSTs so the same token applies.
	token := c.FetchCSRFToken("/login")

	// MaxLoginAttempts wrong-password POSTs should each return 401 —
	// CSRF passes, auth fails.
	for i := 0; i < handlers.MaxLoginAttempts; i++ {
		resp := c.PostForm("/login", loginForm("admin", "wrong", token))
		resp.Body.Close()
		require.Equalf(t, http.StatusUnauthorized, resp.StatusCode, "POST %d/%d should be 401", i+1, handlers.MaxLoginAttempts)
	}

	// The next POST trips the limiter.
	limited := c.PostForm("/login", loginForm("admin", "wrong", token))
	limited.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, limited.StatusCode, "MaxLoginAttempts+1 should be 429")
}

// TestAuth_SessionVersionInvalidation verifies that bumping the
// session_version setting forces all existing sessions to re-login.
// This is the mechanism HandleChangePassword uses to revoke other
// devices when a user rotates their password.
func TestAuth_SessionVersionInvalidation(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "session-pw")

	c := server.LoginAsAdmin(t, "session-pw")

	// Sanity: still logged in.
	ok := c.Get("/")
	ok.Body.Close()
	require.Equal(t, http.StatusOK, ok.StatusCode)

	// Bump session_version on the server side. The session cookie still
	// carries the OLD version; AuthMiddleware should detect the mismatch
	// and clear the session.
	testutil.UpsertSetting(t, db, "session_version", "v2-after-rotation")

	resp := c.Get("/")
	resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

// TestAuth_SetupFlow: a new install sends every page to /setup; the first
// person to submit it chooses the username and password and is signed in;
// after that /setup can't be used again.
func TestAuth_SetupFlow(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	c := server.NewClient(t)
	token := c.FetchCSRFToken("/setup")

	// Other requests while the page is open (the browser's background
	// requests, another tab) mustn't void the form's CSRF token.
	for _, path := range []string{"/", "/login", "/plants"} {
		resp := c.Get(path)
		resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode, path)
		assert.Equal(t, "/setup", resp.Header.Get("Location"), path)
	}

	form := url.Values{}
	form.Set("username", "  grower  ")
	form.Set("password", "first-pw-123")
	form.Set("confirm_password", "first-pw-123")
	form.Set("csrf_token", token)
	resp := c.PostForm("/setup", form)
	resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/", resp.Header.Get("Location"))
	dash := c.Get("/")
	dash.Body.Close()
	assert.Equal(t, http.StatusOK, dash.StatusCode, "signed in straight away")

	user, _ := handlers.GetSetting(db, "auth_username")
	assert.Equal(t, "grower", user, "trimmed")

	// Nobody can set it up a second time.
	other := server.NewClient(t)
	again := other.Get("/setup")
	again.Body.Close()
	assert.Equal(t, "/login", again.Header.Get("Location"))
	form.Set("username", "intruder")
	form.Set("csrf_token", other.FetchCSRFToken("/login"))
	take := other.PostForm("/setup", form)
	take.Body.Close()
	assert.Equal(t, "/login", take.Header.Get("Location"))
	user, _ = handlers.GetSetting(db, "auth_username")
	assert.Equal(t, "grower", user)

	// And the chosen sign-in works from a new browser.
	c3 := server.NewClient(t)
	login := c3.PostForm("/login", loginForm("grower", "first-pw-123", c3.FetchCSRFToken("/login")))
	login.Body.Close()
	assert.Equal(t, "/", login.Header.Get("Location"))
}

// TestAuth_SetupValidation: a blank username, mismatched or short
// passwords are refused and nothing is saved.
func TestAuth_SetupValidation(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	c := server.NewClient(t)

	for _, tc := range []struct{ name, user, pw, confirm string }{
		{"blank username", "   ", "abcdefgh", "abcdefgh"},
		{"mismatch", "grower", "abcdefgh", "abcdefgX"},
		{"too short", "grower", "1234567", "1234567"},
	} {
		form := url.Values{}
		form.Set("username", tc.user)
		form.Set("password", tc.pw)
		form.Set("confirm_password", tc.confirm)
		form.Set("csrf_token", c.FetchCSRFToken("/setup"))
		resp := c.PostForm("/setup", form)
		resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, tc.name)
	}
	assert.True(t, handlers.NeedsSetup(db), "nothing was saved")
}

// TestAuth_AccountChange: Settings > Account needs the current password,
// can rename the account and change the password, and a new password signs
// out other browsers.
func TestAuth_AccountChange(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "start-pw-123")
	c := server.LoginAsAdmin(t, "start-pw-123")
	elsewhere := server.LoginAsAdmin(t, "start-pw-123")

	post := func(user, current, pw, confirm string) *http.Response {
		form := url.Values{}
		form.Set("username", user)
		form.Set("current_password", current)
		form.Set("new_password", pw)
		form.Set("confirm_password", confirm)
		form.Set("csrf_token", c.FetchCSRFToken("/change-password"))
		resp := c.PostForm("/change-password", form)
		resp.Body.Close()
		return resp
	}

	assert.Equal(t, http.StatusBadRequest, post("grower", "wrong-pw", "", "").StatusCode, "the current password is required")
	assert.Equal(t, http.StatusBadRequest, post("grower", "start-pw-123", "abcdefgh", "abcdefgX").StatusCode)
	user, _ := handlers.GetSetting(db, "auth_username")
	assert.Equal(t, "admin", user, "nothing saved yet")

	// Rename only: the password stays and other browsers stay signed in.
	resp := post("grower", "start-pw-123", "", "")
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/settings", resp.Header.Get("Location"))
	still := elsewhere.Get("/")
	still.Body.Close()
	assert.Equal(t, http.StatusOK, still.StatusCode)

	// New password: this browser stays in, the other one is signed out.
	require.Equal(t, http.StatusFound, post("grower", "start-pw-123", "new-pw-4567", "new-pw-4567").StatusCode)
	mine := c.Get("/")
	mine.Body.Close()
	assert.Equal(t, http.StatusOK, mine.StatusCode)
	gone := elsewhere.Get("/")
	gone.Body.Close()
	assert.Equal(t, "/login", gone.Header.Get("Location"))

	c2 := server.NewClient(t)
	login := c2.PostForm("/login", loginForm("grower", "new-pw-4567", c2.FetchCSRFToken("/login")))
	login.Body.Close()
	assert.Equal(t, "/", login.Header.Get("Location"))
}

// TestAuth_APIKey_HappyPath verifies that an X-API-KEY header satisfies
// AuthMiddlewareApi on /api/overlay without any session cookie.
func TestAuth_APIKey_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const plaintextKey = "test-api-key-happy"
	seedHashedAPIKey(t, db, handlers.HashAPIKey(plaintextKey))

	c := server.NewClient(t)
	resp := c.APIGet(t, "/api/overlay", plaintextKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Body should be a JSON object with the documented keys.
	var payload map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.Contains(t, payload, "plants")
	assert.Contains(t, payload, "sensors")
}

// TestAuth_APIKey_RejectsBadKey verifies a wrong key produces 401.
func TestAuth_APIKey_RejectsBadKey(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	seedHashedAPIKey(t, db, handlers.HashAPIKey("the-real-key"))

	c := server.NewClient(t)
	resp := c.APIGet(t, "/api/overlay", "not-the-real-key")
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestAuth_APIKey_RejectsNoCredentials verifies that a request with
// neither an X-API-KEY header nor a session cookie returns 401.
func TestAuth_APIKey_RejectsNoCredentials(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	seedHashedAPIKey(t, db, handlers.HashAPIKey("the-real-key"))

	c := server.NewClient(t)
	resp := c.Get("/api/overlay")
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// seedHashedAPIKey writes an already-hashed api_key into settings. Used
// by the auth tests that exercise plaintext/SHA-256/bcrypt branches and
// need full control over what is stored.
func seedHashedAPIKey(t *testing.T, db *sql.DB, hashed string) {
	t.Helper()
	testutil.UpsertSetting(t, db, "api_key", hashed)
}
