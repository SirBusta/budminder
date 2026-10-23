package handlers_test

// The "New release" notice and window (handlers/updates.go), against a
// stand-in for GitHub's release list.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
	"isley/updates"
)

const updatesPassword = "updates-pass"

// fakeReleases serves release tag with prog as this computer's program.
func fakeReleases(t *testing.T, tag string, prog []byte) string {
	t.Helper()
	name := updates.AssetName(runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(prog)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag, "html_url": "https://github.com/SirBusta/budminder/releases/tag/" + tag,
			"body": "### Fixed\n- Something <script>alert(1)</script>",
			"assets": []map[string]any{
				{"name": name, "browser_download_url": srv.URL + "/" + name},
				{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/sums"},
			},
		})
	})
	mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(prog) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  " + name + "\n"))
	})
	return srv.URL + "/latest"
}

type updateEnv struct {
	db      *testutil.TestServer
	client  *testutil.Client
	token   string
	u       *handlers.Updates
	restart chan struct{}
}

func newUpdateEnv(t *testing.T, tag string, prog []byte) *updateEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, updatesPassword)
	checker := updates.New("1.5.3", fakeReleases(t, tag, prog), func() bool {
		v, _ := handlers.GetSetting(db, handlers.UpdateCheckDisabledSetting)
		return v == "1"
	})
	require.NoError(t, checker.Check(context.Background()))
	restart := make(chan struct{}, 1)
	exe := filepath.Join(t.TempDir(), "budminder-test")
	require.NoError(t, os.WriteFile(exe, []byte("old program"), 0o755))
	u := &handlers.Updates{Checker: checker, DataDir: t.TempDir(), Exe: exe, Restart: func() { restart <- struct{}{} }}
	server := testutil.NewTestServer(t, db, testutil.WithUpdates(u))
	c, token := server.LoginAndFetchCSRF(t, updatesPassword, "/plants")
	testutil.MustExec(t, db, `DELETE FROM settings WHERE name = $1`, handlers.UpdateCheckDisabledSetting)
	return &updateEnv{db: server, client: c, token: token, u: u, restart: restart}
}

func (e *updateEnv) page(t *testing.T, path string) string {
	t.Helper()
	resp := e.client.Get(path)
	defer testutil.DrainAndClose(resp)
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestUpdates_NoticeOnEveryPage(t *testing.T) {
	t.Parallel()
	e := newUpdateEnv(t, "v1.5.4", []byte("x"))
	for _, p := range []string{"/", "/plants", "/strains", "/settings", "/manual"} {
		assert.Contains(t, e.page(t, p), `id="bmUpdateBtn"`, "New release on %s", p)
	}
	// Settings → Check for new versions off: the notice goes.
	resp := e.client.SessionPostJSON(t, "/settings", e.token, map[string]any{"disable_update_check": true, "polling_interval": "60"})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, e.page(t, "/plants"), `id="bmUpdateBtn"`)
	assert.Contains(t, e.page(t, "/settings"), `id="updateCheck" data-autosave >`, "the switch shows it's off")
}

func TestUpdates_NoNoticeWhenUpToDate(t *testing.T) {
	t.Parallel()
	e := newUpdateEnv(t, "v1.5.3", []byte("x"))
	assert.NotContains(t, e.page(t, "/plants"), `id="bmUpdateBtn"`)
	assert.Contains(t, e.page(t, "/settings"), `id="updateCheck" data-autosave checked`, "on by default")
}

func TestUpdates_StatusAndInstall(t *testing.T) {
	t.Parallel()
	e := newUpdateEnv(t, "v1.5.4", []byte("new program"))

	resp := e.client.Get("/updates/status")
	var st struct {
		Available  bool   `json:"available"`
		Current    string `json:"current"`
		Latest     string `json:"latest"`
		NotesHTML  string `json:"notes_html"`
		CanInstall bool   `json:"can_install"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&st))
	testutil.DrainAndClose(resp)
	assert.True(t, st.Available)
	assert.Equal(t, "1.5.3", st.Current)
	assert.Equal(t, "1.5.4", st.Latest)
	assert.Contains(t, st.NotesHTML, "<li>Something")
	assert.NotContains(t, st.NotesHTML, "<script>", "release notes are sanitized")
	assert.True(t, st.CanInstall)

	resp = e.client.SessionPostJSON(t, "/updates/install", e.token, map[string]any{})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, _ := os.ReadFile(e.u.Exe)
	assert.Equal(t, "new program", string(got))
	old, _ := os.ReadFile(e.u.Exe + ".old")
	assert.Equal(t, "old program", string(old), "the old program is kept as a fallback")
	backups, _ := filepath.Glob(filepath.Join(e.u.DataDir, "backups", "before-1.5.4-*.db"))
	assert.Len(t, backups, 1, "the database was backed up first")
	select {
	case <-e.restart:
	case <-time.After(5 * time.Second):
		t.Fatal("BudMinder wasn't asked to restart")
	}
}

func TestUpdates_InstallNeedsSignIn(t *testing.T) {
	t.Parallel()
	e := newUpdateEnv(t, "v1.5.4", []byte("new program"))
	anon := e.db.NewClient(t)
	resp := anon.SessionPostJSON(t, "/updates/install", anon.FetchCSRFToken("/login"), map[string]any{})
	testutil.DrainAndClose(resp)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	got, _ := os.ReadFile(e.u.Exe)
	assert.Equal(t, "old program", string(got))
}
