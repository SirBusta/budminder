package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"isley/env"
	"isley/logger"
	"isley/utils"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// SecureCookies controls the Secure flag on session cookies.
// Set via the BUDMINDER_SECURE_COOKIES environment variable (or ISLEY_SECURE_COOKIES; see package env).
var SecureCookies = strings.EqualFold(env.Get("SECURE_COOKIES"), "true")

// ---------------------------------------------------------------------------
// CSRF token generation
// ---------------------------------------------------------------------------

// GenerateCSRFToken creates a cryptographically random hex token.
func GenerateCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		logger.Log.WithError(err).Error("Failed to generate CSRF token")
		return ""
	}
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Password validation
// ---------------------------------------------------------------------------

// ValidatePasswordComplexity checks that a password meets minimum security
// requirements. Returns an empty string on success or an error message.
func ValidatePasswordComplexity(password string) string {
	if len(password) < MinPasswordLength {
		return "Password must be at least 8 characters long"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Auth handlers
// ---------------------------------------------------------------------------

// HandleLogin processes the POST /login form submission. An install with
// no account of its own yet goes to the setup page instead (NeedsSetup).
func HandleLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	remember := c.PostForm("remember")

	db := DBFromContext(c)
	if NeedsSetup(db) {
		c.Redirect(http.StatusFound, "/setup")
		return
	}

	storedUsername, _ := GetSetting(db, "auth_username")
	storedPasswordHash, _ := GetSetting(db, "auth_password")

	if username != storedUsername || !utils.CheckPasswordHash(password, storedPasswordHash) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		csrfToken, _ := c.Get("csrf_token")
		c.HTML(http.StatusUnauthorized, "views/login.html", gin.H{
			"Error":           "Invalid username or password",
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       csrfToken,
		})
		return
	}

	// SECURITY: a fresh session (no fixation) with a new CSRF token and the
	// current session version, so a later password change can sign it out.
	next := pageAfterLogin(sessions.Default(c).Get(afterLoginKey))
	sessionVersion, _ := GetSetting(db, "session_version")
	startSession(c, remember == "on" || remember == "true", sessionVersion)
	c.Redirect(http.StatusFound, next)
}

// afterLoginKey (in the session) is the page someone was opening when they
// were sent to sign in, e.g. /phone from the tray icon; signing in goes there.
const afterLoginKey = "after_login"

// rememberPage keeps the page being opened for after sign-in: only a page a
// browser navigates to (GET, wants HTML), not a background request.
func rememberPage(c *gin.Context, session sessions.Session) {
	if c.Request.Method == http.MethodGet && strings.Contains(c.GetHeader("Accept"), "text/html") {
		session.Set(afterLoginKey, c.Request.URL.RequestURI())
		_ = session.Save()
	}
}

// pageAfterLogin is the remembered page, or / when there's none. Only a path
// on this site counts ("//host" and "/\host" would leave it).
func pageAfterLogin(v any) string {
	p, _ := v.(string)
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") {
		return "/"
	}
	return p
}

// HandleLogout clears the session and redirects to the login page.
func HandleLogout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/login")
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

// AuthMiddlewareApi returns middleware that validates either an X-API-KEY
// header or an active session before allowing access to API routes.
func AuthMiddlewareApi() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-KEY")
		session := sessions.Default(c)
		loggedIn := session.Get("logged_in")

		if apiKey != "" {
			// Get stored (hashed) API key from settings
			db := DBFromContext(c)

			var storedAPIKey string
			err := db.QueryRow("SELECT value FROM settings WHERE name = 'api_key'").Scan(&storedAPIKey)
			if err != nil {
				logger.Log.WithError(err).Error("Error retrieving API key from database")
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Could not validate API key",
				})
				return
			}

			// Validate the incoming key against the stored hash.
			// CheckAPIKey handles bcrypt (preferred), legacy SHA-256, and
			// plaintext matches for backward compatibility.
			match, legacy := CheckAPIKey(apiKey, storedAPIKey)
			if !match {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "Invalid API key",
				})
				return
			}

			// Auto-upgrade legacy keys (SHA-256 or plaintext) to bcrypt.
			if legacy {
				if newHash := HashAPIKey(apiKey); newHash != "" {
					UpdateSetting(db, ConfigStoreFromContext(c), "api_key", newHash)
					logger.Log.Info("Auto-upgraded legacy API key to bcrypt")
				}
			}

		} else if loggedIn == nil || !loggedIn.(bool) {
			// If no API key is provided, check session for logged_in status
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "API key or session required",
			})
			return
		}

		c.Next()
	}
}

// AuthMiddleware returns middleware that enforces session-based authentication
// for browser routes. Redirects to /login if not authenticated or if the
// session version is stale (e.g. after a password change).
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		loggedIn := session.Get("logged_in")

		if NeedsSetup(DBFromContext(c)) {
			// No account of its own yet (or the old default sign-in): an
			// earlier sign-in no longer counts, and the setup page comes
			// first. The rest of the session (its CSRF token) stays, or a
			// background request would void the setup form being filled in.
			if loggedIn != nil {
				session.Delete("logged_in")
				_ = session.Save()
			}
			c.Redirect(http.StatusFound, "/setup")
			c.Abort()
			return
		}
		if loggedIn == nil || !loggedIn.(bool) {
			rememberPage(c, session)
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// SECURITY: Validate session version to enforce invalidation on password change
		db := DBFromContext(c)
		dbVersion, _ := GetSetting(db, "session_version")
		sessVersion, _ := session.Get("session_version").(string)
		if dbVersion != "" && sessVersion != dbVersion {
			session.Clear()
			session.Save()
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Next()
	}
}
