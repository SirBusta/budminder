package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"isley/utils"
)

// First run: a new install has no account, and its first page asks the
// person to choose a username and password (views/setup.html). Installs
// from before 1.5.6 that still have the old default sign-in (admin /
// budminder, force_password_change) get the same page, since anyone who
// read the README could sign in to them.

// setupMu makes two people submitting the setup page at the same moment
// create one account, not two in turn.
var setupMu sync.Mutex

// NeedsSetup reports whether the install has no account of its own yet.
func NeedsSetup(db *sql.DB) bool {
	user, _ := GetSetting(db, "auth_username")
	if user == "" {
		return true
	}
	force, _ := GetSetting(db, "force_password_change")
	return force == "true"
}

// ValidUsername checks a chosen username: 1 to 64 characters after
// trimming, no control characters. Returns the trimmed name and whether
// it's acceptable.
func ValidUsername(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return name, false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return name, false
		}
	}
	return name, true
}

// startSession signs the browser in: a fresh session (no fixation), a new
// CSRF token, 14 days when "remember" is ticked.
func startSession(c *gin.Context, remember bool, version string) {
	session := sessions.Default(c)
	session.Clear()
	opts := sessions.Options{Path: "/", HttpOnly: true, Secure: SecureCookies, SameSite: http.SameSiteLaxMode}
	if remember {
		opts.MaxAge = 14 * 24 * 60 * 60
	}
	session.Options(opts)
	session.Set("csrf_token", GenerateCSRFToken())
	session.Set("logged_in", true)
	session.Set("session_version", version)
	_ = session.Save()
}

func renderSetup(c *gin.Context, status int, errKey, username string) {
	lang := utils.GetLanguage(c)
	csrfToken, _ := c.Get("csrf_token")
	h := gin.H{
		"lcl":             utils.TranslationService.GetTranslations(lang),
		"languages":       utils.AvailableLanguages,
		"currentLanguage": lang,
		"csrfToken":       csrfToken,
		"cspNonce":        c.GetString("cspNonce"),
		"Username":        username,
	}
	if errKey != "" {
		h["Error"] = T(c, errKey)
	}
	c.HTML(status, "views/setup.html", h)
}

// SetupPageHandler: GET /setup, the "create your account" page. Once an
// account exists it's the sign-in page's turn.
func SetupPageHandler(c *gin.Context) {
	if !NeedsSetup(DBFromContext(c)) {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	renderSetup(c, http.StatusOK, "", "")
}

// HandleSetup: POST /setup. Creates the account and signs the person in.
func HandleSetup(c *gin.Context) {
	username, ok := ValidUsername(c.PostForm("username"))
	password := c.PostForm("password")
	switch {
	case !ok:
		renderSetup(c, http.StatusBadRequest, "setup_err_username", username)
		return
	case password != c.PostForm("confirm_password"):
		renderSetup(c, http.StatusBadRequest, "setup_err_mismatch", username)
		return
	case ValidatePasswordComplexity(password) != "":
		renderSetup(c, http.StatusBadRequest, "setup_err_short", username)
		return
	}

	setupMu.Lock()
	defer setupMu.Unlock()
	db := DBFromContext(c)
	if !NeedsSetup(db) {
		c.Redirect(http.StatusFound, "/login") // someone else just finished it
		return
	}
	hashed, err := utils.HashPassword(password)
	if err != nil {
		renderSetup(c, http.StatusInternalServerError, "setup_err_failed", username)
		return
	}
	store := ConfigStoreFromContext(c)
	version := fmt.Sprintf("%d", time.Now().UnixNano())
	// Order matters: setup stays open until auth_username is written (and,
	// for a pre-1.5.6 install, until force_password_change is), so a write
	// that fails part way leaves setup to retry, never a username with no
	// password.
	for _, kv := range [][2]string{{"auth_password", hashed}, {"session_version", version}, {"auth_username", username}, {"force_password_change", "false"}} {
		if err := UpdateSetting(db, store, kv[0], kv[1]); err != nil {
			renderSetup(c, http.StatusInternalServerError, "setup_err_failed", username)
			return
		}
	}
	startSession(c, c.PostForm("remember") != "", version)
	c.Redirect(http.StatusFound, "/")
}

func renderAccount(c *gin.Context, status int, errKey string) {
	lang := utils.GetLanguage(c)
	csrfToken, _ := c.Get("csrf_token")
	username, _ := GetSetting(DBFromContext(c), "auth_username")
	h := gin.H{
		"lcl":             utils.TranslationService.GetTranslations(lang),
		"languages":       utils.AvailableLanguages,
		"currentLanguage": lang,
		"csrfToken":       csrfToken,
		"cspNonce":        c.GetString("cspNonce"),
		"Username":        username,
	}
	if errKey != "" {
		h["Error"] = T(c, errKey)
	}
	c.HTML(status, "views/change-password.html", h)
}

// AccountPageHandler: GET /change-password, change the username and/or
// password (Settings > Account).
func AccountPageHandler(c *gin.Context) {
	renderAccount(c, http.StatusOK, "")
}

// HandleChangePassword: POST /change-password. The current password is
// required; a blank new password keeps the current one. A new password
// signs out every other browser.
func HandleChangePassword(c *gin.Context) {
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)
	hash, _ := GetSetting(db, "auth_password")
	if !utils.CheckPasswordHash(c.PostForm("current_password"), hash) {
		renderAccount(c, http.StatusBadRequest, "account_err_current")
		return
	}
	username, ok := ValidUsername(c.PostForm("username"))
	if !ok {
		renderAccount(c, http.StatusBadRequest, "setup_err_username")
		return
	}
	newPassword := c.PostForm("new_password")
	if newPassword != "" {
		if newPassword != c.PostForm("confirm_password") {
			renderAccount(c, http.StatusBadRequest, "setup_err_mismatch")
			return
		}
		if ValidatePasswordComplexity(newPassword) != "" {
			renderAccount(c, http.StatusBadRequest, "setup_err_short")
			return
		}
	}

	if err := UpdateSetting(db, store, "auth_username", username); err != nil {
		renderAccount(c, http.StatusInternalServerError, "setup_err_failed")
		return
	}
	if newPassword != "" {
		hashed, err := utils.HashPassword(newPassword)
		if err != nil {
			renderAccount(c, http.StatusInternalServerError, "setup_err_failed")
			return
		}
		version := fmt.Sprintf("%d", time.Now().UnixNano())
		if UpdateSetting(db, store, "auth_password", hashed) != nil || UpdateSetting(db, store, "session_version", version) != nil {
			renderAccount(c, http.StatusInternalServerError, "setup_err_failed")
			return
		}
		// Only this browser gets the new version; every other one is signed out.
		session := sessions.Default(c)
		session.Set("session_version", version)
		_ = session.Save()
	}
	c.Redirect(http.StatusFound, "/settings")
}
