package handlers_test

// Vivosun from the dashboard (handlers/vivosun.go): signing in from
// Settings, Scan Vivosun, adding a reading, the raw-data download and the
// Settings card, against a stand-in Vivosun through a real login + CSRF.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

// newVivosunEnv is a signed-in dashboard whose Vivosun is the fake. Its data
// folder is a temp dir: Stay signed in writes the encryption key there.
func newVivosunEnv(t *testing.T) (*sensorEnv, *fakes.FakeVS) {
	t.Helper()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))
	testutil.SeedAdmin(t, db, sensorManagePassword)
	c, token := server.LoginAndFetchCSRF(t, sensorManagePassword, "/settings")
	e := &sensorEnv{db: db, server: server, client: c, token: token}
	vs := fakes.FakeVivosun(t)
	e.server.ConfigStore.SetVSBaseURL(vs.URL)
	return e, vs
}

func (e *sensorEnv) postJSON(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	resp := e.client.SessionPostJSON(t, path, e.token, body)
	defer testutil.DrainAndClose(resp)
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (e *sensorEnv) vivosunSignIn(t *testing.T, stay bool) {
	t.Helper()
	code, out := e.postJSON(t, "/vivosun/login", map[string]any{
		"email": fakes.VivosunEmail, "password": fakes.VivosunPassword, "stay_signed_in": stay})
	require.Equal(t, http.StatusOK, code, out)
	require.Equal(t, true, out["success"])
}

// scanVivosun returns the found sensors by type.
func (e *sensorEnv) scanVivosun(t *testing.T) (int, map[string]map[string]any, map[string]any) {
	t.Helper()
	resp := e.client.SessionPostJSON(t, "/sensors/scanVivosun", e.token, map[string]any{})
	defer testutil.DrainAndClose(resp)
	var out struct {
		Sensors []map[string]any `json:"sensors"`
		Error   string           `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	byType := map[string]map[string]any{}
	for _, s := range out.Sensors {
		byType[s["type"].(string)] = s
	}
	return resp.StatusCode, byType, map[string]any{"error": out.Error}
}

func TestVivosun_SignIn(t *testing.T) {
	t.Parallel()
	e, vs := newVivosunEnv(t)

	code, out := e.postJSON(t, "/vivosun/login", map[string]any{"email": fakes.VivosunEmail, "password": "wrong"})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "Wrong account or password", out["error"], "Vivosun's own reason is shown")
	assert.Empty(t, e.setting(t, "vivosun.access_token"))
	assert.Empty(t, e.setting(t, "vivosun.enabled"), "a failed sign-in turns nothing on")

	code, _ = e.postJSON(t, "/vivosun/login", map[string]any{"email": " ", "password": "x"})
	assert.Equal(t, http.StatusBadRequest, code, "no email")
	code, _ = e.postJSON(t, "/vivosun/login", map[string]any{"email": "a@b.c", "password": strings.Repeat("x", 300)})
	assert.Equal(t, http.StatusBadRequest, code, "too long")
	assert.EqualValues(t, 1, vs.Logins.Load(), "bad input never reaches Vivosun")

	e.vivosunSignIn(t, true)
	assert.Equal(t, "1", e.setting(t, "vivosun.enabled"), "signing in turns reading Vivosun on")
	assert.Equal(t, fakes.VivosunAccess, e.setting(t, "vivosun.access_token"))
	assert.Equal(t, fakes.VivosunLogin, e.setting(t, "vivosun.login_token"))
	assert.Equal(t, fakes.VivosunEmail, e.setting(t, "vivosun.email"))
	sealed := e.setting(t, "vivosun.password")
	assert.NotEmpty(t, sealed)
	assert.NotContains(t, sealed, fakes.VivosunPassword, "the password is stored encrypted")
	a, l := e.server.ConfigStore.VSTokens()
	assert.Equal(t, [2]string{fakes.VivosunAccess, fakes.VivosunLogin}, [2]string{a, l})
	assert.Equal(t, 1, e.server.ConfigStore.VSEnabled())

	// The Settings card: signed in, staying signed in, nothing secret.
	resp := e.client.Get("/settings")
	page, err := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.NoError(t, err)
	s := string(page)
	assert.Contains(t, s, `id="vsBadge" class="badge bg-success`)
	assert.Contains(t, s, "Signs in again automatically if Vivosun signs it out.")
	assert.NotContains(t, s, `class="st-aci-stay d-none" id="vsStayNote"`)
	for _, secret := range []string{fakes.VivosunPassword, fakes.VivosunAccess, fakes.VivosunLogin, sealed} {
		assert.NotContains(t, s, secret)
	}

	// Signing in without Stay signed in removes the saved password.
	e.vivosunSignIn(t, false)
	assert.Empty(t, e.setting(t, "vivosun.password"))
	assert.Empty(t, e.setting(t, "vivosun.email"))
	assert.Equal(t, fakes.VivosunAccess, e.setting(t, "vivosun.access_token"), "the sign-in itself stays")

	e.vivosunSignIn(t, true)
	code, _ = e.postJSON(t, "/vivosun/forget", map[string]any{})
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, e.setting(t, "vivosun.password"))
	assert.Equal(t, fakes.VivosunAccess, e.setting(t, "vivosun.access_token"))
}

func TestVivosun_SettingsSwitchSaves(t *testing.T) {
	t.Parallel()
	e, _ := newVivosunEnv(t)
	e.vivosunSignIn(t, false)

	body := map[string]any{"polling_interval": "60", "stream_grab_interval": "30000", "sensor_retention_days": "90",
		"log_level": "info", "units": "imperial", "vivosun": map[string]any{"enabled": false}}
	code, out := e.postJSON(t, "/settings", body)
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, "0", e.setting(t, "vivosun.enabled"))
	assert.Equal(t, 0, e.server.ConfigStore.VSEnabled())

	delete(body, "vivosun")
	code, _ = e.postJSON(t, "/settings", body)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "0", e.setting(t, "vivosun.enabled"), "a save without Vivosun leaves it alone")
	assert.Equal(t, fakes.VivosunAccess, e.setting(t, "vivosun.access_token"), "the switch never signs out")
}

func TestVivosun_ScanAndAdd(t *testing.T) {
	t.Parallel()
	e, vs := newVivosunEnv(t)

	code, found, out := e.scanVivosun(t)
	assert.Equal(t, http.StatusBadRequest, code, "not signed in")
	assert.Equal(t, "Sign in to Vivosun in Settings first.", out["error"])
	assert.Empty(t, found)

	e.vivosunSignIn(t, false)
	vs.SetRows(fakes.VCureRow(time.Now().Add(-2*time.Minute).Unix()), fakes.VCureRow(time.Now().Add(-time.Minute).Unix()))
	code, found, out = e.scanVivosun(t)
	require.Equal(t, http.StatusOK, code, out)
	assert.Len(t, found, 9, "inside and room temp/humidity/VPD, electronics, coil, compressor; no Wi-Fi or unplugged probe")

	in := found["VS.pTemp"]
	require.NotNil(t, in)
	assert.Equal(t, fakes.VivosunDeviceID, in["device"])
	assert.Equal(t, "VCure C80 temp", in["name"])
	assert.Equal(t, "°C", in["unit"])
	assert.InDelta(t, 22.5, in["value"], 0.001)
	assert.Equal(t, "sm_vs_note_in", in["note"])
	assert.Equal(t, true, in["home"])
	room := found["VS.bTemp.out"]
	require.NotNil(t, room)
	assert.Equal(t, "sm_vs_note_room", room["note"])
	assert.Equal(t, false, room["home"], "room air starts off the Home page")
	assert.Equal(t, false, found["VS.coreTemp"]["home"])
	assert.Equal(t, false, found["VS.evapT"]["home"])
	comp := found["VS.cure.comp"]
	require.NotNil(t, comp)
	assert.Equal(t, "VCure C80 compressor", comp["name"])
	assert.Equal(t, "on/off", comp["unit"])
	assert.Equal(t, true, comp["more"], "equipment only with Show equipment")
	assert.Equal(t, false, comp["home"])
	assert.InDelta(t, 0, comp["value"], 0.001)
	assert.Nil(t, in["more"], "readings are always listed")
	assert.Equal(t, "VCure C80", e.server.ConfigStore.VSDeviceNames()[fakes.VivosunDeviceID], "a scan fills the device names")

	add := func(s map[string]any) int {
		t.Helper()
		code, out := e.postJSON(t, "/sensors/add", map[string]any{"source": s["source"], "device": s["device"],
			"type": s["type"], "name": s["name"], "unit": s["unit"], "new_zone": "VCure", "home": s["home"]})
		require.Equal(t, http.StatusOK, code, out)
		var id int
		require.NoError(t, e.db.QueryRow(`SELECT id FROM sensors WHERE source = 'vivosun' AND type = $1`, s["type"]).Scan(&id))
		return id
	}
	visibility := func(id int) string {
		var v string
		require.NoError(t, e.db.QueryRow(`SELECT visibility FROM sensors WHERE id = $1`, id).Scan(&v))
		return v
	}
	assert.Equal(t, "zone_plant", visibility(add(in)), "inside air shows on Home")
	assert.Equal(t, "plant", visibility(add(room)), "home:false keeps it off Home")

	_, found, _ = e.scanVivosun(t)
	assert.Len(t, found, 7, "added readings aren't offered again")
	assert.NotContains(t, found, "VS.pTemp")
}

func TestVivosun_ScanAfterTheSignInExpired(t *testing.T) {
	t.Parallel()
	e, vs := newVivosunEnv(t)
	e.vivosunSignIn(t, false)
	vs.Expire()

	code, _, out := e.scanVivosun(t)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "Sign-in expired: sign in again in Settings", out["error"])
	st := handlers.IntegrationStatuses(e.db, e.server.ConfigStore)["vivosun"]
	assert.Equal(t, "auth", st.State, "the card and the Today page say to sign in again")

	resp := e.client.Get("/settings")
	page, _ := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	assert.Contains(t, string(page), `id="vsBadge" class="badge bg-danger align-self-start">Sign-in expired`)
	assert.Contains(t, string(page), `<span id="vsSignLabel">Sign in again</span>`)
}

func TestVivosun_DumpHidesAccountDetails(t *testing.T) {
	t.Parallel()
	e, vs := newVivosunEnv(t)

	resp := e.client.Get("/sensors/dumpVivosun")
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "not signed in")

	e.vivosunSignIn(t, true)
	vs.SetRows(fakes.VCureRow(time.Now().Add(-time.Minute).Unix()))
	resp = e.client.Get("/sensors/dumpVivosun")
	raw, err := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	s := string(raw)
	assert.Contains(t, s, "VCure C80")
	assert.Contains(t, s, "pTemp")
	assert.Contains(t, s, `"row_count": 1`)
	for _, secret := range []string{fakes.VivosunEmail, "example.com", fakes.VivosunDeviceID, "u-1",
		fakes.VivosunAccess, fakes.VivosunLogin, fakes.VivosunPassword} {
		assert.NotContains(t, s, secret)
	}
}
