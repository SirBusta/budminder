package alerts

// Notification methods besides email (Settings > Alerts > Notification
// methods): Pushover, the Home Assistant phone app, ntfy, Discord and a general
// webhook. Every method that is on gets every notification; senders are in
// push.go. Tokens and webhook URLs are sealed like the mail password and left
// out of backups (SealedSettings).

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"isley/climate"
)

// Settings rows for the other notification methods.
const (
	SettingPushoverOn       = "alert_pushover_enabled"
	SettingPushoverUser     = "alert_pushover_user"
	SettingPushoverToken    = "alert_pushover_token" // sealed
	SettingPushoverDevices  = "alert_pushover_devices"
	SettingPushoverPriority = "alert_pushover_priority" // "0", "1" or "2"

	SettingHAAppOn       = "alert_haapp_enabled"
	SettingHAAppPhones   = "alert_haapp_phones"   // notify services, comma-separated, e.g. mobile_app_daves_iphone
	SettingHAAppPriority = "alert_haapp_priority" // HAPriorityNormal or HAPriorityCritical

	SettingNtfyOn       = "alert_ntfy_enabled"
	SettingNtfyServer   = "alert_ntfy_server"
	SettingNtfyTopic    = "alert_ntfy_topic"
	SettingNtfyToken    = "alert_ntfy_token"    // sealed
	SettingNtfyPriority = "alert_ntfy_priority" // "3", "4" or "5"

	SettingDiscordOn  = "alert_discord_enabled"
	SettingDiscordURL = "alert_discord_url" // sealed: the URL is the password

	SettingWebhookOn   = "alert_webhook_enabled"
	SettingWebhookURL  = "alert_webhook_url"  // sealed
	SettingWebhookAuth = "alert_webhook_auth" // sealed: the Authorization header
)

// SealedSettings are the settings stored encrypted (SealPassword). Backups and
// the database download leave them out, and a restore keeps the current ones.
var SealedSettings = []string{SettingEmailPassword, SettingPushoverToken, SettingNtfyToken,
	SettingDiscordURL, SettingWebhookURL, SettingWebhookAuth}

// Method names, as saved in alert_event.retry_methods and used in routes.
const (
	MethodEmail    = "email"
	MethodPushover = "pushover"
	MethodHAApp    = "ha"
	MethodNtfy     = "ntfy"
	MethodDiscord  = "discord"
	MethodWebhook  = "webhook"
)

// MethodNames lists every method in page order.
var MethodNames = []string{MethodEmail, MethodPushover, MethodHAApp, MethodNtfy, MethodDiscord, MethodWebhook}

// MethodLabel is a method's name as delivery errors show it.
func MethodLabel(m string) string {
	switch m {
	case MethodEmail:
		return "Email"
	case MethodPushover:
		return "Pushover"
	case MethodHAApp:
		return "Home Assistant app"
	case MethodNtfy:
		return "ntfy"
	case MethodDiscord:
		return "Discord"
	case MethodWebhook:
		return "Webhook"
	}
	return m
}

// Priorities. All-clear notifications always use the lowest one.
const (
	HAPriorityNormal   = "normal"
	HAPriorityCritical = "critical"

	DefaultPushoverPriority = 1 // High
	DefaultNtfyPriority     = 4 // High
	DefaultHAPriority       = HAPriorityCritical
	DefaultNtfyServer       = "https://ntfy.sh"
)

// Item is one alert in a notification, also what the webhook sends.
type Item struct {
	ID        int       `json:"-"`
	Event     string    `json:"event"` // start, reminder, end (test for a test request)
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	Kind      string    `json:"kind"`
	Zone      string    `json:"zone"`
	Sensor    string    `json:"sensor,omitempty"`
	Measure   string    `json:"measure"`
	Value     *float64  `json:"value"`
	Limit     *float64  `json:"limit"`
	Unit      string    `json:"unit"`
	StartedAt time.Time `json:"started_at"`
	URL       string    `json:"url"`
	resolved  bool
}

// Webhook event names.
const (
	EventStart    = "start"
	EventReminder = "reminder"
	EventEnd      = "end"
	EventTest     = "test"
)

// Notice is one notification for every pending alert, ready for each method:
// Email for email, Title/Text/URL for phone apps and Discord, Items for the
// webhook.
type Notice struct {
	Title    string // the most serious alert's title, "... and N more"
	Text     string
	URL      string // a tap opens this: the first alert's readings, or Settings > Alerts
	LinkText string
	Urgent   bool // a new alert or reminder; false when it's only all clears
	Severity string
	Items    []Item
	Email    Message
}

// BuildNotice makes one notification for every pending notice in events; link
// is BudMinder's address ("" when not set).
func BuildNotice(w Words, events []Event, link string) Notice {
	sorted := append([]Event(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, rj := sorted[i].Pending == NoticeResolved, sorted[j].Pending == NoticeResolved
		if ri != rj {
			return !ri // alerts before back-to-normal notices
		}
		return severityRank(sorted[i].Kind) < severityRank(sorted[j].Kind)
	})
	var n Notice
	var paras, lines []string
	for _, e := range sorted {
		resolved := e.Pending == NoticeResolved
		title, line := w.Title(e, resolved), w.Line(e, resolved)
		short := line
		para := title + ". " + line
		if !resolved {
			n.Urgent = true
			since := w.fill("al_since", map[string]string{"time": w.Clock(e.Started)})
			short += " " + since
			para += " " + since
			if e.Pending == NoticeRepeat {
				short += " " + w.T("al_still")
				para += " " + w.T("al_still")
			}
			switch e.Kind {
			case KindStage:
				para += " " + w.T("al_widen")
			case KindSensor:
				para += " " + w.T("al_rule_change")
			}
		}
		paras = append(paras, para)
		if len(sorted) == 1 {
			lines = append(lines, short)
		} else {
			lines = append(lines, title+". "+short)
		}
		n.Items = append(n.Items, w.item(e, resolved, title, short, link))
	}
	first := ""
	if len(sorted) > 0 {
		first = w.Title(sorted[0], sorted[0].Pending == NoticeResolved)
		n.URL = n.Items[0].URL
		n.Severity = Severity(sorted[0].Kind)
		if sorted[0].Pending == NoticeResolved {
			n.Severity = ""
		}
	}
	subject := w.fill("al_subject", map[string]string{"title": first})
	n.Title = first
	if len(sorted) > 1 {
		subject = w.fill("al_subject_more", map[string]string{"title": first, "n": strconv.Itoa(len(sorted) - 1)})
		n.Title = w.fill("al_push_more", map[string]string{"title": first, "n": strconv.Itoa(len(sorted) - 1)})
	}
	n.Text = strings.Join(lines, "\n")
	n.LinkText = w.T("al_open_budminder")
	if n.URL != "" && len(sorted) > 0 && HasChart(sorted[0].Kind) {
		n.LinkText = w.T("al_ev_view")
	}
	emailLink := ""
	if link != "" {
		emailLink = link + "/settings#alerts"
	}
	n.Email = Simple(subject, paras, emailLink, w.T("al_open_budminder"))
	return n
}

// TestNotice is what Send test sends: one item with event "test".
func TestNotice(title, text, link, linkText string) Notice {
	u := ""
	if link != "" {
		u = link + "/settings#alerts"
	}
	return Notice{Title: title, Text: text, URL: u, LinkText: linkText, Urgent: true, Severity: Warning,
		Items: []Item{{Event: EventTest, Title: title, Text: text, StartedAt: time.Now().UTC().Truncate(time.Second), URL: u}},
		Email: Simple(title, []string{text}, u, linkText)}
}

// item is one alert as the webhook sends it, with values in the units the
// text shows.
func (w Words) item(e Event, resolved bool, title, text, link string) Item {
	it := Item{ID: e.ID, Title: title, Text: text, Kind: e.Kind, Zone: e.Zone, Sensor: e.Sensor, Measure: e.Measure,
		StartedAt: e.Started.UTC(), resolved: resolved}
	switch {
	case resolved:
		it.Event = EventEnd
	case e.Pending == NoticeRepeat:
		it.Event = EventReminder
	default:
		it.Event = EventStart
	}
	switch e.Kind {
	case KindStage:
		it.Value, it.Limit = w.shown(e.Measure, e.Value), w.shown(e.Measure, e.Limit)
		it.Unit = w.unitOf(e.Measure)
	case KindSensor:
		it.Measure = "sensor"
		it.Value, it.Limit, it.Unit = e.Value, e.Limit, e.Detail
	}
	if link != "" {
		if HasChart(e.Kind) && e.ID > 0 {
			it.URL = link + "/sensors#alert=" + strconv.Itoa(e.ID)
		} else {
			it.URL = link + "/settings#alerts"
		}
	}
	return it
}

// shown converts a stored value (temperatures in °C) to what the text shows.
func (w Words) shown(measure string, v *float64) *float64 {
	if v == nil {
		return nil
	}
	x := *v
	switch {
	case measure == climate.Temp && w.Units != "metric":
		x = math.Round((x*9/5+32)*10) / 10
	case measure == climate.VPD:
		x = math.Round(x*100) / 100
	default:
		x = math.Round(x*10) / 10
	}
	return &x
}

func (w Words) unitOf(measure string) string {
	switch measure {
	case climate.Temp:
		if w.Units == "metric" {
			return "°C"
		}
		return "°F"
	case climate.RH:
		return "%"
	case climate.VPD:
		return "kPa"
	}
	return ""
}

// Notifier is one notification method.
type Notifier interface {
	Name() string
	// Enabled says the switch is on.
	Enabled() bool
	// SetUp says something has been entered, so off means paused.
	SetUp() bool
	// Problem names what's missing as a text key, or "".
	Problem() string
	Notify(ctx context.Context, n Notice) error
}

// Methods is every notification method's settings.
type Methods struct {
	Email    Email
	Pushover Pushover
	HA       HAApp
	Ntfy     Ntfy
	Discord  Discord
	Webhook  Webhook
}

// All lists the methods in page order.
func (m *Methods) All() []Notifier {
	return []Notifier{emailNotifier{&m.Email}, &m.Pushover, &m.HA, &m.Ntfy, &m.Discord, &m.Webhook}
}

// Ready lists the methods that are on and complete.
func (m *Methods) Ready() []Notifier {
	var out []Notifier
	for _, n := range m.All() {
		if n.Enabled() && n.Problem() == "" {
			out = append(out, n)
		}
	}
	return out
}

// Paused says no method is on but one has been set up.
func (m *Methods) Paused() bool {
	set := false
	for _, n := range m.All() {
		if n.Enabled() {
			return false
		}
		set = set || n.SetUp()
	}
	return set
}

// Get returns a method by name.
func (m *Methods) Get(name string) Notifier {
	for _, n := range m.All() {
		if n.Name() == name {
			return n
		}
	}
	return nil
}

type emailNotifier struct{ e *Email }

func (x emailNotifier) Name() string  { return MethodEmail }
func (x emailNotifier) Enabled() bool { return x.e.On }
func (x emailNotifier) SetUp() bool   { return x.e.Host != "" }
func (x emailNotifier) Problem() string {
	if x.e.PasswordLost {
		return "al_err_password_lost"
	}
	return x.e.Problem()
}
func (x emailNotifier) Notify(ctx context.Context, n Notice) error { return x.e.Send(ctx, n.Email) }

// LoadMethods reads every method's settings, decrypting secrets with the key in
// dataDir; one that can't be decrypted counts as not set. haURL and haToken are
// the Home Assistant connection (Settings > Integrations); pushoverAPI replaces
// Pushover's address in tests ("" = Pushover).
func LoadMethods(db *sql.DB, dataDir, haURL, haToken, pushoverAPI string) (Methods, error) {
	var m Methods
	e, err := LoadEmail(db, dataDir)
	if err != nil {
		return m, err
	}
	m.Email = e
	vals := map[string]string{}
	rows, err := db.Query(`SELECT name, value FROM settings WHERE name LIKE 'alert%'`)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return m, err
		}
		vals[k] = v
	}
	if err := rows.Err(); err != nil {
		return m, err
	}
	open := func(name string) string {
		s, err := OpenPassword(dataDir, vals[name])
		if err != nil {
			return ""
		}
		return s
	}
	m.Pushover = Pushover{On: vals[SettingPushoverOn] == "1", User: vals[SettingPushoverUser], Token: open(SettingPushoverToken),
		Devices: vals[SettingPushoverDevices], Priority: intOr(vals[SettingPushoverPriority], DefaultPushoverPriority), API: pushoverAPI}
	m.HA = HAApp{On: vals[SettingHAAppOn] == "1", Phones: SplitList(vals[SettingHAAppPhones]), Priority: vals[SettingHAAppPriority],
		URL: strings.TrimRight(haURL, "/"), Token: haToken}
	if m.HA.Priority != HAPriorityNormal {
		m.HA.Priority = DefaultHAPriority
	}
	m.Ntfy = Ntfy{On: vals[SettingNtfyOn] == "1", Server: vals[SettingNtfyServer], Topic: vals[SettingNtfyTopic], Token: open(SettingNtfyToken),
		Priority: intOr(vals[SettingNtfyPriority], DefaultNtfyPriority)}
	if m.Ntfy.Server == "" {
		m.Ntfy.Server = DefaultNtfyServer
	}
	m.Discord = Discord{On: vals[SettingDiscordOn] == "1", URL: open(SettingDiscordURL)}
	m.Webhook = Webhook{On: vals[SettingWebhookOn] == "1", URL: open(SettingWebhookURL), Auth: open(SettingWebhookAuth)}
	return m, nil
}

func intOr(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

// SplitList splits a comma-separated list, dropping blanks.
func SplitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

var (
	pushoverKey    = regexp.MustCompile(`^[A-Za-z0-9]{30}$`)
	pushoverDevice = regexp.MustCompile(`^[A-Za-z0-9_-]{1,25}$`)
	haPhone        = regexp.MustCompile(`^mobile_app_[a-z0-9_]{1,200}$`)
	ntfyTopic      = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)
)

// webAddress says s is an http or https address with a host and no user
// name, password or fragment.
func webAddress(s string) bool {
	if len(s) > 2048 || strings.ContainsAny(s, " \r\n\t") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Fragment == ""
}

// Pushover sends through Pushover's API.
type Pushover struct {
	On       bool
	User     string
	Token    string
	Devices  string // comma-separated; "" = all
	Priority int    // 0 Normal, 1 High, 2 Emergency
	API      string // tests: a stand-in for Pushover's address
}

func (p *Pushover) Name() string  { return MethodPushover }
func (p *Pushover) Enabled() bool { return p.On }
func (p *Pushover) SetUp() bool   { return p.User != "" || p.Token != "" }
func (p *Pushover) Problem() string {
	switch {
	case !pushoverKey.MatchString(p.User):
		return "al_err_po_user"
	case !pushoverKey.MatchString(p.Token):
		return "al_err_po_token"
	case p.Priority < 0 || p.Priority > 2:
		return "al_err_priority"
	}
	for _, d := range SplitList(p.Devices) {
		if !pushoverDevice.MatchString(d) {
			return "al_err_po_devices"
		}
	}
	return ""
}

// HAApp sends to the Home Assistant phone app through Home Assistant's notify
// services (mobile_app_<phone>), using the Home Assistant connection.
type HAApp struct {
	On       bool
	Phones   []string
	Priority string
	URL      string
	Token    string
}

func (h *HAApp) Name() string  { return MethodHAApp }
func (h *HAApp) Enabled() bool { return h.On }
func (h *HAApp) SetUp() bool   { return len(h.Phones) > 0 }
func (h *HAApp) Problem() string {
	switch {
	case h.URL == "" || h.Token == "":
		return "al_err_ha_conn"
	case len(h.Phones) == 0 || len(h.Phones) > 20:
		return "al_err_ha_phones"
	case h.Priority != HAPriorityNormal && h.Priority != HAPriorityCritical:
		return "al_err_priority"
	}
	for _, p := range h.Phones {
		if !haPhone.MatchString(p) {
			return "al_err_ha_phones"
		}
	}
	return ""
}

// Ntfy publishes to an ntfy topic.
type Ntfy struct {
	On       bool
	Server   string
	Topic    string
	Token    string
	Priority int // 3 Default, 4 High, 5 Urgent
}

func (n *Ntfy) Name() string  { return MethodNtfy }
func (n *Ntfy) Enabled() bool { return n.On }
func (n *Ntfy) SetUp() bool   { return n.Topic != "" }
func (n *Ntfy) Problem() string {
	switch {
	case !webAddress(n.Server):
		return "al_err_ntfy_server"
	case !ntfyTopic.MatchString(n.Topic):
		return "al_err_ntfy_topic"
	case len(n.Token) > 512 || strings.ContainsAny(n.Token, " \r\n"):
		return "al_err_ntfy_token"
	case n.Priority < 3 || n.Priority > 5:
		return "al_err_priority"
	}
	return ""
}

// Discord posts to a Discord channel's webhook.
type Discord struct {
	On  bool
	URL string
}

func (d *Discord) Name() string  { return MethodDiscord }
func (d *Discord) Enabled() bool { return d.On }
func (d *Discord) SetUp() bool   { return d.URL != "" }
func (d *Discord) Problem() string {
	if !webAddress(d.URL) || !strings.Contains(d.URL, "/api/webhooks/") {
		return "al_err_discord_url"
	}
	return ""
}

// Webhook posts each alert as JSON.
type Webhook struct {
	On   bool
	URL  string
	Auth string // the Authorization header, optional
}

func (h *Webhook) Name() string  { return MethodWebhook }
func (h *Webhook) Enabled() bool { return h.On }
func (h *Webhook) SetUp() bool   { return h.URL != "" }
func (h *Webhook) Problem() string {
	switch {
	case !webAddress(h.URL):
		return "al_err_webhook_url"
	case len(h.Auth) > 1000 || strings.ContainsAny(h.Auth, "\r\n"):
		return "al_err_webhook_auth"
	}
	return ""
}

// Host is the webhook's host and port, for the status line (the path may hold a token).
func (h *Webhook) Host() string {
	u, err := url.Parse(h.URL)
	if err != nil {
		return ""
	}
	return u.Host
}

// methodError is one method's failure, for the delivery error.
func methodError(name string, err error) string {
	return fmt.Sprintf("%s: %v", MethodLabel(name), err)
}
