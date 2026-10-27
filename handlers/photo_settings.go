package handlers

import (
	"database/sql"
	"strconv"

	"isley/config"
)

// Settings > Photos rows in the settings table.
const (
	PhotoCompressSetting = "photo_compress" // "1": compress new plant photos
	PhotoQualitySetting  = "photo_quality"  // JPEG quality, 50-90 in steps of 5
)

// PhotoQualities are the qualities Settings > Photos offers, best first.
var PhotoQualities = []int{90, 85, 80, 75, 70, 65, 60, 55, 50}

// ValidPhotoQuality reports whether q is one of PhotoQualities.
func ValidPhotoQuality(q int) bool {
	return q >= 50 && q <= 90 && q%5 == 0
}

// loadPhotoSettings fills the Store's photo settings from the database
// (part of LoadSettings).
func loadPhotoSettings(db *sql.DB, store *config.Store) {
	if v, err := GetSetting(db, PhotoCompressSetting); err == nil {
		store.SetPhotoCompress(v == "1")
	}
	q := config.DefaultPhotoQuality
	if v, err := GetSetting(db, PhotoQualitySetting); err == nil && v != "" {
		if n, err := strconv.Atoi(v); err == nil && ValidPhotoQuality(n) {
			q = n
		}
	}
	store.SetPhotoQuality(q)
}

// plant_images.compress_result values. NULL means never checked.
const (
	PhotoCompressed = "compressed" // replaced by a compressed copy; never compressed again
	PhotoKept       = "kept"       // still the original (saved too little, or not re-encodable)
)
