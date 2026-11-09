package alerts

// Senders for Pushover, the Home Assistant app, ntfy, Discord and webhooks
// (settings in notify.go). Errors never carry a URL, token or webhook address:
// they go to the log and to Recent alerts.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// UserAgent identifies BudMinder to notification services. main sets the full
// "BudMinder/<version>" string at startup.
var UserAgent = "BudMinder"

// PushoverAPI is Pushover's message address.
const PushoverAPI = "https://api.pushover.net/1/messages.json"

// Pushover Emergency priority: repeat every 5 minutes for up to 3 hours.
const (
	pushoverRetry  = 300
	pushoverExpire = 10800
)

const pushTimeout = 20 * time.Second

// pushClient never follows redirects: a redirect would send a token or
// webhook request somewhere the user didn't enter.
var pushClient = &http.Client{
	Timeout:       pushTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// cut shortens s to at most n characters (runes).
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// do sends a request and returns the status, or an error with the service's
// own words. The address never appears in the error.
func do(ctx context.Context, req *http.Request) (int, string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, pushTimeout)
		defer cancel()
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", UserAgent)
	resp, err := pushClient.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, "", fmt.Errorf("could not connect: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	status := strconv.Itoa(resp.StatusCode) + " " + http.StatusText(resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, status, fmt.Errorf("HTTP %s%s", status, serviceWords(body))
	}
	return resp.StatusCode, status, nil
}

// serviceWords picks the error message out of a service's answer.
func serviceWords(body []byte) string {
	var v struct {
		Errors  []string `json:"errors"`  // Pushover
		Error   string   `json:"error"`   // ntfy
		Message string   `json:"message"` // Discord, Home Assistant
	}
	if json.Unmarshal(body, &v) == nil {
		switch {
		case len(v.Errors) > 0:
			return ": " + strings.Join(v.Errors, "; ")
		case v.Error != "":
			return ": " + v.Error
		case v.Message != "":
			return ": " + v.Message
		}
	}
	s := strings.TrimSpace(string(body))
	if s == "" || strings.HasPrefix(s, "<") {
		return ""
	}
	return ": " + cut(strings.Join(strings.Fields(s), " "), 200)
}

func postJSON(ctx context.Context, address string, v any, header http.Header) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return "", errors.New("the address is not valid")
	}
	for k, vs := range header {
		req.Header[k] = vs
	}
	req.Header.Set("Content-Type", "application/json")
	_, status, err := do(ctx, req)
	return status, err
}

// Notify sends to Pushover. All clears use Normal priority.
func (p *Pushover) Notify(ctx context.Context, n Notice) error {
	if k := p.Problem(); k != "" {
		return fmt.Errorf("settings incomplete (%s)", k)
	}
	f := url.Values{}
	f.Set("token", p.Token)
	f.Set("user", p.User)
	f.Set("title", cut(n.Title, 250))
	f.Set("message", cut(orDash(n.Text), 1024))
	if d := SplitList(p.Devices); len(d) > 0 {
		f.Set("device", strings.Join(d, ","))
	}
	pri := 0
	if n.Urgent {
		pri = p.Priority
	}
	f.Set("priority", strconv.Itoa(pri))
	if pri == 2 {
		f.Set("retry", strconv.Itoa(pushoverRetry))
		f.Set("expire", strconv.Itoa(pushoverExpire))
	}
	if n.URL != "" && len(n.URL) <= 512 {
		f.Set("url", n.URL)
		f.Set("url_title", cut(n.LinkText, 100))
	}
	api := p.API
	if api == "" {
		api = PushoverAPI
	}
	req, err := http.NewRequest(http.MethodPost, api, strings.NewReader(f.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, _, err = do(ctx, req)
	return err
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// Notify sends to every chosen phone. Critical sounds on iPhones (push.sound)
// and Android phones (alarm stream) when the phone is muted.
func (h *HAApp) Notify(ctx context.Context, n Notice) error {
	if k := h.Problem(); k != "" {
		return fmt.Errorf("settings incomplete (%s)", k)
	}
	data := map[string]any{"group": "budminder", "tag": "budminder"}
	if n.URL != "" {
		data["url"] = n.URL         // iPhone
		data["clickAction"] = n.URL // Android
	}
	if n.Urgent && h.Priority == HAPriorityCritical {
		data["push"] = map[string]any{"sound": map[string]any{"name": "default", "critical": 1, "volume": 1.0}}
		data["ttl"] = 0
		data["priority"] = "high"
		data["channel"] = "alarm_stream"
	}
	body := map[string]any{"title": n.Title, "message": orDash(n.Text), "data": data}
	header := http.Header{"Authorization": {"Bearer " + h.Token}}
	var fails []string
	for _, phone := range h.Phones {
		if _, err := postJSON(ctx, h.URL+"/api/services/notify/"+phone, body, header); err != nil {
			fails = append(fails, phone+": "+err.Error())
		}
	}
	if len(fails) > 0 {
		return errors.New(strings.Join(fails, "; "))
	}
	return nil
}

// Notify publishes to the ntfy topic. All clears use Default priority.
func (t *Ntfy) Notify(ctx context.Context, n Notice) error {
	if k := t.Problem(); k != "" {
		return fmt.Errorf("settings incomplete (%s)", k)
	}
	pri, tag := 3, "white_check_mark"
	if n.Urgent {
		pri, tag = t.Priority, "warning"
	}
	msg := map[string]any{"topic": t.Topic, "title": cut(n.Title, 250), "message": cut(orDash(n.Text), 4000),
		"priority": pri, "tags": []string{tag}}
	if n.URL != "" {
		msg["click"] = n.URL
	}
	var header http.Header
	if t.Token != "" {
		header = http.Header{"Authorization": {"Bearer " + t.Token}}
	}
	_, err := postJSON(ctx, strings.TrimRight(t.Server, "/"), msg, header)
	return err
}

// Discord embed colors.
const (
	discordRed   = 0xdc2626
	discordGreen = 0x16a34a
)

// Notify posts one embed to the Discord channel. Mentions are switched off,
// so text from sensor or zone names can't ping anyone.
func (d *Discord) Notify(ctx context.Context, n Notice) error {
	if k := d.Problem(); k != "" {
		return fmt.Errorf("settings incomplete (%s)", k)
	}
	color := discordGreen // only all clears
	if n.Urgent {
		color = discordRed
	}
	desc := n.Text
	if n.URL != "" {
		desc += "\n[" + n.LinkText + "](" + n.URL + ")"
	}
	embed := map[string]any{"title": cut(n.Title, 256), "description": cut(orDash(desc), 4000), "color": color}
	if n.URL != "" {
		embed["url"] = n.URL
	}
	body := map[string]any{"username": "BudMinder", "embeds": []any{embed}, "allowed_mentions": map[string]any{"parse": []string{}}}
	_, err := postJSON(ctx, d.URL, body, nil)
	return err
}

// Notify posts each alert as its own request.
func (h *Webhook) Notify(ctx context.Context, n Notice) error {
	_, err := h.Send(ctx, n)
	return err
}

// Send posts each alert and returns the last answer's status, e.g. "200 OK".
func (h *Webhook) Send(ctx context.Context, n Notice) (string, error) {
	if k := h.Problem(); k != "" {
		return "", fmt.Errorf("settings incomplete (%s)", k)
	}
	var header http.Header
	if h.Auth != "" {
		header = http.Header{"Authorization": {h.Auth}}
	}
	status := ""
	for _, it := range n.Items {
		s, err := postJSON(ctx, h.URL, it, header)
		if err != nil {
			return "", err
		}
		status = s
	}
	return status, nil
}
