package handlers_test

// The BudMinder name and logo: page title, header wordmark, the credit
// to Isley (Help, Welcome → About), the icons, the web app manifest, and the first-run login.

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func TestBranding_Pages(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)

	body := e.page(t, "/plants")
	assert.Contains(t, body, "<title>BudMinder ")
	assert.Contains(t, body, `src="/static/img/brand/budminder-wordmark.svg" alt="BudMinder"`, "header wordmark")
	assert.Contains(t, body, `src="/static/img/brand/budminder-wordmark-dark.svg"`, "and its dark version")
	assert.Contains(t, body, `<link rel="manifest" href="/static/manifest.json">`)
	assert.Contains(t, body, `href="/static/img/brand/apple-touch-icon.png"`)
	assert.NotContains(t, body, "<footer", "no footer on every page: the credits are in Help")
	assert.NotContains(t, body, "isley_logo.png", "old logo gone")

	// Help, Welcome → About BudMinder: version, credit to Isley and the license.
	welcome := e.page(t, "/manual")
	assert.Contains(t, welcome, `id="mnAbout"`)
	assert.Contains(t, welcome, `href="https://github.com/dwot/isley"`, "credits Isley")
	assert.Contains(t, welcome, "Based on")
	assert.Contains(t, welcome, "Permission is hereby granted", "carries the MIT license text")
	assert.Contains(t, welcome, "Copyright (c) 2024 dwot")
	assert.NotContains(t, e.page(t, "/manual/plant-page"), `id="mnAbout"`, "only on Welcome")

	fresh := testutil.NewTestServer(t, testutil.NewTestDB(t)).NewClient(t)
	resp := fresh.Get("/setup")
	setup, err := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.NoError(t, err)
	assert.Contains(t, string(setup), "brand-wm-lg", "the setup page shows the wordmark")

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, "brand-pw-123")
	resp = testutil.NewTestServer(t, db).NewClient(t).Get("/login")
	login, err := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.NoError(t, err)
	assert.Contains(t, string(login), "brand-wm-lg", "login page shows the wordmark")
}

func TestBranding_IconsAndManifest(t *testing.T) {
	t.Parallel()
	c := testutil.NewTestServer(t, testutil.NewTestDB(t)).NewClient(t)

	resp := c.Get("/favicon.ico")
	ico, err := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.GreaterOrEqual(t, len(ico), 6)
	assert.Equal(t, []byte{0, 0, 1, 0}, ico[:4], "an .ico file")
	assert.Equal(t, 3, int(ico[4]), "with 16, 32 and 48 px images")

	resp = c.Get("/static/manifest.json")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var m struct {
		Name  string `json:"name"`
		Icons []struct {
			Src     string `json:"src"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "BudMinder", m.Name)
	require.Len(t, m.Icons, 3)
	for _, ic := range m.Icons {
		r := c.Get(ic.Src)
		testutil.DrainAndClose(r)
		assert.Equal(t, http.StatusOK, r.StatusCode, ic.Src)
	}
	assert.Equal(t, "maskable", m.Icons[2].Purpose)
}

func TestNeedsSetup(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	assert.True(t, handlers.NeedsSetup(db), "a new install has no account")

	testutil.SeedAdminWithForceChange(t, db, "budminder")
	assert.True(t, handlers.NeedsSetup(db), "the old default sign-in still needs setting up")

	testutil.SeedAdmin(t, db, "a-real-password")
	assert.False(t, handlers.NeedsSetup(db))
}
