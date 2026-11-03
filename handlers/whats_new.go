package handlers

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/updates"
)

// The What's new card (web/static/js/whats-new.js, common/header2.html): after
// an upgrade, one card per release the install hasn't seen, newest first, on
// the first page after signing in. The "whatsnew.seen" setting holds the
// version last seen; an install from before 2.0 has none. Closing the card
// saves the running version. A restore keeps the setting (keepWhatsNewSeen),
// so an old backup doesn't bring cards back.
//
// An install with no strains and no plants never gets the card: it counts as
// new, the same as for the tour, and just has the version noted.

const (
	whatsNewSeenSetting = "whatsnew.seen"
	contextKeyWhatsNew  = "whatsNew"
	releasePageURL      = "https://github.com/SirBusta/budminder/releases/tag/"
)

// whatsNewRelease is one release's card. Its text is in the translations,
// keyed from the version: wn_2_0_f1_title, _f1_text, _f1_link for feature 1,
// plus _f1_p1... for its bullet points and _f1_after for text below them;
// wn_2_0_o1 for other change 1,
// wn_2_0_x1 for bug fix 1. Date (YYYY-MM-DD) is set when the release is cut. A
// release with no entry here has no card.
type whatsNewRelease struct {
	Version  string
	Date     string
	Features []whatsNewFeatureDef
	Other    int
	Fixes    int
}

type whatsNewFeatureDef struct {
	Icon   string
	Link   string
	Points int
	Beta   bool // adds the Beta tag and the note with the feedback address
}

// whatsNewReleases is newest first.
var whatsNewReleases = []whatsNewRelease{
	{
		Version: "2.0",
		Features: []whatsNewFeatureDef{
			{Icon: "fa-bell", Link: "/settings#alerts", Points: 4, Beta: true},
		},
		Other: 4,
		Fixes: 3,
	},
}

// WhatsNewFeature is a new feature on a card.
type WhatsNewFeature struct {
	Icon     string   `json:"icon"`
	Title    string   `json:"title"`
	Text     string   `json:"text"`
	Points   []string `json:"points"`
	After    string   `json:"after"`
	Link     string   `json:"link"`
	LinkText string   `json:"link_text"`
	Beta     bool     `json:"beta"`
}

// WhatsNewCard is one release's card, in the visitor's language.
type WhatsNewCard struct {
	Version  string            `json:"version"`
	Date     string            `json:"date"`
	URL      string            `json:"url"`
	Features []WhatsNewFeature `json:"features"`
	Other    []string          `json:"other"`
	Fixes    []string          `json:"fixes"`
}

// WhatsNewData is what header2.html hands to whats-new.js. From is empty for
// an install from before 2.0, whose earlier version wasn't recorded.
type WhatsNewData struct {
	From  string            `json:"from"`
	To    string            `json:"to"`
	Cards []WhatsNewCard    `json:"cards"`
	T     map[string]string `json:"t"`
}

// WhatsNew answers for one engine. Version is the plain release version
// ("2.0"); empty or anything else (test builds, the demo) never shows a card.
type WhatsNew struct {
	DB      *sql.DB
	Version string
}

// SetWhatsNewOnContext makes w available to WhatsNewSeenHandler.
func SetWhatsNewOnContext(c *gin.Context, w *WhatsNew) {
	c.Set(contextKeyWhatsNew, w)
}

// Page returns the cards to show, or nil. Versions that have no card to show
// are noted here, so a later card's "Updated from" names the real version.
func (w *WhatsNew) Page(lcl map[string]string) *WhatsNewData {
	if w == nil || !updates.Newer(w.Version, "0.0.0") {
		return nil
	}
	seen, err := GetSetting(w.DB, whatsNewSeenSetting)
	if err != nil || seen == w.Version {
		return nil
	}
	if seen != "" && !updates.Newer(w.Version, seen) {
		return nil // an older program on a newer database
	}
	unseen := whatsNewBetween(seen, w.Version)
	if len(unseen) == 0 || hasNoStrainsOrPlants(w.DB) {
		_ = w.MarkSeen()
		return nil
	}
	out := &WhatsNewData{From: seen, To: w.Version, T: map[string]string{}}
	for _, k := range []string{"wn_title", "wn_from", "wn_to", "wn_features", "wn_other", "wn_fixes", "wn_see_all", "wn_got_it", "title_close", "al_beta", "al_beta_note", "al_beta_feedback"} {
		out.T[k] = lcl[k]
	}
	for _, r := range unseen {
		out.Cards = append(out.Cards, r.card(lcl))
	}
	return out
}

// MarkSeen saves the running version as seen.
func (w *WhatsNew) MarkSeen() error {
	return UpdateSetting(w.DB, nil, whatsNewSeenSetting, w.Version)
}

// whatsNewBetween returns the releases after seen ("": all) up to current.
func whatsNewBetween(seen, current string) []whatsNewRelease {
	var out []whatsNewRelease
	for _, r := range whatsNewReleases {
		if updates.Newer(r.Version, current) || (seen != "" && !updates.Newer(r.Version, seen)) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (r whatsNewRelease) card(lcl map[string]string) WhatsNewCard {
	key := "wn_" + strings.ReplaceAll(r.Version, ".", "_")
	card := WhatsNewCard{Version: r.Version, Date: r.Date, URL: releasePageURL + updates.Tag(r.Version)}
	for i, f := range r.Features {
		k := fmt.Sprintf("%s_f%d_", key, i+1)
		feature := WhatsNewFeature{Icon: f.Icon, Title: lcl[k+"title"], Text: lcl[k+"text"], Link: f.Link, LinkText: lcl[k+"link"], Beta: f.Beta}
		for p := 1; p <= f.Points; p++ {
			feature.Points = append(feature.Points, lcl[fmt.Sprintf("%sp%d", k, p)])
		}
		feature.After = lcl[k+"after"]
		card.Features = append(card.Features, feature)
	}
	for i := 1; i <= r.Other; i++ {
		card.Other = append(card.Other, lcl[fmt.Sprintf("%s_o%d", key, i)])
	}
	for i := 1; i <= r.Fixes; i++ {
		card.Fixes = append(card.Fixes, lcl[fmt.Sprintf("%s_x%d", key, i)])
	}
	return card
}

// WhatsNewAllCards returns every release's card, whatever has been seen.
func WhatsNewAllCards(lcl map[string]string) []WhatsNewCard {
	var out []WhatsNewCard
	for _, r := range whatsNewReleases {
		out = append(out, r.card(lcl))
	}
	return out
}

// WhatsNewSeenHandler records that the card was closed (POST /whats-new/seen).
func WhatsNewSeenHandler(c *gin.Context) {
	w, _ := c.Get(contextKeyWhatsNew)
	wn, _ := w.(*WhatsNew)
	if wn == nil || !updates.Newer(wn.Version, "0.0.0") {
		apiOK(c, "api_settings_saved") // no card to close on this build
		return
	}
	if err := wn.MarkSeen(); err != nil {
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	apiOK(c, "api_settings_saved")
}

// keepWhatsNewSeen reads the version seen before a restore replaces the
// settings table and returns a func that puts it back, so a restore neither
// repeats cards nor hides new ones.
func keepWhatsNewSeen(db *sql.DB) func() {
	saved, err := GetSetting(db, whatsNewSeenSetting)
	return func() {
		if err != nil || saved == "" {
			return
		}
		_ = UpdateSetting(db, nil, whatsNewSeenSetting, saved)
	}
}
