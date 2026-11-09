package alerts_test

// Pushover, the Home Assistant app, ntfy, Discord and webhooks against a fake
// receiver (tests/testutil/fakes/notify.go).

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
	"isley/tests/testutil/fakes"
)

const (
	poUser  = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	poToken = "azGDORePK8gMaC0QOYAMyEEuzJnyUi"
)

func words() alerts.Words {
	return alerts.Words{T: func(k string) string {
		switch k {
		case "al_push_more":
			return "{title} and {n} more"
		case "al_ev_high":
			return "{measure} high in {zone}"
		case "al_ev_ok":
			return "{measure} back to normal in {zone}"
		case "al_measure_temp":
			return "Temperature"
		case "al_line_stage_high":
			return "{measure} is {value}, at or above the High limit ({limit})."
		case "al_ev_view":
			return "View readings"
		}
		return k
	}, Units: "imperial", Loc: time.UTC}
}

func f64(v float64) *float64 { return &v }

// tempAlert is a Temperature high alert at 35 °C (95 °F) over a 32.2 °C limit.
func tempAlert(id int, pending string) alerts.Event {
	return alerts.Event{ID: id, Kind: alerts.KindStage, Zone: "Tent", Measure: "temp", Side: "high", Value: f64(35), Limit: f64(32.2),
		Started: time.Date(2026, 11, 8, 21, 18, 0, 0, time.UTC), Pending: pending}
}

func TestBuildNotice_OneAlertAndAGroup(t *testing.T) {
	t.Parallel()
	n := alerts.BuildNotice(words(), []alerts.Event{tempAlert(7, alerts.NoticeFired)}, "http://bm:8080")
	assert.Equal(t, "Temperature high in Tent", n.Title)
	assert.True(t, strings.HasPrefix(n.Text, "Temperature is 95 °F, at or above the High limit (90 °F)."), n.Text)
	assert.Equal(t, "http://bm:8080/sensors#alert=7", n.URL)
	assert.Equal(t, "View readings", n.LinkText)
	assert.True(t, n.Urgent)
	require.Len(t, n.Items, 1)
	it := n.Items[0]
	assert.Equal(t, alerts.EventStart, it.Event)
	assert.InDelta(t, 95, *it.Value, 0.01, "in the Settings units")
	assert.Equal(t, "°F", it.Unit)
	assert.Contains(t, n.Email.Text, "http://bm:8080/settings#alerts")

	ok := tempAlert(8, alerts.NoticeResolved)
	n = alerts.BuildNotice(words(), []alerts.Event{ok, tempAlert(9, alerts.NoticeRepeat)}, "")
	assert.Equal(t, "Temperature high in Tent and 1 more", n.Title, "alerts before all clears")
	assert.True(t, n.Urgent)
	assert.Empty(t, n.URL, "no BudMinder address, no link")
	assert.Equal(t, alerts.EventReminder, n.Items[0].Event)
	assert.Equal(t, alerts.EventEnd, n.Items[1].Event)

	n = alerts.BuildNotice(words(), []alerts.Event{ok}, "")
	assert.False(t, n.Urgent, "only an all clear")
}

func TestPushover_PriorityAndAllClearIsNormal(t *testing.T) {
	t.Parallel()
	f := fakes.NewFakeNotify(t)
	p := alerts.Pushover{On: true, User: poUser, Token: poToken, Devices: "iphone, pixel", Priority: 2, API: f.URL + "/1/messages.json"}
	require.Empty(t, p.Problem())
	ctx := context.Background()
	require.NoError(t, p.Notify(ctx, alerts.BuildNotice(words(), []alerts.Event{tempAlert(1, alerts.NoticeFired)}, "http://bm")))
	require.NoError(t, p.Notify(ctx, alerts.BuildNotice(words(), []alerts.Event{tempAlert(1, alerts.NoticeResolved)}, "http://bm")))
	reqs := f.Requests()
	require.Len(t, reqs, 2)
	first, _ := url.ParseQuery(reqs[0].Body)
	assert.Equal(t, poToken, first.Get("token"))
	assert.Equal(t, poUser, first.Get("user"))
	assert.Equal(t, "2", first.Get("priority"))
	assert.Equal(t, "300", first.Get("retry"))
	assert.Equal(t, "10800", first.Get("expire"))
	assert.Equal(t, "iphone,pixel", first.Get("device"))
	assert.Equal(t, "http://bm/sensors#alert=1", first.Get("url"))
	second, _ := url.ParseQuery(reqs[1].Body)
	assert.Equal(t, "0", second.Get("priority"), "all clears are always Normal")
	assert.Empty(t, second.Get("retry"))

	f.SetStatus(400, `{"user":"invalid","errors":["user identifier is invalid"],"status":0}`)
	err := p.Notify(ctx, alerts.TestNotice("t", "x", "", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user identifier is invalid")
	assert.NotContains(t, err.Error(), poToken)
}

func TestPushover_Problems(t *testing.T) {
	t.Parallel()
	p := alerts.Pushover{User: poUser, Token: poToken, Priority: 1}
	assert.Empty(t, p.Problem())
	p.User = "short"
	assert.Equal(t, "al_err_po_user", p.Problem())
	p.User, p.Token = poUser, ""
	assert.Equal(t, "al_err_po_token", p.Problem())
	p.Token, p.Devices = poToken, "my phone"
	assert.Equal(t, "al_err_po_devices", p.Problem())
	p.Devices, p.Priority = "", 3
	assert.Equal(t, "al_err_priority", p.Problem())
}

func TestHAApp_CriticalKeysOnlyForAlerts(t *testing.T) {
	t.Parallel()
	f := fakes.NewFakeNotify(t)
	h := alerts.HAApp{On: true, Phones: []string{"mobile_app_daves_iphone", "mobile_app_pixel_8"}, Priority: alerts.HAPriorityCritical, URL: f.URL, Token: "tok"}
	require.Empty(t, h.Problem())
	ctx := context.Background()
	require.NoError(t, h.Notify(ctx, alerts.BuildNotice(words(), []alerts.Event{tempAlert(3, alerts.NoticeFired)}, "http://bm")))
	reqs := f.Requests()
	require.Len(t, reqs, 2, "one request per phone")
	assert.Equal(t, "/api/services/notify/mobile_app_daves_iphone", reqs[0].Path)
	assert.Equal(t, "Bearer tok", reqs[0].Header.Get("Authorization"))
	var body struct {
		Title string         `json:"title"`
		Data  map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(reqs[0].Body), &body))
	assert.Equal(t, "Temperature high in Tent", body.Title)
	assert.Equal(t, "alarm_stream", body.Data["channel"])
	assert.Equal(t, "http://bm/sensors#alert=3", body.Data["url"])
	assert.Equal(t, "http://bm/sensors#alert=3", body.Data["clickAction"])
	assert.Contains(t, reqs[0].Body, `"critical":1`)

	require.NoError(t, h.Notify(ctx, alerts.BuildNotice(words(), []alerts.Event{tempAlert(3, alerts.NoticeResolved)}, "")))
	quiet := f.Requests()[2].Body
	assert.NotContains(t, quiet, "critical")
	assert.NotContains(t, quiet, "alarm_stream")

	f.SetStatus(400, `{"message":"Service notify.mobile_app_pixel_8 not found."}`)
	err := h.Notify(ctx, alerts.TestNotice("t", "x", "", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	assert.Equal(t, "al_err_ha_conn", (&alerts.HAApp{Phones: h.Phones, Priority: "normal"}).Problem())
	assert.Equal(t, "al_err_ha_phones", (&alerts.HAApp{URL: f.URL, Token: "t", Phones: []string{"notify.x"}, Priority: "normal"}).Problem())
}

func TestNtfy_JSONPublishAndToken(t *testing.T) {
	t.Parallel()
	f := fakes.NewFakeNotify(t)
	n := alerts.Ntfy{On: true, Server: f.URL + "/", Topic: "budminder-abc", Token: "tk_secret", Priority: 5}
	require.Empty(t, n.Problem())
	ctx := context.Background()
	require.NoError(t, n.Notify(ctx, alerts.BuildNotice(words(), []alerts.Event{tempAlert(4, alerts.NoticeFired)}, "http://bm")))
	require.NoError(t, n.Notify(ctx, alerts.BuildNotice(words(), []alerts.Event{tempAlert(4, alerts.NoticeResolved)}, "http://bm")))
	reqs := f.Requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "/", reqs[0].Path, "JSON publishing goes to the server's root")
	assert.Equal(t, "Bearer tk_secret", reqs[0].Header.Get("Authorization"))
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(reqs[0].Body), &m))
	assert.Equal(t, "budminder-abc", m["topic"])
	assert.EqualValues(t, 5, m["priority"])
	assert.Equal(t, "http://bm/sensors#alert=4", m["click"])
	require.NoError(t, json.Unmarshal([]byte(reqs[1].Body), &m))
	assert.EqualValues(t, 3, m["priority"], "all clears use Default")

	n.Topic = "has space"
	assert.Equal(t, "al_err_ntfy_topic", n.Problem())
	n.Topic, n.Server = "ok", "ntfy.sh"
	assert.Equal(t, "al_err_ntfy_server", n.Problem())
}

func TestDiscord_EmbedWithoutMentions(t *testing.T) {
	t.Parallel()
	f := fakes.NewFakeNotify(t)
	d := alerts.Discord{On: true, URL: f.URL + "/api/webhooks/123/secret-token"}
	require.Empty(t, d.Problem())
	require.NoError(t, d.Notify(context.Background(), alerts.BuildNotice(words(), []alerts.Event{tempAlert(5, alerts.NoticeFired)}, "http://bm")))
	reqs := f.Requests()
	require.Len(t, reqs, 1)
	assert.Contains(t, reqs[0].Body, `"allowed_mentions":{"parse":[]}`)
	assert.Contains(t, reqs[0].Body, `[View readings](http://bm/sensors#alert=5)`)
	assert.Contains(t, reqs[0].Body, `"color":14427686`, "red for every alert: alerts have no levels")

	f.SetStatus(404, `{"message": "Unknown Webhook", "code": 10015}`)
	err := d.Notify(context.Background(), alerts.TestNotice("t", "x", "", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Unknown Webhook")
	assert.NotContains(t, err.Error(), "secret-token", "the URL is the password")

	assert.Equal(t, "al_err_discord_url", (&alerts.Discord{URL: "https://discord.com/channels/1/2"}).Problem())
}

func TestWebhook_OneRequestPerAlert(t *testing.T) {
	t.Parallel()
	f := fakes.NewFakeNotify(t)
	h := alerts.Webhook{On: true, URL: f.URL + "/hook?key=abc", Auth: "Bearer xyz"}
	require.Empty(t, h.Problem())
	n := alerts.BuildNotice(words(), []alerts.Event{tempAlert(1, alerts.NoticeFired), tempAlert(2, alerts.NoticeResolved)}, "http://bm")
	status, err := h.Send(context.Background(), n)
	require.NoError(t, err)
	assert.Equal(t, "200 OK", status)
	reqs := f.Requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "Bearer xyz", reqs[0].Header.Get("Authorization"))
	assert.Equal(t, "application/json", reqs[0].Header.Get("Content-Type"))
	var it map[string]any
	require.NoError(t, json.Unmarshal([]byte(reqs[0].Body), &it))
	assert.Equal(t, "start", it["event"])
	assert.Equal(t, "Tent", it["zone"])
	assert.Equal(t, "temp", it["measure"])
	assert.EqualValues(t, 95, it["value"])
	assert.Equal(t, "°F", it["unit"])
	assert.Equal(t, "2026-11-08T21:18:00Z", it["started_at"])
	assert.Equal(t, "http://bm/sensors#alert=1", it["url"])
	require.NoError(t, json.Unmarshal([]byte(reqs[1].Body), &it))
	assert.Equal(t, "end", it["event"])

	f.SetStatus(500, "<html>oops</html>")
	_, err = h.Send(context.Background(), n)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "key=abc")

	assert.Equal(t, "al_err_webhook_auth", (&alerts.Webhook{URL: f.URL, Auth: "a\nb"}).Problem())
}

func TestPush_UnreachableErrorHidesTheAddress(t *testing.T) {
	t.Parallel()
	h := alerts.Webhook{URL: "http://127.0.0.1:1/hook/secret-path"}
	_, err := h.Send(context.Background(), alerts.TestNotice("t", "x", "", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect")
	assert.NotContains(t, err.Error(), "secret-path")
}
