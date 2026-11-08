package handlers

// Vivosun (Beta): sign-in from Settings, Scan Vivosun on Manage sensors and
// the raw-data download. The polling itself is in watcher.PollVivosun.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/config"
	"isley/logger"
	"isley/secretbox"
	"isley/vivosun"
)

// Vivosun settings rows. The tokens are kept like AC Infinity's; the
// password only with Stay signed in, encrypted (see package secretbox).
const (
	vsSettingEnabled  = "vivosun.enabled"
	vsSettingAccess   = "vivosun.access_token"
	vsSettingLogin    = "vivosun.login_token"
	vsSettingEmail    = "vivosun.email"
	vsSettingPassword = "vivosun.password"
)

// maxVivosunCredential bounds the email and password a sign-in accepts.
const maxVivosunCredential = 256

// vsScanWindow is how far back a scan looks for each device's newest reading.
const vsScanWindow = 15 * time.Minute

func vivosunClient(store *config.Store) *vivosun.Client {
	return &vivosun.Client{HTTP: httpClient, BaseURL: store.VSBaseURL()}
}

func vivosunTokens(store *config.Store) vivosun.Tokens {
	a, l := store.VSTokens()
	return vivosun.Tokens{Access: a, Login: l}
}

// VivosunLoginHandler signs in to Vivosun from the Settings window
// (POST /vivosun/login), saves the tokens and turns reading Vivosun on.
// With stay_signed_in the email and the encrypted password are kept so the
// watcher can sign in again by itself; without it, any saved one is removed.
func VivosunLoginHandler(c *gin.Context) {
	var req struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		StaySignedIn bool   `json:"stay_signed_in"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" || len(req.Email) > maxVivosunCredential || len(req.Password) > maxVivosunCredential {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	tk, err := vivosunClient(store).Login(ctx, req.Email, req.Password)
	var refused *vivosun.RefusedError
	switch {
	case errors.As(err, &refused):
		logger.Log.WithField("code", refused.Code).WithField("message", refused.Msg).Warn("Vivosun refused the sign-in")
		msg := refused.Msg
		if msg == "" {
			msg = T(c, "api_vivosun_refused")
		}
		apiBadRequest(c, msg)
		return
	case err != nil:
		logger.Log.WithError(err).Error("Failed to sign in to Vivosun")
		apiError(c, http.StatusBadGateway, "api_vivosun_unreachable")
		return
	}

	if err := saveVivosunTokens(db, store, tk); err != nil {
		logger.Log.WithError(err).Error("Failed to save the Vivosun sign-in")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	if err := UpdateSetting(db, store, vsSettingEnabled, "1"); err == nil {
		store.SetVSEnabled(1)
	}
	store.ClearIntegrationError(vivosun.Source)

	if req.StaySignedIn {
		if err := saveVivosunPassword(db, store, BackupServiceFromContext(c).DataDir(), req.Email, req.Password); err != nil {
			logger.Log.WithError(err).Error("Couldn't save the Vivosun password; staying signed in is off")
			ForgetVivosunPassword(db, store)
			req.StaySignedIn = false
		}
	} else {
		ForgetVivosunPassword(db, store)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "stay_signed_in": req.StaySignedIn})
}

// VivosunForgetHandler removes the saved Vivosun email and password
// (POST /vivosun/forget); the current sign-in keeps working until it expires.
func VivosunForgetHandler(c *gin.Context) {
	ForgetVivosunPassword(DBFromContext(c), ConfigStoreFromContext(c))
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func saveVivosunTokens(db *sql.DB, store *config.Store, tk vivosun.Tokens) error {
	if err := UpdateSetting(db, store, vsSettingAccess, tk.Access); err != nil {
		return err
	}
	if err := UpdateSetting(db, store, vsSettingLogin, tk.Login); err != nil {
		return err
	}
	store.SetVSTokens(tk.Access, tk.Login)
	return nil
}

func saveVivosunPassword(db *sql.DB, store *config.Store, dataDir, email, password string) error {
	key, err := secretbox.LoadOrCreateKey(filepath.Join(dataDir, secretbox.KeyFile))
	if err != nil {
		return err
	}
	sealed, err := secretbox.Seal(key, password)
	if err != nil {
		return err
	}
	if err := UpdateSetting(db, store, vsSettingEmail, email); err != nil {
		return err
	}
	return UpdateSetting(db, store, vsSettingPassword, sealed)
}

// ForgetVivosunPassword removes the saved Vivosun email and password.
func ForgetVivosunPassword(db *sql.DB, store *config.Store) {
	UpdateSetting(db, store, vsSettingEmail, "")
	UpdateSetting(db, store, vsSettingPassword, "")
}

// NewVivosunRelogin returns the watcher's automatic Vivosun sign-in, used
// when Vivosun refuses the saved tokens. attempted is false when no password
// is saved (Stay signed in is off).
func NewVivosunRelogin(db *sql.DB, store *config.Store, dataDir string) func(ctx context.Context) (attempted bool, err error) {
	return func(ctx context.Context) (bool, error) {
		email, _ := GetSetting(db, vsSettingEmail)
		sealed, _ := GetSetting(db, vsSettingPassword)
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
		tk, err := vivosunClient(store).Login(ctx, email, password)
		if err != nil {
			return true, err
		}
		if err := saveVivosunTokens(db, store, tk); err != nil {
			return true, err
		}
		logger.Log.Info("Signed in to Vivosun again automatically")
		return true, nil
	}
}

// vivosunError answers a failed Vivosun request: sign-in expired, can't
// reach Vivosun, or Vivosun's own refusal.
func vivosunError(c *gin.Context, err error) {
	var refused *vivosun.RefusedError
	switch {
	case errors.Is(err, vivosun.ErrAuth):
		ConfigStoreFromContext(c).ReportIntegration(vivosun.Source, "auth", err.Error(), time.Now())
		apiBadRequest(c, "sm_vs_state_auth")
	case errors.As(err, &refused):
		apiError(c, http.StatusBadGateway, T(c, "api_vivosun_refused")+" "+refused.Msg)
	default:
		apiError(c, http.StatusBadGateway, "api_vivosun_unreachable")
	}
}

// ScanVivosunSensors lists the readings on the account's Vivosun devices
// that aren't added yet (POST /sensors/scanVivosun, preview only: each one
// is added with POST /sensors/add).
func ScanVivosunSensors(c *gin.Context) {
	store := ConfigStoreFromContext(c)
	tk := vivosunTokens(store)
	if !tk.Set() {
		apiBadRequest(c, "sm_vs_scan_signin")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	found, err := vivosunFound(ctx, vivosunClient(store), tk, store, time.Now())
	if err != nil {
		logger.Log.WithError(err).Warn("Scan Vivosun failed")
		vivosunError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sensors": newSensorsOnly(DBFromContext(c), found)})
}

// vivosunFound lists what each device's newest reading row offers. Devices
// with no recent row (offline, or no sensors) offer nothing; a device whose
// readings can't be fetched is skipped, unless the sign-in was refused.
func vivosunFound(ctx context.Context, client *vivosun.Client, tk vivosun.Tokens, store *config.Store, now time.Time) ([]foundSensor, error) {
	devs, err := client.Devices(ctx, tk)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	seen := map[string]int{}
	found := []foundSensor{}
	for _, d := range devs {
		names[d.ID] = d.Name
		seen[d.Name]++
		rows, err := client.Readings(ctx, tk, d, now.Add(-vsScanWindow), now)
		if errors.Is(err, vivosun.ErrAuth) {
			return nil, err
		}
		if err != nil {
			logger.Log.WithError(err).WithField("device", d.Name).Warn("Couldn't read a Vivosun device")
			continue
		}
		if len(rows) == 0 {
			continue
		}
		newest := rows[len(rows)-1].Values
		// Two devices with one name: the second one's readings are numbered.
		num := ""
		if seen[d.Name] > 1 {
			num = fmt.Sprintf(" (%d)", seen[d.Name])
		}
		for _, o := range vivosun.Offers(d.Model, newest) {
			v, home := o.Value, o.Home
			found = append(found, foundSensor{Source: vivosun.Source, Device: d.ID, Type: o.Type,
				Name: d.Name + " " + o.Suffix + num, Unit: o.Unit, Value: &v, Note: o.Note, Home: &home, More: o.More})
		}
	}
	store.SetVSDeviceNames(names)
	return found, nil
}

// DumpVivosunJSON returns, for troubleshooting, what Vivosun sends about the
// account's devices with the last hour of readings, account details always
// hidden (GET /sensors/dumpVivosun).
func DumpVivosunJSON(c *gin.Context) {
	store := ConfigStoreFromContext(c)
	tk := vivosunTokens(store)
	if !tk.Set() {
		apiBadRequest(c, "sm_vs_scan_signin")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	client := vivosunClient(store)
	env, err := client.DevicesRaw(ctx, tk)
	if err != nil {
		vivosunError(c, err)
		return
	}
	devs, _ := client.Devices(ctx, tk)
	now := time.Now()
	readings := []any{}
	for _, d := range devs {
		rec := map[string]any{"device": d.Name}
		rows, err := client.ReadingsRaw(ctx, tk, d, now.Add(-time.Hour), now)
		if err != nil {
			rec["error"] = err.Error()
		} else {
			rec["row_count"] = len(rows)
			rec["rows"] = rows
		}
		readings = append(readings, rec)
	}
	email, _ := GetSetting(DBFromContext(c), vsSettingEmail)
	out := map[string]any{
		"saved_at":  now.UTC().Format(time.RFC3339),
		"note":      "Account details hidden. Readings are x100; -6666 = no sensor.",
		"devices":   vivosun.Hide(env["data"], email),
		"readings":  vivosun.Hide(readings, email),
		"budminder": Version,
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		apiInternalError(c, "api_error_reading_response")
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", b)
}
