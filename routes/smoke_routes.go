package routes

// Smoke reports: the Smoke Reports page and a report's page are readable like
// the Strains page; writing them (form, API) needs a login.

import (
	"net/http"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"isley/handlers"
	"isley/utils"
)

func smokePage(c *gin.Context, version, view, title string, extra gin.H) {
	lang := utils.GetLanguage(c)
	translations := utils.TranslationService.GetTranslations(lang)
	currentPath, _ := c.Get("currentPath")
	h := gin.H{
		"title":           title,
		"currentPath":     currentPath,
		"version":         version,
		"loggedIn":        sessions.Default(c).Get("logged_in"),
		"lcl":             translations,
		"languages":       utils.AvailableLanguages,
		"currentLanguage": lang,
		"csrfToken":       c.GetString("csrf_token"),
		"cspNonce":        c.GetString("cspNonce"),
	}
	for k, v := range extra {
		h[k] = v
	}
	c.HTML(http.StatusOK, view, h)
}

// addSmokeBasicRoutes: the list page, its data, and a report's page.
func addSmokeBasicRoutes(r *gin.RouterGroup, version string) {
	r.GET("/smoke", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		loggedIn := sessions.Default(c).Get("logged_in") != nil
		smokePage(c, version, "views/smoke.html", "Smoke Reports", gin.H{
			"pageData": handlers.SmokePageData(c, loggedIn, lang, utils.TranslationService.GetTranslations(lang)),
		})
	})
	r.GET("/smoke/all", handlers.SmokeAllHandler)
	r.GET("/smoke/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.Redirect(http.StatusFound, "/smoke")
			return
		}
		lang := utils.GetLanguage(c)
		view, err := handlers.BuildSmokeReportView(handlers.DBFromContext(c), utils.TranslationService.GetTranslations(lang), id)
		if err != nil {
			c.Redirect(http.StatusFound, "/smoke")
			return
		}
		smokePage(c, version, "views/smoke-report.html", "Smoke Report", gin.H{"v": view})
	})
}

// addSmokeProtectedRoutes: the New / Edit form.
func addSmokeProtectedRoutes(r *gin.RouterGroup, version string) {
	form := func(c *gin.Context, report *handlers.SmokeReport) {
		lang := utils.GetLanguage(c)
		smokePage(c, version, "views/smoke-form.html", "Smoke Report", gin.H{
			"pageData": handlers.SmokeFormData(c, lang, utils.TranslationService.GetTranslations(lang), report),
			"editing":  report != nil,
		})
	}
	r.GET("/smoke/new", func(c *gin.Context) { form(c, nil) })
	r.GET("/smoke/:id/edit", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.Redirect(http.StatusFound, "/smoke")
			return
		}
		rep, err := handlers.GetSmokeReport(handlers.DBFromContext(c), id)
		if err != nil {
			c.Redirect(http.StatusFound, "/smoke")
			return
		}
		form(c, &rep)
	})
}

// addSmokeAPIRoutes: saving, photos, prefill and settings.
func addSmokeAPIRoutes(r *gin.RouterGroup) {
	r.GET("/smoke/prefill", handlers.SmokePrefillHandler)
	r.GET("/smoke/journal-photos", handlers.SmokeJournalPhotosHandler)
	r.POST("/smoke", handlers.CreateSmokeReportHandler)
	r.POST("/smoke/settings", handlers.SaveSmokeSettingsHandler)
	r.PUT("/smoke/:id", handlers.UpdateSmokeReportHandler)
	r.DELETE("/smoke/:id", handlers.DeleteSmokeReportHandler)
	r.POST("/smoke/:id/photos", handlers.UploadSmokePhotosHandler)
	r.POST("/smoke/:id/photos/url", handlers.SmokePhotoFromURLHandler)
	r.POST("/smoke/:id/photos/journal", handlers.SmokePhotoFromJournalHandler)
	r.PUT("/smoke/:id/photos/order", handlers.OrderSmokePhotosHandler)
	r.DELETE("/smoke/:id/photos/:photoID", handlers.DeleteSmokePhotoHandler)
}
