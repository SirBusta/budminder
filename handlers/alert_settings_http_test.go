package handlers_test

// Settings > Alerts, Email card, through the real engine with a signed-in
// session (handlers/alert_settings.go).

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

type alertRoutes struct {
	server *testutil.TestServer
	c      *testutil.Client
	csrf   string
}

func newAlertRoutes(t *testing.T) *alertRoutes {
	t.Helper()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, "alerts-pass-123")
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))
	c, csrf := server.LoginAndFetchCSRF(t, "alerts-pass-123", "/settings")
	return &alertRoutes{server: server, c: c, csrf: csrf}
}

func (r *alertRoutes) setting(t *testing.T, name string) string {
	t.Helper()
	var v string
	err := r.server.DB.QueryRow(`SELECT value FROM settings WHERE name = $1`, name).Scan(&v)
	if err != nil {
		return ""
	}
	return v
}

// password decrypts the saved mail password with the server's key.
func (r *alertRoutes) password(t *testing.T) string {
	t.Helper()
	p, err := alerts.OpenPassword(r.server.DataDir, r.setting(t, "alert_email_password"))
	require.NoError(t, err)
	return p
}

func (r *alertRoutes) post(t *testing.T, path string, body map[string]any) (int, map[string]any) {
	t.Helper()
	resp := r.c.SessionPostJSON(t, path, r.csrf, body)
	return resp.StatusCode, decodeJSON(t, resp)
}

func mailForm(f *fakes.FakeSMTP) map[string]any {
	return map[string]any{"on": true, "host": f.Host, "port": f.Port, "security": "none",
		"username": f.User, "password": f.Pass, "from": "budminder@example.com",
		"to": "grower@example.com, partner@example.com", "link": "http://192.168.1.10:8080/"}
}

func TestAlertSettingsHTTP_Save(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	form := map[string]any{"on": true, "host": "smtp.example.com", "port": 587, "security": "starttls",
		"username": "grower@example.com", "password": "app-pass-123", "from": "grower@example.com",
		"to": "grower@example.com; partner@example.com", "link": "http://192.168.1.10:8080/"}

	code, out := r.post(t, "/settings/alerts/email", form)
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, true, out["passwordSet"])
	assert.Equal(t, "1", r.setting(t, "alert_email_enabled"))
	assert.Equal(t, "smtp.example.com", r.setting(t, "alert_email_host"))
	assert.Equal(t, "app-pass-123", r.password(t))
	assert.NotContains(t, r.setting(t, "alert_email_password"), "app-pass-123", "stored encrypted")
	assert.Equal(t, "grower@example.com, partner@example.com", r.setting(t, "alert_email_to"))
	assert.Equal(t, "http://192.168.1.10:8080", r.setting(t, "alert_link_base"))

	// The page shows the settings but never the password.
	page := r.c.Get("/settings")
	body, _ := io.ReadAll(page.Body)
	testutil.DrainAndClose(page)
	assert.Contains(t, string(body), `id="alCard"`)
	assert.Contains(t, string(body), `value="smtp.example.com"`)
	assert.NotContains(t, string(body), "app-pass-123")

	// A blank password keeps the saved one; a blank username clears it.
	form["password"] = ""
	form["port"] = 465
	form["security"] = "tls"
	code, _ = r.post(t, "/settings/alerts/email", form)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "app-pass-123", r.password(t))
	assert.Equal(t, "465", r.setting(t, "alert_email_port"))

	form["username"] = ""
	code, out = r.post(t, "/settings/alerts/email", form)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, out["passwordSet"])
	assert.Equal(t, "", r.setting(t, "alert_email_password"))
}

func TestAlertSettingsHTTP_SaveRefusesIncompleteWhenOn(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)

	for name, form := range map[string]map[string]any{
		"no host":     {"on": true, "port": 587, "security": "starttls", "from": "a@example.com", "to": "a@example.com"},
		"bad to":      {"on": true, "host": "smtp.example.com", "port": 587, "security": "starttls", "from": "a@example.com", "to": "nobody"},
		"header":      {"on": true, "host": "smtp.example.com", "port": 587, "security": "starttls", "from": "a@example.com\r\nBcc: b@example.com", "to": "a@example.com"},
		"bad link":    {"on": false, "link": "javascript:alert(1)"},
		"bad payload": nil,
	} {
		code, _ := r.post(t, "/settings/alerts/email", form)
		assert.Equal(t, http.StatusBadRequest, code, name)
	}
	assert.Equal(t, "", r.setting(t, "alert_email_enabled"))

	// Off, a half-filled form is kept for later.
	code, _ := r.post(t, "/settings/alerts/email", map[string]any{"on": false, "host": "smtp.example.com"})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "0", r.setting(t, "alert_email_enabled"))
	assert.Equal(t, "smtp.example.com", r.setting(t, "alert_email_host"))
}

func TestAlertSettingsHTTP_TestEmail(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain, fakes.WithSMTPLogin("grower@example.com", "app-pass-123"))

	// Unsaved form: the test uses what's typed.
	code, out := r.post(t, "/settings/alerts/email/test", mailForm(f))
	require.Equal(t, http.StatusOK, code, out)
	got := f.Mail()
	require.Len(t, got, 1)
	assert.Equal(t, []string{"grower@example.com", "partner@example.com"}, got[0].To)
	assert.Contains(t, got[0].Data, "http://192.168.1.10:8080/settings#alerts")
	assert.Equal(t, "", r.setting(t, "alert_email_host"), "a test doesn't save")

	// Saved password, blank field: the test falls back to it.
	code, _ = r.post(t, "/settings/alerts/email", mailForm(f))
	require.Equal(t, http.StatusOK, code)
	form := mailForm(f)
	form["password"] = ""
	code, _ = r.post(t, "/settings/alerts/email/test", form)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, f.Mail(), 2)

	// The mail server's refusal is shown, without the password.
	form["password"] = "wrong-pass"
	code, out = r.post(t, "/settings/alerts/email/test", form)
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Contains(t, out["error"], "Username and Password not accepted")
	assert.NotContains(t, out["error"], "wrong-pass")
	assert.Contains(t, out["hint"], "app password", "with plain advice")

	// A saved password this data folder can't decrypt (key file gone)
	// counts as none: the page says so and the test sends no password.
	require.NoError(t, os.Remove(filepath.Join(r.server.DataDir, "budminder.key")))
	page := r.c.Get("/settings")
	body, _ := io.ReadAll(page.Body)
	testutil.DrainAndClose(page)
	assert.Contains(t, string(body), `id="alPassBadge">Not set<`)
	form["password"] = ""
	code, _ = r.post(t, "/settings/alerts/email/test", form)
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Len(t, f.Mail(), 2)

	// Incomplete settings are refused before connecting.
	form["to"] = ""
	code, _ = r.post(t, "/settings/alerts/email/test", form)
	assert.Equal(t, http.StatusBadRequest, code)
}
