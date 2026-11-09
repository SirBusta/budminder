package handlers

// Settings > Alerts > Notification methods besides email: Pushover, the Home
// Assistant app, ntfy, Discord and a webhook (senders in alerts/push.go), and
// the BudMinder address every method's links use. Each method saves when a
// field changes; Send test uses what's typed, saved or not. A blank secret
// keeps the saved one; Remove clears the two optional ones.

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/alerts"
	"isley/homeassistant"
	"isley/logger"
	"isley/utils"
)

// alertMethodsPage is what the page needs about the other methods. Secrets
// never go to the page: only whether one is saved.
type alertMethodsPage struct {
	HAConnected bool   `json:"haConnected"`
	HAURL       string `json:"haURL"`

	PushoverOn       bool   `json:"pushoverOn"`
	PushoverUser     string `json:"pushoverUser"`
	PushoverTokenSet bool   `json:"pushoverTokenSet"`
	PushoverDevices  string `json:"pushoverDevices"`
	PushoverPriority int    `json:"pushoverPriority"`

	HAOn       bool                  `json:"haOn"`
	HAPhones   []homeassistant.Phone `json:"haPhones"` // the saved ones, named by their ids until the list is refreshed
	HAPriority string                `json:"haPriority"`

	NtfyOn       bool   `json:"ntfyOn"`
	NtfyServer   string `json:"ntfyServer"`
	NtfyTopic    string `json:"ntfyTopic"`
	NtfyTokenSet bool   `json:"ntfyTokenSet"`
	NtfyPriority int    `json:"ntfyPriority"`

	DiscordOn     bool `json:"discordOn"`
	DiscordURLSet bool `json:"discordURLSet"`

	WebhookOn      bool   `json:"webhookOn"`
	WebhookURLSet  bool   `json:"webhookURLSet"`
	WebhookHost    string `json:"webhookHost"`
	WebhookAuthSet bool   `json:"webhookAuthSet"`

	// Ready: on and complete, by method name (email too), for the status lines.
	Ready map[string]bool `json:"ready"`
	// AnySetUp: some method has been set up, so with every one off, notifications are paused.
	AnySetUp bool `json:"anySetUp"`
}

// loadAlertMethods reads every method's settings for a request.
func loadAlertMethods(c *gin.Context) (alerts.Methods, error) {
	store := ConfigStoreFromContext(c)
	return alerts.LoadMethods(DBFromContext(c), BackupServiceFromContext(c).DataDir(), store.HAURL(), store.HAToken(), store.PushoverAPI())
}

func alertMethodsPageData(c *gin.Context) alertMethodsPage {
	m, err := loadAlertMethods(c)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read the notification settings")
	}
	p := alertMethodsPage{
		HAConnected: m.HA.URL != "" && m.HA.Token != "", HAURL: m.HA.URL,
		PushoverOn: m.Pushover.On, PushoverUser: m.Pushover.User, PushoverTokenSet: m.Pushover.Token != "",
		PushoverDevices: m.Pushover.Devices, PushoverPriority: m.Pushover.Priority,
		HAOn: m.HA.On, HAPriority: m.HA.Priority, HAPhones: []homeassistant.Phone{},
		NtfyOn: m.Ntfy.On, NtfyServer: m.Ntfy.Server, NtfyTopic: m.Ntfy.Topic, NtfyTokenSet: m.Ntfy.Token != "", NtfyPriority: m.Ntfy.Priority,
		DiscordOn: m.Discord.On, DiscordURLSet: m.Discord.URL != "",
		WebhookOn: m.Webhook.On, WebhookURLSet: m.Webhook.URL != "", WebhookHost: m.Webhook.Host(), WebhookAuthSet: m.Webhook.Auth != "",
		Ready: map[string]bool{},
	}
	names, _ := GetSetting(DBFromContext(c), settingHAAppNames)
	known := map[string]string{}
	for _, pair := range alerts.SplitList(names) {
		if id, name, ok := strings.Cut(pair, "="); ok {
			known[id] = name
		}
	}
	for _, id := range m.HA.Phones {
		name := known[id]
		if name == "" {
			name = strings.ReplaceAll(strings.TrimPrefix(id, "mobile_app_"), "_", " ")
		}
		p.HAPhones = append(p.HAPhones, homeassistant.Phone{ID: id, Name: name})
	}
	for _, n := range m.Ready() {
		p.Ready[n.Name()] = true
	}
	for _, n := range m.All() {
		p.AnySetUp = p.AnySetUp || n.SetUp()
	}
	return p
}

// settingHAAppNames keeps the chosen phones' names (id=name, comma-separated),
// so the page can show them without asking Home Assistant.
const settingHAAppNames = "alert_haapp_names"

// alertMethodRequest is one method's fields; each method reads its own.
type alertMethodRequest struct {
	On       *bool    `json:"on"` // required, so an empty body can't clear everything
	User     string   `json:"user"`
	Token    string   `json:"token"` // "" keeps the saved one
	Devices  string   `json:"devices"`
	Priority string   `json:"priority"`
	Phones   []string `json:"phones"`
	Names    []string `json:"names"` // the phones' names, same order
	Server   string   `json:"server"`
	Topic    string   `json:"topic"`
	URL      string   `json:"url"`  // "" keeps the saved one
	Auth     string   `json:"auth"` // "" keeps the saved one
	// RemoveToken / RemoveAuth clear ntfy's access token or the webhook's
	// Authorization header (optional secrets: a blank box keeps them).
	RemoveToken bool `json:"remove_token"`
	RemoveAuth  bool `json:"remove_auth"`
}

// alertMethodFrom fills method name from the request over the saved settings
// m. ok is false for an unknown method.
func alertMethodFrom(name string, req alertMethodRequest, m *alerts.Methods) (alerts.Notifier, bool) {
	on := req.On != nil && *req.On
	keep := func(typed, saved string) string {
		if typed = strings.TrimSpace(typed); typed != "" {
			return typed
		}
		return saved
	}
	switch name {
	case alerts.MethodPushover:
		p := &m.Pushover
		p.On, p.User, p.Token = on, strings.TrimSpace(req.User), keep(req.Token, p.Token)
		p.Devices = strings.Join(alerts.SplitList(req.Devices), ", ")
		p.Priority = atoiOr(req.Priority, -1)
		return p, true
	case alerts.MethodHAApp:
		h := &m.HA
		h.On, h.Phones, h.Priority = on, nil, req.Priority
		for _, ph := range req.Phones {
			if ph = strings.TrimSpace(ph); ph != "" {
				h.Phones = append(h.Phones, ph)
			}
		}
		return h, true
	case alerts.MethodNtfy:
		n := &m.Ntfy
		n.On, n.Server, n.Topic = on, strings.TrimRight(strings.TrimSpace(req.Server), "/"), strings.TrimSpace(req.Topic)
		if n.Server == "" {
			n.Server = alerts.DefaultNtfyServer
		}
		n.Token = keep(req.Token, n.Token)
		if req.RemoveToken {
			n.Token = ""
		}
		n.Priority = atoiOr(req.Priority, -1)
		return n, true
	case alerts.MethodDiscord:
		d := &m.Discord
		d.On, d.URL = on, keep(req.URL, d.URL)
		return d, true
	case alerts.MethodWebhook:
		w := &m.Webhook
		w.On, w.URL, w.Auth = on, keep(req.URL, w.URL), keep(req.Auth, w.Auth)
		if req.RemoveAuth {
			w.Auth = ""
		}
		return w, true
	}
	return nil, false
}

func atoiOr(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return v
	}
	return def
}

// methodTooLong checks lengths and line breaks, which apply even while a method is off.
func methodTooLong(req alertMethodRequest) bool {
	for _, s := range []string{req.User, req.Devices, req.Priority, req.Server, req.Topic} {
		if len(s) > 300 || strings.ContainsAny(s, "\r\n") {
			return true
		}
	}
	for _, s := range []string{req.Token, req.URL, req.Auth} {
		if len(s) > 2048 || strings.ContainsAny(s, "\r\n") {
			return true
		}
	}
	if len(req.Phones) > 20 || len(req.Names) > 20 {
		return true
	}
	for _, s := range append(append([]string{}, req.Phones...), req.Names...) {
		if len(s) > 200 || strings.ContainsAny(s, "\r\n,=") {
			return true
		}
	}
	return false
}

// SaveAlertMethodHandler: POST /settings/alerts/methods/:method. Turning a
// method on checks every field and is refused while nothing could send an
// alert; while off, what's typed is kept as it is. The language in use
// becomes the language notifications are written in.
func SaveAlertMethodHandler(c *gin.Context) {
	var req alertMethodRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.On == nil || methodTooLong(req) {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	m, err := loadAlertMethods(c)
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	name := c.Param("method")
	wasOn := false
	if n := m.Get(name); n != nil {
		wasOn = n.Enabled()
	}
	n, ok := alertMethodFrom(name, req, &m)
	if !ok {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	db := DBFromContext(c)
	if n.Enabled() {
		if p := n.Problem(); p != "" {
			apiBadRequest(c, p)
			return
		}
		set, err := anyAlertSet(db)
		if err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
		if !wasOn && !set {
			c.JSON(http.StatusBadRequest, gin.H{"code": "nothing_set", "error": T(c, "al_refuse_plain")})
			return
		}
	}
	dataDir := BackupServiceFromContext(c).DataDir()
	seal := func(s string) (string, bool) {
		v, err := alerts.SealPassword(dataDir, s)
		if err != nil {
			logger.Log.WithError(err).Error("Could not encrypt a notification secret")
			apiInternalError(c, "failed_to_save_settings")
			return "", false
		}
		return v, true
	}
	on := "0"
	if n.Enabled() {
		on = "1"
	}
	var rows [][2]string
	switch name {
	case alerts.MethodPushover:
		tok, ok := seal(m.Pushover.Token)
		if !ok {
			return
		}
		rows = [][2]string{{alerts.SettingPushoverOn, on}, {alerts.SettingPushoverUser, m.Pushover.User}, {alerts.SettingPushoverToken, tok},
			{alerts.SettingPushoverDevices, m.Pushover.Devices}, {alerts.SettingPushoverPriority, strconv.Itoa(m.Pushover.Priority)}}
	case alerts.MethodHAApp:
		var named []string
		for i, id := range m.HA.Phones {
			if i < len(req.Names) && strings.TrimSpace(req.Names[i]) != "" {
				named = append(named, id+"="+strings.TrimSpace(req.Names[i]))
			}
		}
		rows = [][2]string{{alerts.SettingHAAppOn, on}, {alerts.SettingHAAppPhones, strings.Join(m.HA.Phones, ",")},
			{alerts.SettingHAAppPriority, m.HA.Priority}, {settingHAAppNames, strings.Join(named, ",")}}
	case alerts.MethodNtfy:
		tok, ok := seal(m.Ntfy.Token)
		if !ok {
			return
		}
		rows = [][2]string{{alerts.SettingNtfyOn, on}, {alerts.SettingNtfyServer, m.Ntfy.Server}, {alerts.SettingNtfyTopic, m.Ntfy.Topic},
			{alerts.SettingNtfyToken, tok}, {alerts.SettingNtfyPriority, strconv.Itoa(m.Ntfy.Priority)}}
	case alerts.MethodDiscord:
		u, ok := seal(m.Discord.URL)
		if !ok {
			return
		}
		rows = [][2]string{{alerts.SettingDiscordOn, on}, {alerts.SettingDiscordURL, u}}
	case alerts.MethodWebhook:
		u, ok := seal(m.Webhook.URL)
		if !ok {
			return
		}
		a, ok := seal(m.Webhook.Auth)
		if !ok {
			return
		}
		rows = [][2]string{{alerts.SettingWebhookOn, on}, {alerts.SettingWebhookURL, u}, {alerts.SettingWebhookAuth, a}}
	}
	rows = append(rows, [2]string{alerts.SettingLanguage, utils.GetLanguage(c)})
	for _, r := range rows {
		if err := UpdateSetting(db, nil, r[0], r[1]); err != nil {
			apiInternalError(c, "api_database_error")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"on": n.Enabled(), "ready": n.Enabled() && n.Problem() == "", "methods": alertMethodsPageData(c)})
}

// TestAlertMethodHandler: POST /settings/alerts/methods/:method/test sends a
// test notification with what's in the form, saved or not.
func TestAlertMethodHandler(c *gin.Context) {
	var req alertMethodRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.On == nil || methodTooLong(req) {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	m, err := loadAlertMethods(c)
	if err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	name := c.Param("method")
	n, ok := alertMethodFrom(name, req, &m)
	if !ok {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	if p := n.Problem(); p != "" {
		apiBadRequest(c, p)
		return
	}
	notice := alerts.TestNotice(T(c, "al_test_push_title"), T(c, "al_test_push_body"), m.Email.Link, T(c, "al_open_budminder"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	failedKey := "al_test_failed_push"
	if name == alerts.MethodWebhook {
		failedKey = "al_test_failed_webhook"
		status, err := m.Webhook.Send(ctx, notice)
		if err != nil {
			logger.Log.WithError(err).Warn("Test webhook failed")
			c.JSON(http.StatusBadGateway, gin.H{"error": T(c, failedKey) + " " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": strings.ReplaceAll(T(c, "al_test_sent_webhook"), "{status}", status)})
		return
	}
	if err := n.Notify(ctx, notice); err != nil {
		logger.Log.WithError(err).WithField("method", name).Warn("Test notification failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": T(c, failedKey) + " " + err.Error()})
		return
	}
	msg := T(c, "al_test_sent_"+name)
	if name == alerts.MethodHAApp {
		names := req.Names
		if len(names) == 0 {
			names = m.HA.Phones
		}
		msg = strings.ReplaceAll(msg, "{phones}", strings.Join(names, ", "))
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

// AlertHAPhonesHandler: GET /settings/alerts/ha-phones lists the phones with
// the Home Assistant app, through the saved Home Assistant connection.
func AlertHAPhonesHandler(c *gin.Context) {
	client, err := haClient(ConfigStoreFromContext(c), "", "")
	if err != nil {
		apiBadRequest(c, "al_err_ha_conn")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*homeassistant.Timeout)
	defer cancel()
	phones, err := client.Phones(ctx)
	if err != nil {
		logger.Log.WithError(err).Warn("Could not list the Home Assistant app's phones")
		status, key := haErrorKey(err)
		apiError(c, status, key)
		return
	}
	c.JSON(http.StatusOK, gin.H{"phones": phones})
}

// SaveAlertLinkHandler: POST /settings/alerts/link saves the BudMinder address
// every method's links use.
func SaveAlertLinkHandler(c *gin.Context) {
	var req struct {
		Link string `json:"link"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	link := strings.TrimRight(strings.TrimSpace(req.Link), "/")
	if link != "" {
		u, err := url.Parse(link)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(link) > 300 || strings.ContainsAny(link, " \r\n") {
			apiBadRequest(c, "al_err_link")
			return
		}
	}
	if err := UpdateSetting(DBFromContext(c), nil, alerts.SettingLinkAddress, link); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"link": link})
}
