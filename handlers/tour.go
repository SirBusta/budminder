package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// The first-run tour (web/static/js/tour.js, common/tour.html): four tips
// with pictures and short clips, shown over the page. A new install (no
// strains, no plants) is asked once whether it wants the tour; the answer,
// either way, is kept in the "tour.answered" setting. Anyone can start it
// again from the Help manual. The demo asks each visitor instead and keeps
// the answer in their browser (tour.js), so nothing is saved there.

const tourAnsweredSetting = "tour.answered"

// TourPageData is what common/tour.html hands to tour.js: the tour's text
// (the "tour_" translations) and whether to ask about the tour on this page
// (askable: a signed-in visit to Today; the demo asks there too, see tour.js).
func TourPageData(c *gin.Context, askable bool, translations map[string]string) gin.H {
	t := map[string]string{}
	for k, v := range translations {
		if strings.HasPrefix(k, "tour_") {
			t[k] = v
		}
	}
	return gin.H{"t": t, "askable": askable, "ask": askable && tourShouldAsk(c)}
}

// tourShouldAsk: not answered yet, and nothing added yet.
func tourShouldAsk(c *gin.Context) bool {
	db := DBFromContext(c)
	answered, err := GetSetting(db, tourAnsweredSetting)
	if err != nil || answered != "" {
		return false
	}
	var strains, plants int
	if err := db.QueryRow("SELECT COUNT(*) FROM strain").Scan(&strains); err != nil {
		return false
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM plant").Scan(&plants); err != nil {
		return false
	}
	return strains == 0 && plants == 0
}

// TourAnsweredHandler records that the tour question was answered
// (POST /tour/answered), so it isn't asked again.
func TourAnsweredHandler(c *gin.Context) {
	if err := UpdateSetting(DBFromContext(c), nil, tourAnsweredSetting, "1"); err != nil {
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	apiOK(c, "api_settings_saved")
}
