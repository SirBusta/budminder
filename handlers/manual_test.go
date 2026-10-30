package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// The manual is read from the repo, the same files the program embeds.
func loadTestManual(t *testing.T) *handlers.Manual {
	t.Helper()
	m, err := handlers.LoadManual(os.DirFS(".."))
	require.NoError(t, err)
	return m
}

func TestManual_EverySectionHasAFileAndEveryFileASection(t *testing.T) {
	t.Parallel()
	m := loadTestManual(t)
	files, err := filepath.Glob(filepath.Join("..", "web", "manual", "en", "*.md"))
	require.NoError(t, err)
	assert.Len(t, files, len(m.Sections), "every .md file is listed in web/manual/manual.json")
	for _, f := range files {
		slug := strings.TrimSuffix(filepath.Base(f), ".md")
		assert.NotNil(t, m.Section(slug), "%s is not in manual.json", f)
	}
}

// Every picture a section uses exists in both themes, and every numbered
// circle in the text is on one of that section's pictures.
func TestManual_FiguresAndMarkers(t *testing.T) {
	t.Parallel()
	m := loadTestManual(t)
	raw, err := os.ReadFile(filepath.Join("..", "web", "manual", "figures.json"))
	require.NoError(t, err)
	var figs map[string][]string
	require.NoError(t, json.Unmarshal(raw, &figs))

	for _, s := range m.Sections {
		have := map[string]bool{}
		for _, f := range s.Figures {
			for _, theme := range []string{"light", "dark"} {
				_, err := os.Stat(filepath.Join("..", handlers.ManualFigureDir, f+"-"+theme+".webp"))
				assert.NoError(t, err, "%s: picture %s-%s.webp", s.Slug, f, theme)
			}
			require.Contains(t, figs, f, "%s: %s is not in figures.json (made by data/demo/manual_figs.py)", s.Slug, f)
			for _, n := range figs[f] {
				have[n] = true
			}
		}
		for _, n := range s.Markers {
			assert.True(t, have[n], "%s: circle {%s} isn't on any of its pictures", s.Slug, n)
		}
	}
}

func TestManual_ForPath(t *testing.T) {
	t.Parallel()
	m := loadTestManual(t)
	for path, want := range map[string]string{
		"/":               "today",
		"/plants":         "plants-list",
		"/plant/5":        "plant-page",
		"/plant/new":      "adding-a-plant",
		"/plant/5/edit":   "adding-a-plant",
		"/strains":        "strains-and-seed-vault",
		"/strain/3":       "strain-page",
		"/strain/new":     "adding-a-strain",
		"/smoke":          "writing-a-report",
		"/smoke/new":      "writing-a-report",
		"/smoke/7":        "report-page",
		"/sensors":        "environment",
		"/sensors/manage": "manage-sensors",
		"/settings":       "settings-general",
		"/login":          "first-login",
		"/nowhere/at/all": "welcome",
		"":                "welcome",
	} {
		assert.Equal(t, want, m.ForPath(path).Slug, "path %q", path)
	}
}

func TestManual_RenderExtras(t *testing.T) {
	t.Parallel()
	m := loadTestManual(t)
	s := m.Section("plant-page")
	require.NotNil(t, s)
	h := string(s.HTML)
	assert.Contains(t, h, `<span class="mn-n">1</span>`, "{1} becomes a circle")
	assert.Contains(t, h, `/static/img/manual/plant-stages-light.webp`)
	assert.Contains(t, h, `/static/img/manual/plant-stages-dark.webp`)
	assert.NotContains(t, h, "fig:")
	require.NotEmpty(t, s.Headings)
	assert.Equal(t, "growth-stages", s.Headings[0].ID)
	assert.Equal(t, "Growth stages", s.Headings[0].Title)
	assert.NotContains(t, s.Text, "<", "search text is plain")
	assert.Contains(t, h, "plant's", "straight apostrophes")
	assert.NotContains(t, h, "&rsquo;")
	assert.Nil(t, m.Prev(m.Sections[0]))
	assert.Equal(t, m.Sections[1], m.Next(m.Sections[0]))
}

// The manual is readable without logging in, even with guest view off.
func TestManual_PagesForAGuest(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	c := server.NewClient(t)

	resp := c.Get("/manual/plant-page")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "Growth stages")
	assert.Contains(t, string(body), `href="/manual/today"`, "the left menu lists the sections")

	resp = c.Get("/manual/alerts")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "Dry run")

	resp = c.Get("/manual")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp = c.Get("/manual/no-such-section")
	resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/manual", resp.Header.Get("Location"))

	resp = c.Get("/manual/for?path=/plant/12")
	resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/manual/plant-page", resp.Header.Get("Location"))

	resp = c.Get("/manual/index.json")
	var idx []struct{ Slug, Title, Group, Text string }
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&idx))
	resp.Body.Close()
	require.NotEmpty(t, idx)
	assert.Equal(t, "welcome", idx[0].Slug)
	assert.NotEmpty(t, idx[0].Text)
}

// Logged in, the top menu's Help item points at the manual for that page.
func TestManual_HelpInTopMenu(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	resp := e.client.Get("/plants")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), `href="/manual/for?path=%2fplants"`)
}

// Every link to another section (and to a heading in it) goes somewhere.
func TestManual_LinksResolve(t *testing.T) {
	t.Parallel()
	m := loadTestManual(t)
	link := regexp.MustCompile(`href="(/manual/[^"]*|#[^"]*)"`)
	for _, s := range m.Sections {
		for _, l := range link.FindAllStringSubmatch(string(s.HTML), -1) {
			target, slug, anchor := l[1], s.Slug, ""
			if strings.HasPrefix(target, "/manual/") {
				rest := strings.TrimPrefix(target, "/manual/")
				slug, anchor, _ = strings.Cut(rest, "#")
			} else {
				anchor = strings.TrimPrefix(target, "#")
			}
			to := m.Section(slug)
			if !assert.NotNil(t, to, "%s links to missing section %q", s.Slug, slug) || anchor == "" {
				continue
			}
			found := false
			for _, h := range to.Headings {
				found = found || h.ID == anchor
			}
			assert.True(t, found, "%s links to %s#%s, but that heading doesn't exist", s.Slug, slug, anchor)
		}
	}
}
