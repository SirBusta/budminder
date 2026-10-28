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
)

// AlertSettingsData is what the Alerts section of Settings renders with.
// The password itself never goes to the page.
type AlertSettingsData struct {
	Email       alerts.Email
	To          string // recipients, comma-separated
	PasswordSet bool
}

// AlertSettingsPageData builds the Settings > Alerts data for a request.
func AlertSettingsPageData(c *gin.Context) AlertSettingsData {
	e, err := alerts.LoadEmail(DBFromContext(c))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read email settings")
	}
	d := AlertSettingsData{To: strings.Join(e.To, ", "), PasswordSet: e.Password != ""}
	e.Password = ""
	if e.Port == 0 {
		e.Port = 587
	}
	d.Email = e
	return d
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
// password when none was typed. A blank username clears the password.
func (r alertEmailRequest) email(db *sql.DB) (alerts.Email, error) {
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
		e.Password = saved
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
	e, err := req.email(db)
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
	rows := [][2]string{
		{alerts.SettingEmailOn, on},
		{alerts.SettingEmailHost, e.Host},
		{alerts.SettingEmailPort, port},
		{alerts.SettingEmailSecurity, e.Security},
		{alerts.SettingEmailUser, e.Username},
		{alerts.SettingEmailPassword, e.Password},
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
// server's answer when it fails.
func TestAlertEmailHandler(c *gin.Context) {
	var req alertEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.On == nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	e, err := req.email(DBFromContext(c))
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
		c.JSON(http.StatusBadGateway, gin.H{"error": T(c, "al_test_failed") + " " + err.Error()})
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
