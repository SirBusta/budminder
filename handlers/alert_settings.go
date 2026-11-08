package handlers

// Settings > Alerts: notification methods (alerts/email.go sends) and Recent alerts.

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
	EmailReady  bool // on and complete, so BudMinder can send
	EmailSetUp  bool // a mail server has been entered, so off means paused
	Rules       alertRulesPage
	Events      alertEventLists
}

// alertEventLists is the two tabs of Recent alerts.
type alertEventLists struct {
	Recent  []alertRow `json:"recent"`
	Cleared []alertRow `json:"cleared"`
}

// alertRow is one line of Recent alerts.
type alertRow struct {
	ID          int    `json:"id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Value       string `json:"value,omitempty"` // the reading when it started (limit and sensor alerts)
	Line        string `json:"line"`
	Zone        string `json:"zone"`  // for the Zone filter
	Type        string `json:"type"`  // for the Type filter: a measure, "rule", "light", "offline" or "integration"
	Chart       bool   `json:"chart"` // opens the Sensors page at this alert
	Error       string `json:"error"`
	When        string `json:"when"`
	Active      bool   `json:"active"`
	State       string `json:"state"`
	Delivery    string `json:"delivery"`
	DeliveryKey string `json:"delivery_key"`
}

// recentAlertsShown is how many alerts each tab of Recent alerts lists.
const recentAlertsShown = 50

// alertWordsFor builds event text for the request's language.
func alertWordsFor(c *gin.Context) alerts.Words {
	return alerts.Words{T: func(key string) string { return T(c, key) }, Units: ConfigStoreFromContext(c).Units(), Loc: requestLocation(c)}
}

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
		EmailReady: e.On && !e.PasswordLost && e.Problem() == "", EmailSetUp: e.Host != ""}
	e.Password = ""
	if e.Port == 0 {
		e.Port = 587
	}
	d.Email = e

	d.Rules = alertRulesPageData(c)
	d.Events = alertEventLists{Recent: alertRows(c, false), Cleared: alertRows(c, true)}
	return d
}

// alertRows builds one tab of Recent alerts.
func alertRows(c *gin.Context, cleared bool) []alertRow {
	out := []alertRow{}
	events, err := RecentAlerts(DBFromContext(c), cleared, recentAlertsShown)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read recent alerts")
	}
	w := alertWordsFor(c)
	loc := requestLocation(c)
	for _, ev := range events {
		r := alertRow{ID: ev.ID, Severity: alerts.Severity(ev.Kind), Title: w.Title(ev, false), Active: ev.Resolved == nil, DeliveryKey: ev.Delivery,
			Zone: ev.Zone, Type: alertType(ev), Chart: alerts.HasChart(ev.Kind)}
		switch {
		case alerts.HasReadings(ev.Kind):
			r.Value = w.RowValue(ev)
			r.Line = w.RowLine(ev, r.Active)
		case r.Active:
			r.Line = w.Line(ev, false)
		}
		r.When = alertWhen(ev, loc)
		r.State = T(c, "al_state_ended")
		if r.Active {
			r.State = T(c, "al_state_active")
		}
		if ev.Delivery != "" {
			r.Delivery = T(c, "al_delivery_"+ev.Delivery)
		}
		if ev.Delivery == alerts.DeliveryFailed {
			r.Error = ev.DeliveryError
		}
		out = append(out, r)
	}
	return out
}

// alertType is what the Type filter of Recent alerts groups an alert under.
func alertType(ev alerts.Event) string {
	switch ev.Kind {
	case alerts.KindStage:
		return ev.Measure
	case alerts.KindSensor:
		return "rule"
	case alerts.KindLight, alerts.KindClock:
		return "light"
	}
	return ev.Kind
}

// alertWhen is when an alert started, and ended: the end's date shows only
// when it's another day.
func alertWhen(ev alerts.Event, loc *time.Location) string {
	start := ev.Started.In(loc)
	out := start.Format("Jan 2, 3:04 PM")
	if ev.Resolved == nil {
		return out
	}
	end := ev.Resolved.In(loc)
	if end.YearDay() == start.YearDay() && end.Year() == start.Year() {
		return out + " – " + end.Format("3:04 PM")
	}
	return out + " – " + end.Format("Jan 2, 3:04 PM")
}

// setAlertCleared moves one ended alert to the Cleared tab or back.
func setAlertCleared(c *gin.Context, cleared bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	if _, err := DBFromContext(c).Exec(`UPDATE alert_event SET cleared = $1 WHERE id = $2 AND resolved_at IS NOT NULL`, boolInt(cleared), id); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	apiOK(c, "api_settings_saved")
}

// ClearAlertHandler: POST /settings/alerts/events/:id/clear. An active alert
// can't be cleared. Cleared alerts still go after 90 days.
func ClearAlertHandler(c *gin.Context) { setAlertCleared(c, true) }

// RestoreAlertHandler: POST /settings/alerts/events/:id/restore.
func RestoreAlertHandler(c *gin.Context) { setAlertCleared(c, false) }

// ClearEndedAlertsHandler: POST /settings/alerts/clear-ended. With ids (the
// alerts a Zone or Type filter shows), only those; without, every ended alert.
func ClearEndedAlertsHandler(c *gin.Context) {
	var body struct {
		IDs *[]int `json:"ids"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			apiBadRequest(c, "api_invalid_request_payload")
			return
		}
	}
	db := DBFromContext(c)
	if body.IDs == nil {
		if _, err := db.Exec(`UPDATE alert_event SET cleared = 1 WHERE resolved_at IS NOT NULL`); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
		apiOK(c, "api_settings_saved")
		return
	}
	if len(*body.IDs) > recentAlertsShown {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	for _, id := range *body.IDs {
		if _, err := db.Exec(`UPDATE alert_event SET cleared = 1 WHERE id = $1 AND resolved_at IS NOT NULL`, id); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
	}
	apiOK(c, "api_settings_saved")
}

// anyAlertSet says something could send an alert notification: a limit in use,
// a sensor alert that's on, or a Light check.
func anyAlertSet(db *sql.DB) (bool, error) {
	cs, err := LoadClimate(db)
	if err != nil {
		return false, err
	}
	if cs.anyLimit() {
		return true, nil
	}
	rules, err := loadAlertRules(db)
	if err != nil {
		return false, err
	}
	for _, r := range rules {
		if r.Enabled {
			return true, nil
		}
	}
	zones, err := LoadZoneSetups(db)
	if err != nil {
		return false, err
	}
	for _, z := range zones {
		if z.LightSensorID != nil && len(z.Schedule) > 0 {
			return true, nil
		}
	}
	return false, nil
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

// SaveAlertEmailHandler: POST /settings/alerts/email. Turning notifications on
// is refused while nothing could send one. The language in use becomes the
// language alert emails are written in, since there's no install-wide language.
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
	if e.On {
		was, err := GetSetting(db, alerts.SettingEmailOn)
		if err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
		set, err := anyAlertSet(db)
		if err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
		if was != "1" && !set {
			c.JSON(http.StatusBadRequest, gin.H{"code": "nothing_set", "error": T(c, "al_refuse_plain")})
			return
		}
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
		{alerts.SettingLanguage, utils.GetLanguage(c)},
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
