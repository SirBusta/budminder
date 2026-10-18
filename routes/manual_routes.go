package routes

// The Help manual: readable by everyone, logged in or not, even with guest
// view off (registered with the public routes in app/routes.go).

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"isley/handlers"
	"isley/logger"
)

// AddManualRoutes registers /manual, /manual/:slug, /manual/for and the
// search index.
func AddManualRoutes(r *gin.RouterGroup, version string) {
	load := func(c *gin.Context) *handlers.Manual {
		m, err := handlers.LoadManual(handlers.AssetsFromContext(c))
		if err != nil {
			logger.Log.WithError(err).Error("Failed to load the manual")
			c.String(http.StatusInternalServerError, "The manual could not be loaded.")
			return nil
		}
		return m
	}
	show := func(c *gin.Context, m *handlers.Manual, s *handlers.ManualSection) {
		smokePage(c, version, "views/manual.html", s.Title+" · Help", gin.H{
			"manual": m,
			"sec":    s,
			"prev":   m.Prev(s),
			"next":   m.Next(s),
		})
	}

	r.GET("/manual", func(c *gin.Context) {
		if m := load(c); m != nil {
			show(c, m, m.Sections[0])
		}
	})
	// The ? icon: the section for the page you were on.
	r.GET("/manual/for", func(c *gin.Context) {
		m := load(c)
		if m == nil {
			return
		}
		c.Redirect(http.StatusFound, "/manual/"+url.PathEscape(m.ForPath(c.Query("path")).Slug))
	})
	r.GET("/manual/index.json", func(c *gin.Context) {
		if m := load(c); m != nil {
			c.JSON(http.StatusOK, m.SearchIndex())
		}
	})
	r.GET("/manual/:slug", func(c *gin.Context) {
		m := load(c)
		if m == nil {
			return
		}
		s := m.Section(c.Param("slug"))
		if s == nil {
			c.Redirect(http.StatusFound, "/manual")
			return
		}
		show(c, m, s)
	})
}
