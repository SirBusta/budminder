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
	_, c, _ := whatsNewServer(t, "2.6", "1.9.0", true)

	got := readWhatsNew(t, c, "/")
	require.NotNil(t, got)
	assert.Equal(t, "1.9.0", got.From)
	assert.Equal(t, "2.6", got.To)
	require.Len(t, got.Cards, 5)
	assert.Equal(t, "2.5", got.Cards[0].Version, "newest first")
	assert.Equal(t, "2.4", got.Cards[1].Version)
	assert.Equal(t, "2.3", got.Cards[2].Version)
	assert.Equal(t, "2.2", got.Cards[3].Version)
	assert.Equal(t, "2.0", got.Cards[4].Version)
}

// A Beta feature with its own note (Vivosun in 2.4) sends it; one without
// (alerts) sends none, so the card shows the alerts note.
func TestWhatsNew_BetaFeatureCarriesItsOwnNote(t *testing.T) {
	t.Parallel()
	_, c, _ := whatsNewServer(t, "2.4", "2.2", true)

	got := readWhatsNew(t, c, "/")
	require.NotNil(t, got)
	require.Len(t, got.Cards, 2)
	require.Equal(t, "2.4", got.Cards[0].Version)
	vs := got.Cards[0].Features[0]
	assert.True(t, vs.Beta)
	assert.Contains(t, vs.BetaNote, "VCure C80")
	assert.Equal(t, "Personal details are automatically removed from the file.", vs.BetaAfter, "the line after the address")
	alerts := got.Cards[1].Features[0]
	assert.True(t, alerts.Beta)
	assert.Empty(t, alerts.BetaNote)
	assert.Empty(t, alerts.BetaAfter)
}

// 2.5's notification methods carry the Beta tag without a note: Settings shows it.
func TestWhatsNew_BetaTagWithoutANote(t *testing.T) {
	t.Parallel()
	_, c, _ := whatsNewServer(t, "2.5", "2.4", true)

	got := readWhatsNew(t, c, "/")
	require.NotNil(t, got)
	require.Len(t, got.Cards, 1)
	f := got.Cards[0].Features[0]
	assert.True(t, f.Beta)
	assert.True(t, f.NoNote)
	assert.Empty(t, f.BetaNote)
	assert.Len(t, got.Cards[0].Features, 2)
	assert.Len(t, got.Cards[0].Other, 3)
	assert.Len(t, got.Cards[0].Fixes, 1)
	assert.False(t, got.Cards[0].Features[1].Beta)
}

// 2.2 to 2.3: the Recent alerts card, with its picture's words and units.
func TestWhatsNew_From22ShowsThe23CardWithItsPicture(t *testing.T) {
	t.Parallel()
	_, c, _ := whatsNewServer(t, "2.3", "2.2", true)

	got := readWhatsNew(t, c, "/")
	require.NotNil(t, got)
	require.Len(t, got.Cards, 1)
	card := got.Cards[0]
	assert.Equal(t, "2.3", card.Version)
	require.Len(t, card.Features, 1)
	f := card.Features[0]
	assert.Equal(t, "More details in Recent alerts", f.Title)
	assert.Equal(t, "/settings#alerts", f.Link)
	assert.True(t, f.Beta)
	assert.Len(t, f.Points, 3)
	assert.Equal(t, "alert_chart", f.Figure)
	require.Len(t, card.Other, 1)
	assert.Contains(t, card.Other[0], "CO2 high in 3x3 ACI Tent")
	assert.Equal(t, "Recent alerts", got.T["al_recent_title"], "the picture's words come with it")
	assert.Equal(t, "Temperature high: {value}", got.T["wn_2_3_fig_alert"])
	assert.NotEmpty(t, got.T["wn_2_3_fig_alt"])
	assert.False(t, got.Metric, "imperial by default")
}

func TestWhatsNew_PictureFollowsMetricUnits(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, whatsNewPassword)
	require.NoError(t, handlers.UpdateSetting(db, nil, "whatsnew.seen", "2.2"))
	server := testutil.NewTestServer(t, db, testutil.WithReleaseVersion("2.3"))
	server.ConfigStore.SetUnits("metric")
	seedGrow(t, server)
	c, _ := server.LoginAndFetchCSRF(t, whatsNewPassword, "/")
	got := readWhatsNew(t, c, "/")
	require.NotNil(t, got)
	assert.True(t, got.Metric)
}

// 2.1 to 2.2: only the 2.2 card, with both features and their links.
func TestWhatsNew_From21ShowsOnlyThe22Card(t *testing.T) {
	t.Parallel()
	_, c, _ := whatsNewServer(t, "2.2", "2.1", true)

	got := readWhatsNew(t, c, "/")
	require.NotNil(t, got)
	assert.Equal(t, "2.1", got.From)
	require.Len(t, got.Cards, 1)
	card := got.Cards[0]
	assert.Equal(t, "2.2", card.Version)
	assert.Equal(t, "https://github.com/SirBusta/budminder/releases/tag/v2.2.0", card.URL)
	require.Len(t, card.Features, 2)
	assert.Equal(t, "Seed Vault pack prices", card.Features[0].Title)
	assert.Equal(t, "/strains#vault", card.Features[0].Link)
	assert.Len(t, card.Features[0].Points, 3)
	assert.Contains(t, card.Features[0].After, "including freebies.")
	assert.False(t, card.Features[0].Beta)
	assert.Equal(t, "/strain/new", card.Features[1].Link)
	assert.Empty(t, card.Features[1].Points)
	assert.Empty(t, card.Other)
	assert.Empty(t, card.Fixes)
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
				for _, k := range handlers.WhatsNewFigureKeys(f.Figure) {
					assert.NotEmpty(t, lcl[k], "%s %s picture %s", lang, card.Version, k)
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
