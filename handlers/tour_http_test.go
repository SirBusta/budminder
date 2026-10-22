package handlers_test

// The first-run tour (handlers/tour.go): who gets asked about it, that the
// answer sticks, and that the demo never saves it.

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

const tourPassword = "tour-pass"

type tourPage struct {
	Tour struct {
		T       map[string]string `json:"t"`
		Ask     bool              `json:"ask"`
		Askable bool              `json:"askable"`
	} `json:"tour"`
	Demo bool `json:"demo"`
}

var tourDataRe = regexp.MustCompile(`(?s)<script type="application/json" id="tourData">(.*?)</script>`)

// readTour fetches a page and returns the tour data it hands to tour.js.
func readTour(t *testing.T, c *testutil.Client, path string) tourPage {
	t.Helper()
	resp := c.Get(path)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, path)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	m := tourDataRe.FindSubmatch(body)
	require.NotNil(t, m, "no tour data on %s", path)
	var out tourPage
	require.NoError(t, json.Unmarshal(m[1], &out), string(m[1]))
	return out
}

func TestTour_AskedOnceOnANewInstall(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, tourPassword)
	server := testutil.NewTestServer(t, db)
	c, token := server.LoginAndFetchCSRF(t, tourPassword, "/")

	first := readTour(t, c, "/")
	assert.True(t, first.Tour.Ask, "a new install is asked")
	assert.False(t, first.Demo)
	assert.Equal(t, "Would you like a quick tour?", first.Tour.T["tour_ask_title"])
	assert.NotEmpty(t, first.Tour.T["tour_4_3_title"], "all the tour's text comes along")

	resp := c.SessionPostJSON(t, "/tour/answered", token, map[string]any{})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.False(t, readTour(t, c, "/").Tour.Ask, "asked only once")
	answered, err := handlers.GetSetting(db, "tour.answered")
	require.NoError(t, err)
	assert.Equal(t, "1", answered)
}

func TestTour_NotAskedOnceThereAreStrains(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, tourPassword)
	testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "Seed Junky"), "Permanent Marker")
	server := testutil.NewTestServer(t, db)
	c := server.LoginAsAdmin(t, tourPassword)

	assert.False(t, readTour(t, c, "/").Tour.Ask, "an install already in use isn't asked")
}

func TestTour_HelpOffersTheTourButNeverAsks(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, tourPassword)
	server := testutil.NewTestServer(t, db)
	c := server.LoginAsAdmin(t, tourPassword)

	page := readTour(t, c, "/manual")
	assert.False(t, page.Tour.Ask)
	assert.False(t, page.Tour.Askable)

	resp := c.Get("/manual")
	defer testutil.DrainAndClose(resp)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `data-tour-start="4"`, "the Welcome card can jump to a tip")
}

func TestTour_GuestsAreNotAsked(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, tourPassword)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	assert.False(t, readTour(t, server.NewClient(t), "/").Tour.Ask)
}

func TestTour_AnsweringNeedsSignIn(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, tourPassword)
	server := testutil.NewTestServer(t, db)
	c := server.NewClient(t)

	resp := c.SessionPostJSON(t, "/tour/answered", c.FetchCSRFToken("/login"), map[string]any{})
	testutil.DrainAndClose(resp)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	answered, err := handlers.GetSetting(db, "tour.answered")
	require.NoError(t, err)
	assert.Empty(t, answered)
}

// The demo asks each visitor in their browser (tour.js) and saves nothing.
func TestTour_DemoAsksInTheBrowserAndSavesNothing(t *testing.T) {
	t.Parallel()
	e := newDemo(t)

	page := readTour(t, e.client, "/")
	assert.True(t, page.Demo)
	assert.True(t, page.Tour.Askable, "tour.js asks demo visitors on Today")

	status, _ := e.send(t, http.MethodPost, "/tour/answered", "application/json", []byte(`{}`))
	assert.Equal(t, http.StatusForbidden, status)
}
