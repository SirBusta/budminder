package handlers

// The Help manual: sections written in Markdown (web/manual/en/<slug>.md),
// listed in order in web/manual/manual.json, built into the program and
// shown at /manual/<slug>. The content is ours (embedded, not user input),
// so it is rendered without a sanitizer.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"regexp"
	"strings"
	"sync"

	"github.com/russross/blackfriday/v2"
)

const (
	manualOutlinePath = "web/manual/manual.json"
	manualDir         = "web/manual/en"
	// ManualFigureDir is where the manual's pictures live, as <fig>-light.webp
	// and <fig>-dark.webp (made by data/demo/manual_figs.py).
	ManualFigureDir = "web/static/img/manual"
)

// ManualSection is one page of the manual.
type ManualSection struct {
	Slug  string   `json:"slug"`
	Title string   `json:"title"`
	Group string   `json:"group"`
	Paths []string `json:"-"` // app pages it explains, for the ? icon ("*" = any one path part)

	HTML     template.HTML   `json:"-"`
	Headings []ManualHeading `json:"-"`
	Text     string          `json:"text"` // plain text, for search
	Figures  []string        `json:"-"`    // fig: names used
	Markers  []string        `json:"-"`    // {n} numbers used
}

// ManualHeading is an h2 in a section, for "On this page".
type ManualHeading struct {
	ID    string
	Title string
}

// ManualGroup is a heading in the left menu.
type ManualGroup struct {
	Title    string
	Sections []*ManualSection
}

// Manual is the whole manual, in menu order.
type Manual struct {
	Groups   []ManualGroup
	Sections []*ManualSection // flat, in order
	bySlug   map[string]*ManualSection
}

// Section returns the section with that slug, or nil.
func (m *Manual) Section(slug string) *ManualSection { return m.bySlug[slug] }

// ForPath returns the section that explains an app page: the most specific
// match of its paths ("/plant/*" matches "/plant/12"), or the first section.
func (m *Manual) ForPath(path string) *ManualSection {
	best, bestScore := m.Sections[0], -1
	if path == "" {
		return best
	}
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for _, s := range m.Sections {
		for _, p := range s.Paths {
			pp := strings.Split(strings.TrimSuffix(p, "/"), "/")
			if len(pp) != len(parts) {
				continue
			}
			score := 0
			for i := range pp {
				if pp[i] == "*" {
					if parts[i] == "" {
						score = -1
						break
					}
					continue
				}
				if pp[i] != parts[i] {
					score = -1
					break
				}
				score++
			}
			if score > bestScore {
				best, bestScore = s, score
			}
		}
	}
	return best
}

// Prev and Next return the neighboring sections, or nil.
func (m *Manual) Prev(s *ManualSection) *ManualSection { return m.step(s, -1) }
func (m *Manual) Next(s *ManualSection) *ManualSection { return m.step(s, 1) }

func (m *Manual) step(s *ManualSection, d int) *ManualSection {
	for i, x := range m.Sections {
		if x == s && i+d >= 0 && i+d < len(m.Sections) {
			return m.Sections[i+d]
		}
	}
	return nil
}

// manualCache holds one parsed manual per asset FS: the files are built into
// the program and never change while it runs.
var manualCache sync.Map

// LoadManual reads and renders the manual from the assets FS (cached).
func LoadManual(assets fs.FS) (*Manual, error) {
	if assets == nil {
		return nil, fmt.Errorf("no embedded assets")
	}
	if m, ok := manualCache.Load(assets); ok {
		return m.(*Manual), nil
	}
	m, err := parseManual(assets)
	if err != nil {
		return nil, err
	}
	manualCache.Store(assets, m)
	return m, nil
}

func parseManual(assets fs.FS) (*Manual, error) {
	data, err := fs.ReadFile(assets, manualOutlinePath)
	if err != nil {
		return nil, err
	}
	var outline struct {
		Groups []struct {
			Title    string `json:"title"`
			Sections []struct {
				Slug  string   `json:"slug"`
				Title string   `json:"title"`
				Paths []string `json:"paths"`
			} `json:"sections"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &outline); err != nil {
		return nil, fmt.Errorf("%s: %w", manualOutlinePath, err)
	}
	m := &Manual{bySlug: map[string]*ManualSection{}}
	for _, g := range outline.Groups {
		grp := ManualGroup{Title: g.Title}
		for _, sec := range g.Sections {
			if m.bySlug[sec.Slug] != nil {
				return nil, fmt.Errorf("manual: section %q listed twice", sec.Slug)
			}
			md, err := fs.ReadFile(assets, manualDir+"/"+sec.Slug+".md")
			if err != nil {
				return nil, fmt.Errorf("manual: %w", err)
			}
			s := &ManualSection{Slug: sec.Slug, Title: sec.Title, Group: g.Title, Paths: sec.Paths}
			renderManualSection(s, md)
			grp.Sections = append(grp.Sections, s)
			m.Sections = append(m.Sections, s)
			m.bySlug[s.Slug] = s
		}
		m.Groups = append(m.Groups, grp)
	}
	if len(m.Sections) == 0 {
		return nil, fmt.Errorf("manual: no sections")
	}
	return m, nil
}

var (
	// ![alt](fig:name) on its own line → the picture in both themes.
	manualFigP   = regexp.MustCompile(`<p><img src="fig:([a-z0-9-]+)" alt="([^"]*)" /></p>`)
	manualFig    = regexp.MustCompile(`<img src="fig:([a-z0-9-]+)" alt="([^"]*)" />`)
	manualMarker = regexp.MustCompile(`\{(\d{1,2})\}`)
	manualH2     = regexp.MustCompile(`<h2 id="([^"]+)">(.*?)</h2>`)
	manualTags   = regexp.MustCompile(`<[^>]+>`)
	manualBlocks = regexp.MustCompile(`</?(?:p|li|h\d|ul|ol|blockquote|div|table|tr|td|th|br)[^>]*>`)
	manualSpaces = regexp.MustCompile(`\s+`)
)

func renderManualSection(s *ManualSection, md []byte) {
	// No "smart" quotes or dashes: the manual keeps straight apostrophes.
	out := blackfriday.Run(bytes.ReplaceAll(md, []byte("\r\n"), []byte("\n")),
		blackfriday.WithExtensions(blackfriday.CommonExtensions|blackfriday.AutoHeadingIDs),
		blackfriday.WithRenderer(blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{Flags: blackfriday.UseXHTML})))
	h := string(out)

	for _, f := range manualFig.FindAllStringSubmatch(h, -1) {
		s.Figures = append(s.Figures, f[1])
	}
	figure := func(name, alt string) string {
		src := "/static/img/manual/" + name
		return fmt.Sprintf(`<span class="mn-fig"><img class="light" src="%s-light.webp" alt="%s" loading="lazy"><img class="dark" src="%s-dark.webp" alt="%s" loading="lazy"></span>`,
			src, alt, src, alt)
	}
	h = manualFigP.ReplaceAllStringFunc(h, func(x string) string {
		g := manualFigP.FindStringSubmatch(x)
		return `<div class="mn-figwrap">` + figure(g[1], g[2]) + `</div>`
	})
	h = manualFig.ReplaceAllStringFunc(h, func(x string) string {
		g := manualFig.FindStringSubmatch(x)
		return figure(g[1], g[2])
	})

	seen := map[string]bool{}
	for _, mk := range manualMarker.FindAllStringSubmatch(h, -1) {
		if !seen[mk[1]] {
			seen[mk[1]] = true
			s.Markers = append(s.Markers, mk[1])
		}
	}
	h = manualMarker.ReplaceAllString(h, `<span class="mn-n">$1</span>`)

	for _, hd := range manualH2.FindAllStringSubmatch(h, -1) {
		s.Headings = append(s.Headings, ManualHeading{ID: hd[1], Title: html.UnescapeString(manualTags.ReplaceAllString(hd[2], ""))})
	}
	s.HTML = template.HTML(h) // #nosec G203 -- built-in manual text, not user input
	// Plain text for search: blocks become spaces, inline tags just go.
	text := manualMarker.ReplaceAllString(string(out), "")
	text = html.UnescapeString(manualTags.ReplaceAllString(manualBlocks.ReplaceAllString(text, " "), ""))
	s.Text = strings.TrimSpace(manualSpaces.ReplaceAllString(text, " "))
}

// ManualSearchIndex is the search data the manual page loads: every
// section's title, group and plain text.
func (m *Manual) SearchIndex() []*ManualSection { return m.Sections }
