package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/utils"
)

// Stock tracking on a strain: "wanted" (on the user's wish list) and
// seeds_added_on (when the current seeds were added to stock). Both follow
// the seed count, so every save that sets seed_count goes through
// resolveStrainStock:
//   - seeds in stock: wanted is cleared (you have it now). The date is the
//     one the user entered; when they left it empty and the strain is being
//     (re)stocked from zero, it's today; otherwise it stays as it was.
//   - no seeds: the date is cleared (nothing to date); wanted is the user's.

// strainStockPrev is what's stored before a save; prevCount < 0 means the
// strain is new.
type strainStockPrev struct {
	Count   int
	Wanted  bool
	AddedOn string
}

var newStrainStock = strainStockPrev{Count: -1}

// strainStock is what to store: wanted as 0/1, seeds_added_on as a string or
// nil (NULL).
type strainStock struct {
	Wanted  int
	AddedOn any
}

var errInvalidSeedsAddedOn = errors.New("api_invalid_seeds_added_on")

// loadStrainStock reads a strain's current stock fields.
func loadStrainStock(q interface {
	QueryRow(string, ...any) *sql.Row
}, id int) (strainStockPrev, error) {
	var p strainStockPrev
	var wanted int
	err := q.QueryRow("SELECT seed_count, wanted, coalesce(seeds_added_on, '') FROM strain WHERE id = $1", id).
		Scan(&p.Count, &wanted, &p.AddedOn)
	p.Wanted = wanted == 1
	return p, err
}

// resolveStrainStock applies the rules above. wanted and addedOn may be nil
// ("not sent": keep what's stored).
func resolveStrainStock(seedCount int, wanted *bool, addedOn *string, prev strainStockPrev, today string) (strainStock, error) {
	var out strainStock
	date := ""
	if addedOn != nil {
		date = strings.TrimSpace(*addedOn)
		if date != "" {
			if _, err := time.Parse(utils.LayoutDate, date); err != nil {
				return out, errInvalidSeedsAddedOn
			}
		}
	}

	if seedCount <= 0 {
		w := prev.Wanted
		if wanted != nil {
			w = *wanted
		}
		if w {
			out.Wanted = 1
		}
		return out, nil
	}

	restocking := prev.Count <= 0
	switch {
	case date != "":
		out.AddedOn = date
	case restocking:
		out.AddedOn = today
	case addedOn == nil && prev.AddedOn != "":
		out.AddedOn = prev.AddedOn
	}
	return out, nil
}

// StockToday is today's date (YYYY-MM-DD) in the configured timezone, for
// pages that count days (the strain page's Day N) the same way.
func StockToday(c *gin.Context) string { return stockToday(c) }

// stockToday is today's date for an automatic seeds_added_on, in the
// configured timezone when one is set.
func stockToday(c *gin.Context) string {
	now := time.Now()
	if tz := ConfigStoreFromContext(c).Timezone(); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			now = now.In(loc)
		}
	}
	return now.Format(utils.LayoutDate)
}

// SetStrainWantedHandler adds a strain to, or removes it from, the wish
// list. A strain with seeds in stock can't be wanted.
// POST /strains/:id/wanted  {"wanted": true}
func SetStrainWantedHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	var req struct {
		Wanted bool `json:"wanted"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	db := DBFromContext(c)
	wanted := 0
	if req.Wanted {
		wanted = 1
	}
	res, err := db.Exec("UPDATE strain SET wanted = CASE WHEN seed_count > 0 THEN 0 ELSE $1 END WHERE id = $2", wanted, id)
	if err == nil {
		if n, _ := res.RowsAffected(); n == 0 {
			apiNotFound(c, "api_strain_not_found")
			return
		}
	}
	var stored int
	if err == nil {
		err = db.QueryRow("SELECT wanted FROM strain WHERE id = $1", id).Scan(&stored)
	}
	if err != nil {
		logger.Log.WithError(err).Error("Failed to set wanted")
		apiInternalError(c, "api_failed_to_update_strain")
		return
	}
	ConfigStoreFromContext(c).SetStrains(GetStrains(db))
	c.JSON(http.StatusOK, gin.H{"wanted": stored == 1})
}

// AddStrainSeedsHandler adds seeds to a strain's stock (the Strains page's
// "+ Seeds"): the count goes up, and the type, storage spot and date are
// set. Stock rules as everywhere else (resolveStrainStock): Wanted is
// cleared, a strain restocked from zero is dated today unless a date is
// given, and existing stock keeps its date unless a new one is given.
// POST /strains/:id/seeds  {"count": 5, "seed_type": "feminized",
// "seeds_added_on": "2026-10-10", "seed_location": "Jar 1"}
func AddStrainSeedsHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "AddStrainSeedsHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	var req struct {
		Count        int     `json:"count"`
		SeedType     *string `json:"seed_type"`
		SeedsAddedOn *string `json:"seeds_added_on"`
		SeedLocation *string `json:"seed_location"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Count < 1 || req.Count > 100000 {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	if req.SeedType != nil {
		v, ok := normalizeSeedType(*req.SeedType)
		if !ok {
			apiBadRequest(c, "api_invalid_seed_type")
			return
		}
		req.SeedType = &v
	}
	if req.SeedLocation != nil {
		v := strings.TrimSpace(*req.SeedLocation)
		if err := utils.ValidateStringLength("seed_location", v, utils.MaxNameLength); err != nil {
			apiBadRequest(c, err.Error())
			return
		}
		req.SeedLocation = &v
	}

	db := DBFromContext(c)
	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()
	prev, err := loadStrainStock(tx, id)
	if err == sql.ErrNoRows {
		apiNotFound(c, "api_strain_not_found")
		return
	}
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to load strain stock")
		apiInternalError(c, "api_failed_to_update_strain")
		return
	}
	count := prev.Count + req.Count
	if prev.Count < 0 {
		count = req.Count
	}
	notWanted := false
	stock, err := resolveStrainStock(count, &notWanted, req.SeedsAddedOn, prev, stockToday(c))
	if err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE strain SET seed_count = $1, wanted = $2, seeds_added_on = $3,
		seed_type = COALESCE($4, seed_type), seed_location = COALESCE($5, seed_location) WHERE id = $6`,
		count, stock.Wanted, stock.AddedOn, req.SeedType, req.SeedLocation, id); err != nil {
		fieldLogger.WithError(err).Error("Failed to add seeds")
		apiInternalError(c, "api_failed_to_update_strain")
		return
	}
	if err := tx.Commit(); err != nil {
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}
	ConfigStoreFromContext(c).SetStrains(GetStrains(db))
	fieldLogger.WithFields(map[string]interface{}{"strain_id": id, "added": req.Count, "seed_count": count}).Info("Seeds added")
	c.JSON(http.StatusOK, gin.H{"seed_count": count, "seeds_added_on": stock.AddedOn, "wanted": stock.Wanted == 1})
}

// StrainsPageData is the Strains page's data attribute: who's logged in,
// today's date (the configured timezone, as for seed dates), the language,
// and the page's own translated strings ("sp_" keys).
func StrainsPageData(c *gin.Context, loggedIn bool, lang string, translations map[string]string) gin.H {
	t := map[string]string{}
	for k, v := range translations {
		if strings.HasPrefix(k, "sp_") {
			t[k] = v
		}
	}
	return gin.H{"logged_in": loggedIn, "today": stockToday(c), "lang": lang, "t": t}
}

// UseStrainSeedsHandler takes seeds out of a strain's stock (the strain
// page's "−", e.g. a seed given away or one that didn't pop). The count
// never goes below zero; at zero the stock date is cleared like everywhere
// else (resolveStrainStock). Starting a grow uses its seed on its own.
// POST /strains/:id/seeds/use  {"count": 1}
func UseStrainSeedsHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	var req struct {
		Count int `json:"count"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Count < 1 || req.Count > 100000 {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	db := DBFromContext(c)
	tx, err := db.Begin()
	if err != nil {
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()
	prev, err := loadStrainStock(tx, id)
	if err == sql.ErrNoRows {
		apiNotFound(c, "api_strain_not_found")
		return
	}
	if err != nil {
		apiInternalError(c, "api_failed_to_update_strain")
		return
	}
	count := max(0, prev.Count-req.Count)
	keep := prev.AddedOn
	stock, err := resolveStrainStock(count, nil, &keep, prev, stockToday(c))
	if err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE strain SET seed_count = $1, wanted = $2, seeds_added_on = $3 WHERE id = $4`,
		count, stock.Wanted, stock.AddedOn, id); err != nil {
		logger.Log.WithError(err).Error("Failed to use seeds")
		apiInternalError(c, "api_failed_to_update_strain")
		return
	}
	if err := tx.Commit(); err != nil {
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}
	ConfigStoreFromContext(c).SetStrains(GetStrains(db))
	c.JSON(http.StatusOK, gin.H{"seed_count": count, "seeds_added_on": stock.AddedOn, "wanted": stock.Wanted == 1})
}
