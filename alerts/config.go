package alerts

import (
	"database/sql"
	"errors"
)

// SettingLanguage is the language emails are written in: the one in use when
// the Email settings were last saved, since there's no install-wide language.
const SettingLanguage = "alert_language"

// LoadLanguage reads the language emails are written in.
func LoadLanguage(db *sql.DB) (string, error) {
	var v string
	err := db.QueryRow("SELECT value FROM settings WHERE name = $1", SettingLanguage).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) || err == nil && v == "" {
		return "en", nil
	}
	return v, err
}
