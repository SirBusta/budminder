package routes

import (
	"database/sql"
	"fmt"
	"isley/handlers"
	"isley/logger"
	"isley/model"
	"isley/utils"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func AddBasicRoutes(r *gin.RouterGroup, version string) {
	r.GET("/", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		loggedIn := sessions.Default(c).Get("logged_in") != nil
		pageData, err := handlers.HomePageData(c, loggedIn, lang, translations)
		if err != nil {
			pageData = gin.H{"plants": []any{}, "tents": []any{}, "recent": []any{}, "logged_in": loggedIn, "t": gin.H{}}
		}
		c.HTML(http.StatusOK, "views/index.html", gin.H{
			"pageData":        pageData,
			"hasCamera":       homeHasCamera(pageData),
			"title":           "Today",
			"currentPath":     currentPath,
			"version":         version,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})
	r.GET("/home/data", handlers.HomeDataHandler)

	// Translations API - returns JSON map of translations for the requested language
	r.GET("/api/translations", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.JSON(http.StatusOK, translations)
	})

	r.GET("/plants", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		loggedIn := sessions.Default(c).Get("logged_in") != nil
		c.HTML(http.StatusOK, "views/plants.html", gin.H{
			"pageData":        handlers.PlantsPageData(c, loggedIn, lang, translations),
			"title":           "Plants",
			"currentPath":     currentPath,
			"version":         version,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	// The old Activity Log page: now the All plants view of the Plant page.
	r.GET("/activities", handlers.ActivitiesRedirect)

	r.GET("/strains", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		loggedIn := sessions.Default(c).Get("logged_in") != nil
		c.HTML(http.StatusOK, "views/strains.html", gin.H{
			"pageData":             handlers.StrainsPageData(c, loggedIn, lang, translations),
			"title":                "Strains",
			"currentPath":          currentPath,
			"version":              version,
			"strains":              store.Strains(),
			"breeders":             store.Breeders(),
			"plants":               handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":           store.Activities(),
			"loggedIn":             sessions.Default(c).Get("logged_in"),
			"cannadbEnabled":       store.CannadbEnabled() == 1,
			"straincompassEnabled": store.StraincompassEnabled() == 1,
			"lcl":                  translations,
			"languages":            utils.AvailableLanguages,
			"currentLanguage":      lang,
			"csrfToken":            c.GetString("csrf_token"),
			"cspNonce":             c.GetString("cspNonce"),
		})
	})

	// The old per-sensor chart page: the Environment page shows every
	// reading of the sensor's tent now.
	r.GET("/graph/:id", func(c *gin.Context) {
		var zone sql.NullInt64
		_ = handlers.DBFromContext(c).QueryRow(`SELECT zone_id FROM sensors WHERE id = $1`, c.Param("id")).Scan(&zone)
		if zone.Valid && zone.Int64 > 0 {
			c.Redirect(http.StatusFound, fmt.Sprintf("/sensors#zone=%d", zone.Int64))
			return
		}
		c.Redirect(http.StatusFound, "/sensors")
	})

	r.GET("/plant/new", func(c *gin.Context) {
		// Redirect to login if not authenticated
		if sessions.Default(c).Get("logged_in") == nil {
			c.Redirect(http.StatusFound, "/login")
			return
		}
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/plant-add.html", gin.H{
			"title":           "Add Plant",
			"currentPath":     currentPath,
			"version":         version,
			"zones":           store.Zones(),
			"strains":         store.Strains(),
			"statuses":        store.Statuses(),
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/plant/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		db := handlers.DBFromContext(c)
		idStr := c.Param("id")
		plant := handlers.GetPlant(db, idStr)
		if plant.ID == 0 {
			c.Redirect(http.StatusFound, "/plants")
			return
		}
		prevPlantID, nextPlantID := handlers.GetAdjacentPlantIDs(db, int(plant.ID))
		loggedIn := sessions.Default(c).Get("logged_in") != nil
		pageData, err := handlers.BuildPlantPageData(db, plant, loggedIn, lang, translations)
		if err != nil {
			logger.Log.WithError(err).WithField("plant_id", plant.ID).Error("Failed to build plant page data")
		}
		pageData.Prev, pageData.Next = prevPlantID, nextPlantID
		c.HTML(http.StatusOK, "views/plant.html", gin.H{
			"pageData":        pageData,
			"title":           "Plant Details",
			"currentPath":     currentPath,
			"version":         version,
			"plant":           plant,
			"zones":           store.Zones(),
			"strains":         store.Strains(),
			"statuses":        store.Statuses(),
			"breeders":        store.Breeders(),
			"measurements":    store.Metrics(),
			"sensors":         handlers.GetSensors(db),
			"plants":          handlers.GetLivingPlants(db),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
			"prevPlantID":     prevPlantID,
			"nextPlantID":     nextPlantID,
		})
	})

	r.GET("/strain/new", func(c *gin.Context) {
		// Redirect to login if not authenticated
		if sessions.Default(c).Get("logged_in") == nil {
			c.Redirect(http.StatusFound, "/login")
			return
		}
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/strain-add.html", gin.H{
			"title":           "Add Strain",
			"currentPath":     currentPath,
			"version":         version,
			"prefillName":     c.Query("name"),
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/strain/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		db := handlers.DBFromContext(c)
		strain := handlers.GetStrain(db, c.Param("id"))
		history, err := handlers.LoadStrainHistory(db, strain.ID, handlers.StockToday(c), translations)
		if err != nil {
			history = handlers.StrainHistory{}
		}
		c.HTML(http.StatusOK, "views/strain.html", gin.H{
			"title":            "Strain Details",
			"history":          history,
			"noHistory":        len(history.Grows) == 0 && history.PhotoCount == 0,
			"breederCount":     handlers.BreederStrainCount(db, strain.BreederID),
			"strainLink":       handlers.SafeStrainLink(strain.Url),
			"strainLinkHost":   handlers.StrainLinkHost(strain.Url),
			"growURL":          handlers.StrainGrowURL(handlers.StrainGrowTarget{ID: strain.ID, Name: strain.Name, SeedCount: strain.SeedCount}, len(history.Grows)),
			"canGrow":          strain.SeedCount > 0 || strain.SeedType == "clone",
			"potency":          handlers.StrainPotency(strain),
			"effects":          handlers.StrainEffectRows(strain),
			"terpenes":         handlers.StrainTerpeneRows(strain),
			"today":            handlers.StockToday(c),
			"currentPath":      currentPath,
			"version":          version,
			"strain":           strain,
			"cannadbURL":       handlers.CannadbWebURL(strain.CannadbURI),
			"straincompassURL": handlers.StraincompassWebURL(strain.StraincompassSlug),
			// Family diagram: parents you don't have link to a StrainCompass search.
			"straincompassEnabled": store.StraincompassEnabled() == 1,
			"lineageSourceURL": handlers.CannadbWebURL(strain.LineageSourceURI),
			"breeders":         store.Breeders(),
			"loggedIn":         sessions.Default(c).Get("logged_in"),
			"lcl":              translations,
			"languages":        utils.AvailableLanguages,
			"currentLanguage":  lang,
			"csrfToken":        c.GetString("csrf_token"),
			"cspNonce":         c.GetString("cspNonce"),
		})
	})

	r.GET("/listFonts", utils.ListFontsHandler)
	r.GET("/listLogos", utils.ListLogosHandler)

	r.GET("/plant/:id/notebook", handlers.PlantNotebookHandler)
	r.GET("/notebook", handlers.AllPlantsNotebookHandler)
	r.GET("/plants/living", handlers.LivingPlantsHandler)
	r.GET("/plants/harvested", handlers.HarvestedPlantsHandler)
	r.GET("/plants/dead", handlers.DeadPlantsHandler)
	r.GET("/plants/by-strain/:strainID", handlers.PlantsByStrainHandler)
	r.GET("/sensorData", handlers.ChartHandler)
	r.GET("/sensors/grouped", func(c *gin.Context) {
		groupedSensors := handlers.GetGroupedSensorsWithLatestReading(
			handlers.DBFromContext(c), handlers.SensorCacheServiceFromContext(c))
		c.JSON(http.StatusOK, groupedSensors)
	})
	r.GET("/strains/:id", handlers.GetStrainHandler)
	r.GET("/strains/in-stock", handlers.InStockStrainsHandler)
	r.GET("/strains/all", handlers.AllStrainsHandler)
	r.GET("/strains/out-of-stock", handlers.OutOfStockStrainsHandler)
	r.GET("/strains/wanted", handlers.WantedStrainsHandler)
	r.POST("/decorateImage", utils.DecorateImageHandler)
	r.GET("/streams", handlers.GetStreamsByZoneHandler)

	// Lineage (public read)
	r.GET("/strains/:id/lineage", handlers.GetLineageHandler)
	r.GET("/strains/:id/descendants", handlers.GetDescendantsHandler)
	r.GET("/strains/lookup", handlers.LookupStrainByName)
}

func AddProtectedApiRoutes(r *gin.RouterGroup) {
	// API endpoints
	r.POST("/plants", handlers.AddPlant)
	r.POST("/plant", func(c *gin.Context) { handlers.UpdatePlant(c) })
	r.POST("/plant/status", handlers.UpdatePlantStatus)
	r.DELETE("/plant/delete/:id", handlers.DeletePlant)
	r.POST("/plant/link-sensors", handlers.LinkSensorsToPlant)
	r.POST("/plantStatus/edit", handlers.EditStatus)
	r.DELETE("/plantStatus/delete/:id", handlers.DeleteStatus)
	r.POST("/plantMeasurement", func(c *gin.Context) { handlers.CreatePlantMeasurement(c) })
	r.POST("/plantMeasurement/edit", handlers.EditMeasurement)
	r.DELETE("/plantMeasurement/delete/:id", handlers.DeleteMeasurement)
	r.POST("/plantActivity", func(c *gin.Context) { handlers.CreatePlantActivity(c) })
	r.POST("/plantActivity/edit", handlers.EditActivity)
	r.DELETE("/plantActivity/delete/:id", handlers.DeleteActivity)
	r.PUT("/plant/:id/day/:date", handlers.SavePlantDayHandler)
	r.DELETE("/plant/:id/day/:date", handlers.DeletePlantDayHandler)
	r.POST("/plants/day", handlers.LogPlantsDayHandler)
	r.POST("/plant/:plantID/images/upload", handlers.UploadPlantImages)
	r.DELETE("/plant/images/:imageID/delete", handlers.DeletePlantImage)

	r.POST("/sensors/scanACI", handlers.ScanACInfinitySensors)
	r.POST("/sensors/scanEC", handlers.ScanEcoWittSensors)
	r.POST("/sensors/scanHA", handlers.ScanHomeAssistantSensors)
	r.POST("/sensors/edit", handlers.EditSensor)
	r.DELETE("/sensors/delete/:id", handlers.DeleteSensor)
	r.PUT("/sensors/:id", handlers.UpdateSensorHandler)
	r.POST("/sensors/add", handlers.AddScannedSensorHandler)
	r.GET("/sensors/series", handlers.SensorSeriesHandler)

	// Debug endpoint to dump raw AC Infinity API response
	r.GET("/sensors/dumpACI", handlers.DumpACInfinityJSON)

	r.POST("/strains", handlers.AddStrainHandler)

	r.PUT("/strains/:id", handlers.UpdateStrainHandler)
	r.DELETE("/strains/:id", handlers.DeleteStrainHandler)
	r.POST("/strains/:id/wanted", handlers.SetStrainWantedHandler)
	r.POST("/strains/:id/seeds", handlers.AddStrainSeedsHandler)
	r.POST("/strains/:id/seeds/use", handlers.UseStrainSeedsHandler)

	// CannaDB import (search + one-click import). Auth-gated so anonymous
	// users can't drive outbound calls against the shared per-IP rate budget.
	r.GET("/strains/cannadb/search", handlers.CannadbSearchHandler)
	r.POST("/strains/cannadb/import", handlers.CannadbImportHandler)

	// StrainCompass import (search + one-click import). Auth-gated for the
	// same reason as CannaDB above: anonymous users must not be able to
	// drive outbound calls against a shared per-key/per-IP rate budget.
	r.GET("/strains/straincompass/search", handlers.StraincompassSearchHandler)
	r.POST("/strains/straincompass/import", handlers.StraincompassImportHandler)

	// Save a reviewed import (see /strain/import/review).
	r.POST("/strains/import/save", handlers.SaveImportedStrainHandler)

	// Seed-pack (packaging) image: upload, remove, or accept/discard a held CannaDB offer.
	r.POST("/strains/:id/packaging-image", handlers.UploadPackagingImageHandler)
	r.DELETE("/strains/:id/packaging-image", handlers.DeletePackagingImageHandler)
	r.POST("/strains/:id/packaging-image/accept", handlers.AcceptPackagingImageHandler)
	r.POST("/strains/:id/packaging-image/discard", handlers.DiscardPackagingImageHandler)

	// Lineage (protected write)
	r.POST("/strains/:id/lineage", handlers.AddLineageHandler)
	r.PUT("/strains/:id/lineage", handlers.SetLineageHandler)
	r.PUT("/lineage/:lineageID", handlers.UpdateLineageHandler)
	r.DELETE("/lineage/:lineageID", handlers.DeleteLineageHandler)

	r.POST("/aci/login", handlers.ACILoginHandler)
	r.POST("/aci/forget", handlers.ACIForgetHandler)

	r.POST("/zones", handlers.AddZoneHandler)
	r.PUT("/zones/:id", handlers.UpdateZoneHandler)
	r.DELETE("/zones/:id", handlers.DeleteZoneHandler)

	r.POST("/statuses/:id/vpd", handlers.UpdateStatusVPDHandler)

	r.POST("/metrics", handlers.AddMetricHandler)
	r.GET("/metrics", handlers.GetMetricsHandler)
	r.PUT("/metrics/:id", handlers.UpdateMetricHandler)
	r.DELETE("/metrics/:id", handlers.DeleteMetricHandler)

	r.POST("/activities", handlers.AddActivityHandler)
	r.PUT("/activities/:id", handlers.UpdateActivityHandler)
	r.DELETE("/activities/:id", handlers.DeleteActivityHandler)
	r.POST("/streams", handlers.AddStreamHandler)
	r.PUT("/streams/:id", handlers.UpdateStreamHandler)
	r.DELETE("/streams/:id", handlers.DeleteStreamHandler)

	r.GET("/breeders/reference", handlers.BreederReferenceHandler)
	r.GET("/breeders/reference/status", handlers.BreederReferenceStatusHandler)
	r.POST("/breeders/reference/settings", handlers.BreederReferenceSettingsHandler)
	r.POST("/breeders/reference/refresh", handlers.BreederReferenceRefreshHandler)
	r.POST("/breeders", handlers.AddBreederHandler)
	r.PUT("/breeders/:id", handlers.UpdateBreederHandler)
	r.DELETE("/breeders/:id", handlers.DeleteBreederHandler)

	r.POST("/settings/upload-logo", handlers.UploadLogo)
	r.GET("/settings/logs", handlers.GetLogs)
	r.GET("/settings/logs/download", handlers.DownloadLogs)
	r.POST("/settings/backup/create", handlers.CreateBackup)
	r.GET("/settings/backup/status", handlers.GetBackupStatus)
	r.GET("/settings/backup/list", handlers.ListBackups)
	r.GET("/settings/backup/download/:name", handlers.DownloadBackup)
	r.DELETE("/settings/backup/:name", handlers.DeleteBackup)
	r.POST("/settings/backup/restore", handlers.ImportBackup)
	r.GET("/settings/backup/restore/status", handlers.GetRestoreStatus)
	r.GET("/settings/backup/sqlite/download", handlers.DownloadSQLiteDB)
	r.POST("/settings/backup/sqlite/upload", handlers.UploadSQLiteDB)
	r.POST("/record-multi-activity", handlers.RecordMultiPlantActivity)
	r.POST("/settings", handlers.SaveSettings)
	r.POST("/settings/homeassistant/test", handlers.TestHomeAssistantHandler)
}

// AddExternalApiRoutes External API endpoints
func AddExternalApiRoutes(r *gin.RouterGroup) {
	r.POST("/api/sensors/ingest", handlers.IngestRateLimitMiddleware(), handlers.IngestSensorData)
	r.GET("/api/overlay", handlers.IngestRateLimitMiddleware(), handlers.GetOverlayData)
}

func AddProtectedRoutes(r *gin.RouterGroup, version string) {
	r.GET("/plant/:id/edit", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/plant-edit.html", gin.H{
			"title":           "Edit Plant",
			"currentPath":     currentPath,
			"version":         version,
			"plant":           handlers.GetPlant(handlers.DBFromContext(c), c.Param("id")),
			"zones":           store.Zones(),
			"strains":         store.Strains(),
			"statuses":        store.Statuses(),
			"breeders":        store.Breeders(),
			"measurements":    store.Metrics(),
			"sensors":         handlers.GetSensors(handlers.DBFromContext(c)),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	// Review an import before saving: the Edit Strain page in review mode.
	r.GET("/strain/import/review", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		review := handlers.PrepareImportReview(c)
		c.HTML(http.StatusOK, "views/strain-edit.html", gin.H{
			"title":           "Review Strain Info",
			"currentPath":     currentPath,
			"version":         version,
			"strain":          review.Strain,
			"reviewMode":      true,
			"review":          review,
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/strain/:id/edit", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/strain-edit.html", gin.H{
			"title":           "Edit Strain",
			"currentPath":     currentPath,
			"version":         version,
			"strain":          handlers.GetStrain(handlers.DBFromContext(c), c.Param("id")),
			"packagingOffer":  handlers.PendingPackagingURL(c),
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/settings", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		db := handlers.DBFromContext(c)
		zonePlants, zoneSensors := handlers.ZoneCounts(db)
		c.HTML(http.StatusOK, "views/settings.html", gin.H{
			"zonePlants":      zonePlants,
			"zoneSensors":     zoneSensors,
			"title":           "Settings",
			"currentPath":     currentPath,
			"version":         version,
			"settings":        handlers.GetSettings(db),
			"integrations":    handlers.IntegrationStatuses(db, store),
			"activityIcons":   handlers.ActivityIcons,
			"activityColors":  handlers.ActivityColors,
			"zones":           store.Zones(),
			"statuses":        store.Statuses(),
			"sensors":         handlers.GetSensors(db),
			"metrics":         store.Metrics(),
			"plants":          handlers.GetLivingPlants(db),
			"activities":      store.Activities(),
			"breeders":        store.Breeders(),
			"streams":         handlers.GetStreams(db),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"dbDriver":        model.GetDriver(),
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	// Environment: charts per tent (Sensors in the menu).
	r.GET("/sensors", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/sensors.html", gin.H{
			"pageData":        handlers.SensorsPageData(c, "en_", lang, translations),
			"title":           "Environment",
			"currentPath":     currentPath,
			"version":         version,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	// Manage sensors: scan, tents, where each reading shows.
	r.GET("/sensors/manage", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/sensors-manage.html", gin.H{
			"pageData":        handlers.SensorsPageData(c, "sm_", lang, translations),
			"title":           "Manage sensors",
			"currentPath":     currentPath,
			"version":         version,
			"settings":        handlers.GetSettings(handlers.DBFromContext(c)),
			"sensors":         handlers.GetSensors(handlers.DBFromContext(c)),
			"zones":           store.Zones(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	// Activity log exports (auth-gated; unauthenticated users are redirected
	// to /login by AuthMiddleware).  Filters mirror the /activities page.
	r.GET("/activities/export/csv", handlers.ExportActivitiesCSV)
	r.GET("/activities/export/xlsx", handlers.ExportActivitiesXLSX)
}

// homeHasCamera reports whether the Home page shows a camera tile (the
// video player script is only loaded then).
func homeHasCamera(data gin.H) bool {
	tents, ok := data["tents"].([]handlers.HomeTent)
	if !ok {
		return false
	}
	for _, t := range tents {
		if len(t.Cameras) > 0 {
			return true
		}
	}
	return false
}
