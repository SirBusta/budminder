package handlers

// Settings > Alerts: the Email card (alerts/email.go sends).

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/alerts"
	"isley/logger"
	"isley/utils"
)

// AlertSettingsData is what the Alerts section of Settings renders with.
// The password itself never goes to the page.
type AlertSettingsData struct {
	Email       alerts.Email
	To          string // recipients, comma-separated
	PasswordSet bool
	EmailReady  bool // on and complete, so Send email can send
	Climate     alertClimateRequest
	Ranges      alertClimateRanges
	Box         alertClimateBox
	Rules       alertRulesPage
	Recent      []alertRow
}

// alertClimateRequest is the Climate alerts card.
type alertClimateRequest struct {
	Mode           string `json:"mode"`
	SafetyReadings int    `json:"safety_readings"`
	WindowMinutes  int    `json:"window_minutes"`
	DriftHours     int    `json:"drift_hours"`
	PauseMinutes   int    `json:"pause_minutes"`
	RepeatCritical int    `json:"repeat_critical_minutes"`
	RepeatWarning  int    `json:"repeat_warning_hours"`
}

// alertClimateRanges gives the card's number boxes their limits.
type alertClimateRanges struct {
	Safety, Window, Drift, Pause, RepeatCritical, RepeatWarning [2]int
}

// alertClimateBox is the text of the Climate alerts box on Grow setup.
type alertClimateBox struct {
	Title, Safety, Stage, DriftName, Pause string
}

// alertRow is one line of Recent alerts.
type alertRow struct {
	Severity    string
	Title       string
	Line        string
	Error       string
	When        string
	Active      bool
	State       string
	Delivery    string
	DeliveryKey string
}

// recentAlertsShown is how many alerts Settings > Alerts lists.
const recentAlertsShown = 50

func climateRequest(cfg alerts.ClimateConfig) alertClimateRequest {
	t := cfg.Timing
	return alertClimateRequest{
		Mode: cfg.Mode, SafetyReadings: t.SafetyReadings, WindowMinutes: int(t.Window.Minutes()), DriftHours: int(t.Drift.Hours()),
		PauseMinutes: int(t.Pause.Minutes()), RepeatCritical: int(t.RepeatCritical.Minutes()), RepeatWarning: int(t.RepeatWarning.Hours()),
	}
}

// alertWordsFor builds event text for the request's language.
func alertWordsFor(c *gin.Context) alerts.Words {
	return alerts.Words{T: func(key string) string { return T(c, key) }, Units: ConfigStoreFromContext(c).Units(), Loc: requestLocation(c)}
}

func fillN(s string, n int) string { return strings.ReplaceAll(s, "{n}", strconv.Itoa(n)) }

// AlertSettingsPageData builds the Settings > Alerts data for a request.
func AlertSettingsPageData(c *gin.Context) AlertSettingsData {
	db := DBFromContext(c)
	e, err := alerts.LoadEmail(db, BackupServiceFromContext(c).DataDir())
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read email settings")
	}
	if e.PasswordLost {
		logger.Log.Warn("The saved mail password can't be read here (no key file, or another one); enter it again in Settings > Alerts")
	}
	d := AlertSettingsData{To: strings.Join(e.To, ", "), PasswordSet: e.Password != "",
		EmailReady: e.On && !e.PasswordLost && e.Problem() == ""}
	e.Password = ""
	if e.Port == 0 {
		e.Port = 587
	}
	d.Email = e

	cfg, err := alerts.LoadClimateConfig(db)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read the Climate alerts settings")
	}
	d.Climate = climateRequest(cfg)
	d.Ranges = alertClimateRanges{alerts.SafetyReadingsRange, alerts.WindowMinutesRange, alerts.DriftHoursRange,
		alerts.PauseMinutesRange, alerts.RepeatCriticalRange, alerts.RepeatWarningRange}
	d.Box = alertClimateBox{
		Title:     strings.ReplaceAll(T(c, "cl_al_title"), "{mode}", T(c, "al_mode_"+d.Climate.Mode)),
		Safety:    fillN(T(c, "cl_al_safety"), d.Climate.SafetyReadings),
		Stage:     fillN(T(c, "cl_al_stage"), d.Climate.WindowMinutes),
		DriftName: fillN(T(c, "cl_al_drift_b"), d.Climate.DriftHours),
		Pause:     fillN(T(c, "cl_al_pause"), d.Climate.PauseMinutes),
	}

	d.Rules = alertRulesPageData(c)

	events, err := RecentAlerts(db, recentAlertsShown)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read recent alerts")
	}
	w := alertWordsFor(c)
	loc := requestLocation(c)
	for _, ev := range events {
		r := alertRow{Severity: alerts.Severity(ev.Kind), Title: w.Title(ev, false), Active: ev.Resolved == nil, DeliveryKey: ev.Delivery}
		if r.Active {
			r.Line = w.Line(ev, false)
			r.State = T(c, "al_state_active")
			r.When = ev.Started.In(loc).Format("Jan 2, 3:04 PM")
		} else {
			r.State = T(c, "al_state_ended")
			r.When = ev.Started.In(loc).Format("Jan 2, 3:04 PM") + " – " + ev.Resolved.In(loc).Format("3:04 PM")
		}
		if ev.Delivery != "" {
			r.Delivery = T(c, "al_delivery_"+ev.Delivery)
		}
		if ev.Delivery == alerts.DeliveryFailed {
			r.Error = ev.DeliveryError
		}
		d.Recent = append(d.Recent, r)
	}
	return d
}

// SaveAlertClimateHandler: POST /settings/alerts/climate saves the Climate
// alerts card. The language in use becomes the language alert emails are
// written in, since there's no install-wide language.
func SaveAlertClimateHandler(c *gin.Context) {
	var req alertClimateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	if req.Mode != alerts.ModeOff && req.Mode != alerts.ModeDryRun && req.Mode != alerts.ModeEmail {
		apiBadRequest(c, "api_alert_bad_mode")
		return
	}
	rows := []struct {
		name  string
		value int
		rng   [2]int
	}{
		{alerts.SettingSafetyReadings, req.SafetyReadings, alerts.SafetyReadingsRange},
		{alerts.SettingWindowMinutes, req.WindowMinutes, alerts.WindowMinutesRange},
		{alerts.SettingDriftHours, req.DriftHours, alerts.DriftHoursRange},
		{alerts.SettingPauseMinutes, req.PauseMinutes, alerts.PauseMinutesRange},
		{alerts.SettingRepeatCritical, req.RepeatCritical, alerts.RepeatCriticalRange},
		{alerts.SettingRepeatWarning, req.RepeatWarning, alerts.RepeatWarningRange},
	}
	for _, r := range rows {
		if r.value < r.rng[0] || r.value > r.rng[1] {
			apiBadRequest(c, "api_alert_bad_number")
			return
		}
	}
	db := DBFromContext(c)
	if err := UpdateSetting(db, nil, alerts.SettingClimateMode, req.Mode); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	if err := UpdateSetting(db, nil, alerts.SettingLanguage, utils.GetLanguage(c)); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	for _, r := range rows {
		if err := UpdateSetting(db, nil, r.name, strconv.Itoa(r.value)); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
	}
	apiOK(c, "api_settings_saved")
}

type alertEmailRequest struct {
	On       *bool  `json:"on"` // required, so an empty body can't clear everything
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Username string `json:"username"`
	Password string `json:"password"` // "" keeps the saved one
	From     string `json:"from"`
	To       string `json:"to"`
	Link     string `json:"link"`
}

// email turns the request into settings, falling back to the saved
// password (decrypted with the key in dataDir) when none was typed; one
// that can't be decrypted counts as none. A blank username clears the
// password.
func (r alertEmailRequest) email(db *sql.DB, dataDir string) (alerts.Email, error) {
	e := alerts.Email{
		On:       r.On != nil && *r.On,
		Host:     strings.TrimSpace(r.Host),
		Port:     r.Port,
		Security: r.Security,
		Username: strings.TrimSpace(r.Username),
		Password: r.Password,
		From:     strings.TrimSpace(r.From),
		To:       alerts.SplitAddresses(r.To),
		Link:     strings.TrimRight(strings.TrimSpace(r.Link), "/"),
	}
	if e.Username == "" {
		e.Password = ""
	} else if e.Password == "" {
		saved, err := GetSetting(db, alerts.SettingEmailPassword)
		if err != nil {
			return e, err
		}
		e.Password, _ = alerts.OpenPassword(dataDir, saved)
	}
	return e, nil
}

// alertEmailProblem checks what the user typed: everything when email is
// on (or for a test), only lengths and the link's form otherwise.
func alertEmailProblem(e alerts.Email, full bool) string {
	if len(e.Host) > 253 || len(e.Username) > 254 || len(e.Password) > 256 || len(e.From) > 254 || len(e.To) > alerts.MaxRecipients {
		return "al_err_too_long"
	}
	if strings.ContainsAny(e.Username+e.Password, "\r\n") {
		return "al_err_username"
	}
	if e.Link != "" {
		u, err := url.Parse(e.Link)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(e.Link) > 300 {
			return "al_err_link"
		}
	}
	if full {
		return e.Problem()
	}
	return ""
}

// SaveAlertEmailHandler: POST /settings/alerts/email.
func SaveAlertEmailHandler(c *gin.Context) {
	var req alertEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.On == nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	db := DBFromContext(c)
	dataDir := BackupServiceFromContext(c).DataDir()
	e, err := req.email(db, dataDir)
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	if p := alertEmailProblem(e, e.On); p != "" {
		apiBadRequest(c, p)
		return
	}
	on := "0"
	if e.On {
		on = "1"
	}
	port := ""
	if e.Port > 0 {
		port = strconv.Itoa(e.Port)
	}
	sealed, err := alerts.SealPassword(dataDir, e.Password)
	if err != nil {
		logger.Log.WithError(err).Error("Could not encrypt the mail password")
		apiInternalError(c, "failed_to_save_settings")
		return
	}
	rows := [][2]string{
		{alerts.SettingEmailOn, on},
		{alerts.SettingEmailHost, e.Host},
		{alerts.SettingEmailPort, port},
		{alerts.SettingEmailSecurity, e.Security},
		{alerts.SettingEmailUser, e.Username},
		{alerts.SettingEmailPassword, sealed},
		{alerts.SettingEmailFrom, e.From},
		{alerts.SettingEmailTo, strings.Join(e.To, ", ")},
		{alerts.SettingLinkAddress, e.Link},
	}
	for _, r := range rows {
		if err := UpdateSetting(db, nil, r[0], r[1]); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"on": e.On, "passwordSet": e.Password != ""})
}

// TestAlertEmailHandler: POST /settings/alerts/email/test sends a test
// email with what's in the form (saved or not), and reports the mail
// server's answer when it fails, with advice (hint) when there is some.
func TestAlertEmailHandler(c *gin.Context) {
	var req alertEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.On == nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	e, err := req.email(DBFromContext(c), BackupServiceFromContext(c).DataDir())
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	if p := alertEmailProblem(e, true); p != "" {
		apiBadRequest(c, p)
		return
	}
	link := ""
	if e.Link != "" {
		link = e.Link + "/settings#alerts"
	}
	m := alerts.Simple(T(c, "al_test_subject"), []string{T(c, "al_test_body")}, link, T(c, "al_open_budminder"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := e.Send(ctx, m); err != nil {
		logger.Log.WithError(err).WithField("host", e.Host).Warn("Test email failed")
		out := gin.H{"error": T(c, "al_test_failed") + " " + err.Error()}
		if h := alerts.Hint(err, e.Host); h != "" {
			out["hint"] = T(c, h)
		}
		c.JSON(http.StatusBadGateway, out)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": T(c, "al_test_sent")})
}

// keepMailPassword reads the saved mail password before a restore
// replaces the settings table (backups leave it out) and returns a func
// that puts it back if the restored settings have none.
func keepMailPassword(db *sql.DB) func() {
	saved, err := GetSetting(db, alerts.SettingEmailPassword)
	return func() {
		if err != nil || saved == "" {
			return
		}
		if now, err := GetSetting(db, alerts.SettingEmailPassword); err == nil && now == "" {
			if err := UpdateSetting(db, nil, alerts.SettingEmailPassword, saved); err != nil {
				logger.Log.WithError(err).Warn("Could not keep the mail password after the restore")
			}
		}
	}
}
