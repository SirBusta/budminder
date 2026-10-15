package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"isley/acinfinity"
	"isley/config"
	"isley/logger"
	"isley/secretbox"
)

// ACILoginHandler signs in to AC Infinity from the Settings window
// (POST /aci/login) and saves the token BudMinder polls with. With
// stay_signed_in, the email and the password (encrypted, see package
// secretbox) are kept so the watcher can sign in again by itself when AC
// Infinity logs BudMinder out; without it, any saved password is removed.
func ACILoginHandler(c *gin.Context) {
	var req struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		StaySignedIn bool   `json:"stay_signed_in"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.WithError(err).Error("Failed to bind JSON request")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": T(c, "api_invalid_request_payload")})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	token, err := acinfinity.Login(ctx, nil, "", req.Email, req.Password)
	var refused *acinfinity.RefusedError
	switch {
	case errors.As(err, &refused):
		logger.Log.WithField("code", refused.Code).WithField("message", refused.Msg).Error("AC Infinity refused the sign-in")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": refused.Msg})
		return
	case err != nil:
		logger.Log.WithError(err).Error("Failed to sign in to AC Infinity")
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": T(c, "api_failed_to_connect_aci")})
		return
	}

	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)
	UpdateSetting(db, store, "aci.token", token)
	// A fresh sign-in: forget the old refusal until the next poll says how it went.
	store.ClearIntegrationError("acinfinity")

	if req.StaySignedIn {
		if err := SaveACIPassword(db, store, BackupServiceFromContext(c).DataDir(), req.Email, req.Password); err != nil {
			logger.Log.WithError(err).Error("Couldn't save the AC Infinity password; staying signed in is off")
			ForgetACIPassword(db, store)
		}
	} else {
		ForgetACIPassword(db, store)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "stay_signed_in": req.StaySignedIn})
}

// ACIForgetHandler removes the saved AC Infinity email and password
// (POST /aci/forget); the current sign-in keeps working until it expires.
func ACIForgetHandler(c *gin.Context) {
	ForgetACIPassword(DBFromContext(c), ConfigStoreFromContext(c))
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// SaveACIPassword keeps the email and the password, encrypted with the key in
// <dataDir>/budminder.key (made on first use).
func SaveACIPassword(db *sql.DB, store *config.Store, dataDir, email, password string) error {
	key, err := secretbox.LoadOrCreateKey(filepath.Join(dataDir, secretbox.KeyFile))
	if err != nil {
		return err
	}
	sealed, err := secretbox.Seal(key, password)
	if err != nil {
		return err
	}
	if err := UpdateSetting(db, store, "aci.email", email); err != nil {
		return err
	}
	return UpdateSetting(db, store, "aci.password", sealed)
}

// ForgetACIPassword removes the saved AC Infinity email and password.
func ForgetACIPassword(db *sql.DB, store *config.Store) {
	UpdateSetting(db, store, "aci.email", "")
	UpdateSetting(db, store, "aci.password", "")
}

// NewACIRelogin returns the watcher's automatic sign-in: when AC Infinity
// refuses the saved token, it signs in with the saved email and password
// and saves the new token. attempted is false when no password is saved
// (staying signed in is off). baseURL "" means AC Infinity itself.
func NewACIRelogin(db *sql.DB, store *config.Store, dataDir, baseURL string) func(ctx context.Context) (attempted bool, err error) {
	return func(ctx context.Context) (bool, error) {
		email, _ := GetSetting(db, "aci.email")
		sealed, _ := GetSetting(db, "aci.password")
		if email == "" || sealed == "" {
			return false, nil
		}
		key, err := secretbox.LoadKey(filepath.Join(dataDir, secretbox.KeyFile))
		if errors.Is(err, secretbox.ErrNoKey) {
			return true, errors.New("the saved password can't be read on this computer (no key file); sign in again")
		}
		var password string
		if err == nil {
			password, err = secretbox.Open(key, sealed)
		}
		if err != nil {
			return true, errors.New("the saved password can't be read; sign in again")
		}
		token, err := acinfinity.Login(ctx, nil, baseURL, email, password)
		if err != nil {
			return true, err
		}
		UpdateSetting(db, store, "aci.token", token)
		logger.Log.Info("Signed in to AC Infinity again automatically")
		return true, nil
	}
}
