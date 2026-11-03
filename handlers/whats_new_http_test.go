package handlers_test

// The What's new card (handlers/whats_new.go): who sees it, which releases,
// that closing it sticks, and that every card has its text in every language.

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
	"isley/updates"
	"isley/utils"
)

const whatsNewPassword = "whats-new-pass"

var whatsNewRe = regexp.MustCompile(`(?s)<script type="application/json" id="bmWhatsNew">(.*?)</script>`)

// readWhatsNew fetches a page and returns the cards it hands to whats-new.js,
// or nil when the page has none.
func readWhatsNew(t *testing.T, c *testutil.Client, path string) *handlers.WhatsNewData {
	t.Helper()
	resp := c.Get(path)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, path)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	m := whatsNewRe.FindSubmatch(body)
	if m == nil {
		return nil
	}
	var out struct {
		Data handlers.WhatsNewData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(m[1], &out), string(m[1]))
	return &out.Data
}

// seedGrow gives the install a plant, so it doesn't count as new.
func seedGrow(t *testing.T, s *testutil.TestServer) {
	t.Helper()
	breeder := testutil.SeedBreeder(t, s.DB, "Breeder")
	strain := testutil.SeedStrain(t, s.DB, breeder, "Strain")
	testutil.SeedPlant(t, s.DB, "Plant", strain, testutil.SeedZone(t, s.DB, "Zone"))
}

func whatsNewServer(t *testing.T, version, seen string, grow bool) (*testutil.TestServer, *testutil.Client, string) {
	t.Helper()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, whatsNewPassword)
	if seen != "" {
		require.NoError(t, handlers.UpdateSetting(db, nil, "whatsnew.seen", seen))
	}
	server := testutil.NewTestServer(t, db, testutil.WithReleaseVersion(version))
	if grow {
		seedGrow(t, server)
	}
	c, token := server.LoginAndFetchCSRF(t, whatsNewPassword, "/")
	return server, c, token
}

func TestWhatsNew_UpgradeFromBefore2SeesTheCardUntilClosed(t *testing.T) {
	t.Parallel()
	server, c, token := whatsNewServer(t, "2.0", "", true)

	got := readWhatsNew(t, c, "/plants")
	require.NotNil(t, got, "any signed-in page shows it")
	assert.Empty(t, got.From, "the earlier version wasn't recorded")
	assert.Equal(t, "2.0", got.To)
	require.Len(t, got.Cards, 1)
	card := got.Cards[0]
	assert.Equal(t, "2.0", card.Version)
	assert.Equal(t, "https://github.com/SirBusta/budminder/releases/tag/v2.0.0", card.URL)
	require.NotEmpty(t, card.Features)
	assert.Equal(t, "Email alerts", card.Features[0].Title)
	assert.Equal(t, "/settings#alerts", card.Features[0].Link)
	assert.True(t, card.Features[0].Beta)
	assert.Len(t, card.Features[0].Points, 4)
	assert.Contains(t, card.Features[0].After, "Settings > Alerts")
	assert.NotEmpty(t, got.T["al_beta_note"])

	assert.NotNil(t, readWhatsNew(t, c, "/"), "still there until it's closed")

	resp := c.SessionPostJSON(t, "/whats-new/seen", token, map[string]any{})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Nil(t, readWhatsNew(t, c, "/"), "shown once")
	seen, err := handlers.GetSetting(server.DB, "whatsnew.seen")
	require.NoError(t, err)
	assert.Equal(t, "2.0", seen)
}

func TestWhatsNew_NoCard(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		version, seen string
		grow          bool
		wantSeen      string
	}{
		{"an install with nothing in it counts as new", "2.0", "", false, "2.0"},
		{"this version was already seen", "2.0", "2.0", true, "2.0"},
		{"an older program leaves a newer record alone", "2.0", "2.1.0", true, "2.1.0"},
		{"an older release has no card", "1.5.10", "", true, "1.5.10"},
		{"a release with no card of its own is noted", "2.1", "2.0", true, "2.1"},
		{"a test build", "isley-test", "", true, ""},
		{"no version", "", "", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server, c, _ := whatsNewServer(t, tc.version, tc.seen, tc.grow)
			assert.Nil(t, readWhatsNew(t, c, "/"))
			seen, err := handlers.GetSetting(server.DB, "whatsnew.seen")
			require.NoError(t, err)
			assert.Equal(t, tc.wantSeen, seen)
		})
	}
}

// A release skipped on the way still shows, and a later one with no entry of
// its own doesn't get in the way.
func TestWhatsNew_SkippedReleaseShowsWithTheRealFromVersion(t *testing.T) {
	t.Parallel()
	_, c, _ := whatsNewServer(t, "2.3", "1.9.0", true)

	got := readWhatsNew(t, c, "/")
	require.NotNil(t, got)
	assert.Equal(t, "1.9.0", got.From)
	assert.Equal(t, "2.3", got.To)
	require.Len(t, got.Cards, 1)
	assert.Equal(t, "2.0", got.Cards[0].Version)
}

func TestWhatsNew_SignedOutPageHasNoCard(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, whatsNewPassword)
	server := testutil.NewTestServer(t, db, testutil.WithReleaseVersion("2.0"), testutil.WithGuestMode())
	seedGrow(t, server)

	assert.Nil(t, readWhatsNew(t, server.NewClient(t), "/"))
	seen, err := handlers.GetSetting(db, "whatsnew.seen")
	require.NoError(t, err)
	assert.Empty(t, seen, "a guest doesn't use up the card")

	c := server.NewClient(t)
	resp := c.SessionPostJSON(t, "/whats-new/seen", c.FetchCSRFToken("/login"), map[string]any{})
	testutil.DrainAndClose(resp)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

func TestWhatsNew_TextInEveryLanguage(t *testing.T) {
	t.Parallel()
	for _, lang := range utils.AvailableLanguages {
		lcl := utils.TranslationService.GetTranslations(lang)
		for _, k := range []string{"wn_title", "wn_from", "wn_to", "wn_features", "wn_other", "wn_fixes", "wn_see_all", "wn_got_it"} {
			assert.NotEmpty(t, lcl[k], "%s %s", lang, k)
		}
		for _, card := range handlers.WhatsNewAllCards(lcl) {
			for _, f := range card.Features {
				assert.NotEmpty(t, f.Title, "%s %s", lang, card.Version)
				assert.NotEmpty(t, f.Text, "%s %s", lang, card.Version)
				assert.NotEmpty(t, f.LinkText, "%s %s", lang, card.Version)
				for _, s := range f.Points {
					assert.NotEmpty(t, s, "%s %s", lang, card.Version)
				}
			}
			for _, s := range append(append([]string{}, card.Other...), card.Fixes...) {
				assert.NotEmpty(t, s, "%s %s", lang, card.Version)
			}
		}
	}
}

// A release can't go out with a blank date on its card: this fails as soon as
// VERSION reaches a card whose Date wasn't filled in.
func TestWhatsNew_ReleasedCardsHaveADate(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../VERSION")
	require.NoError(t, err)
	current := strings.TrimSpace(string(raw))
	for _, card := range handlers.WhatsNewAllCards(map[string]string{}) {
		if updates.Newer(card.Version, current) {
			continue
		}
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, card.Date, "card %s", card.Version)
	}
}

// The demo never shows it, and can't save it.
func TestWhatsNew_DemoNeverShowsIt(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	assert.Nil(t, readWhatsNew(t, e.client, "/"))
	status, _ := e.send(t, http.MethodPost, "/whats-new/seen", "application/json", []byte(`{}`))
	assert.Equal(t, http.StatusForbidden, status)
}
