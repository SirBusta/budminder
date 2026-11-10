package handlers_test

// Settings > Alerts, the notification methods besides email
// (handlers/alert_methods.go), through the real engine with a signed-in session.

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
	"isley/handlers"
	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

const (
	testPoUser  = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	testPoToken = "azGDORePK8gMaC0QOYAMyEEuzJnyUi"
)

func (r *alertRoutes) secret(t *testing.T, name string) string {
	t.Helper()
	v, err := alerts.OpenPassword(r.server.DataDir, r.setting(t, name))
	require.NoError(t, err)
	return v
}

func TestAlertMethodsHTTP_PushoverSaveSealsTheTokenAndKeepsIt(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	code, out := r.post(t, "/settings/alerts/methods/pushover", map[string]any{"on": true, "user": testPoUser, "token": testPoToken, "devices": "iphone ,pixel", "priority": "2"})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, true, out["ready"])
	assert.Equal(t, "1", r.setting(t, alerts.SettingPushoverOn))
	assert.Equal(t, "iphone, pixel", r.setting(t, alerts.SettingPushoverDevices))
	assert.Equal(t, "2", r.setting(t, alerts.SettingPushoverPriority))
	assert.NotContains(t, r.setting(t, alerts.SettingPushoverToken), testPoToken, "stored encrypted")
	assert.Equal(t, testPoToken, r.secret(t, alerts.SettingPushoverToken))
	methods := out["methods"].(map[string]any)
	assert.Equal(t, true, methods["pushoverTokenSet"])

	// A blank token keeps the saved one.
	code, out = r.post(t, "/settings/alerts/methods/pushover", map[string]any{"on": true, "user": testPoUser, "token": "", "priority": "1"})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, testPoToken, r.secret(t, alerts.SettingPushoverToken))

	// The page says a token is saved but never shows it.
	page := r.c.Get("/settings")
	body, _ := io.ReadAll(page.Body)
	testutil.DrainAndClose(page)
	assert.Contains(t, string(body), `data-m="pushover"`)
	assert.Contains(t, string(body), testPoUser)
	assert.NotContains(t, string(body), testPoToken)
}

func TestAlertMethodsHTTP_TurningOnChecksFields(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	code, out := r.post(t, "/settings/alerts/methods/pushover", map[string]any{"on": true, "user": "nope", "token": testPoToken, "priority": "1"})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, out["error"], "user key")
	assert.Empty(t, r.setting(t, alerts.SettingPushoverOn), "nothing saved")

	// Off, what's typed is kept as it is.
	code, _ = r.post(t, "/settings/alerts/methods/ntfy", map[string]any{"on": false, "topic": "half typed", "priority": "4"})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "half typed", r.setting(t, alerts.SettingNtfyTopic))
	assert.Equal(t, alerts.DefaultNtfyServer, r.setting(t, alerts.SettingNtfyServer))

	code, _ = r.post(t, "/settings/alerts/methods/slack", map[string]any{"on": false})
	assert.Equal(t, http.StatusBadRequest, code, "unknown method")
	code, _ = r.post(t, "/settings/alerts/methods/discord", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code, "on is required")
}

func TestAlertMethodsHTTP_RefusedWhileNothingCanAlert(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	_, err := r.server.DB.Exec(`DELETE FROM climate_target`)
	require.NoError(t, err)
	f := fakes.NewFakeNotify(t)
	code, out := r.post(t, "/settings/alerts/methods/webhook", map[string]any{"on": true, "url": f.URL + "/hook"})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "nothing_set", out["code"])
}

func TestAlertMethodsHTTP_RemoveOptionalSecrets(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	f := fakes.NewFakeNotify(t)
	code, out := r.post(t, "/settings/alerts/methods/ntfy", map[string]any{"on": true, "server": f.URL, "topic": "budminder-x1", "token": "tk_1", "priority": "4"})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, "tk_1", r.secret(t, alerts.SettingNtfyToken))
	code, _ = r.post(t, "/settings/alerts/methods/ntfy", map[string]any{"on": true, "server": f.URL, "topic": "budminder-x1", "priority": "4", "remove_token": true})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, r.setting(t, alerts.SettingNtfyToken))

	code, _ = r.post(t, "/settings/alerts/methods/webhook", map[string]any{"on": true, "url": f.URL + "/h", "auth": "Bearer a"})
	require.Equal(t, http.StatusOK, code)
	code, _ = r.post(t, "/settings/alerts/methods/webhook", map[string]any{"on": true, "remove_auth": true})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, r.setting(t, alerts.SettingWebhookAuth))
	assert.Equal(t, f.URL+"/h", r.secret(t, alerts.SettingWebhookURL), "the URL stays")
}

func TestAlertMethodsHTTP_SendTest(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	f := fakes.NewFakeNotify(t)
	r.server.ConfigStore.SetPushoverAPI(f.URL + "/1/messages.json")

	code, out := r.post(t, "/settings/alerts/methods/pushover/test", map[string]any{"on": false, "user": testPoUser, "token": testPoToken, "priority": "1"})
	require.Equal(t, http.StatusOK, code, out)
	assert.Contains(t, out["message"], "Check your phone")
	reqs := f.Requests()
	require.Len(t, reqs, 1)
	form, _ := url.ParseQuery(reqs[0].Body)
	assert.Equal(t, "BudMinder test notification", form.Get("title"))
	assert.Equal(t, "1", form.Get("priority"), "a test uses the chosen priority")
	assert.Empty(t, r.setting(t, alerts.SettingPushoverUser), "a test saves nothing")

	code, out = r.post(t, "/settings/alerts/methods/webhook/test", map[string]any{"on": false, "url": f.URL + "/hook"})
	require.Equal(t, http.StatusOK, code, out)
	assert.Contains(t, out["message"], "200 OK")
	assert.Contains(t, f.Requests()[1].Body, `"event":"test"`)

	f.SetStatus(404, `{"message":"Unknown Webhook"}`)
	code, out = r.post(t, "/settings/alerts/methods/discord/test", map[string]any{"on": false, "url": f.URL + "/api/webhooks/1/tok"})
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Contains(t, out["error"], "Unknown Webhook")

	code, out = r.post(t, "/settings/alerts/methods/discord/test", map[string]any{"on": false, "url": "https://example.com/x"})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, out["error"], "Discord")
}

func TestAlertMethodsHTTP_SendTestLater(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	f := fakes.NewFakeNotify(t)
	r.server.ConfigStore.SetPushoverAPI(f.URL + "/1/messages.json")
	body := map[string]any{"on": false, "user": testPoUser, "token": testPoToken, "priority": "1"}

	// "Send in 30 seconds" asks for at most 30.
	for _, d := range []int{31, -1} {
		body["delay"] = d
		code, _ := r.post(t, "/settings/alerts/methods/pushover/test", body)
		assert.Equal(t, http.StatusBadRequest, code, "delay %d", d)
	}
	assert.Empty(t, f.Requests(), "nothing sent")

	// A bad field is refused at once, not after the wait.
	body["delay"] = 30
	body["user"] = "nope"
	start := time.Now()
	code, _ := r.post(t, "/settings/alerts/methods/pushover/test", body)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Less(t, time.Since(start), 5*time.Second)

	body["delay"] = 1
	body["user"] = testPoUser
	start = time.Now()
	code, out := r.post(t, "/settings/alerts/methods/pushover/test", body)
	require.Equal(t, http.StatusOK, code, out)
	assert.GreaterOrEqual(t, time.Since(start), time.Second, "waits before sending")
	require.Len(t, f.Requests(), 1)
}

func TestAlertMethodsHTTP_NtfyChoices(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	f := fakes.NewFakeNotify(t)

	// Default (3) is no longer offered: Regular is 4, Wake me up 5.
	code, out := r.post(t, "/settings/alerts/methods/ntfy", map[string]any{"on": true, "server": f.URL, "topic": "budminder-x1", "priority": "3"})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, out["error"], "Reload the page")
	code, out = r.post(t, "/settings/alerts/methods/ntfy", map[string]any{"on": true, "server": f.URL, "topic": "budminder-x1", "priority": "5"})
	require.Equal(t, http.StatusOK, code, out)

	// Saved as 3 before 2.5: shown and sent as Regular.
	_, err := r.server.DB.Exec(`UPDATE settings SET value = '3' WHERE name = $1`, alerts.SettingNtfyPriority)
	require.NoError(t, err)
	page := r.settingsPage(t)
	i := strings.Index(page, `data-m="ntfy"`)
	require.Positive(t, i)
	section := page[i:]
	section = section[:strings.Index(section, "</section>")]
	assert.Contains(t, section, `data-pri value="4"`)
	m, err := alerts.LoadMethods(r.server.DB, r.server.DataDir, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, 4, m.Ntfy.Priority)
}

func TestAlertMethodsHTTP_HomeAssistantPhones(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	code, _ := r.post(t, "/settings/alerts/methods/ha", map[string]any{"on": true, "phones": []string{"mobile_app_daves_iphone"}, "priority": "critical"})
	assert.Equal(t, http.StatusBadRequest, code, "no Home Assistant connection yet")

	ha := fakes.FakeHomeAssistant(t, "states")
	ha.Services.Store([]byte(`[{"domain":"light","services":{"turn_on":{}}},{"domain":"notify","services":{"mobile_app_pixel_8":{},"mobile_app_daves_iphone":{},"persistent_notification":{}}}]`))
	r.server.ConfigStore.SetHAURL(ha.URL)
	r.server.ConfigStore.SetHAToken(fakes.HomeAssistantToken)

	resp := r.c.Get("/settings/alerts/ha-phones")
	out := decodeJSON(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, out)
	phones := out["phones"].([]any)
	require.Len(t, phones, 2)
	ids := []string{phones[0].(map[string]any)["id"].(string), phones[1].(map[string]any)["id"].(string)}
	assert.ElementsMatch(t, []string{"mobile_app_pixel_8", "mobile_app_daves_iphone"}, ids)

	code, out = r.post(t, "/settings/alerts/methods/ha", map[string]any{"on": true, "phones": []string{"mobile_app_daves_iphone"}, "names": []string{"Dave's iPhone"}, "priority": "critical"})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, "mobile_app_daves_iphone", r.setting(t, alerts.SettingHAAppPhones))
	methods := out["methods"].(map[string]any)
	saved := methods["haPhones"].([]any)
	require.Len(t, saved, 1)
	assert.Equal(t, "Dave's iPhone", saved[0].(map[string]any)["name"])

	code, out = r.post(t, "/settings/alerts/methods/ha", map[string]any{"on": true, "phones": []string{"notify.everyone"}, "priority": "critical"})
	assert.Equal(t, http.StatusBadRequest, code, out)
}

func TestAlertMethodsHTTP_LinkHasItsOwnField(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	code, _ := r.post(t, "/settings/alerts/link", map[string]any{"link": "http://192.168.1.10:8080/"})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "http://192.168.1.10:8080", r.setting(t, alerts.SettingLinkAddress))
	code, _ = r.post(t, "/settings/alerts/link", map[string]any{"link": "javascript:alert(1)"})
	assert.Equal(t, http.StatusBadRequest, code)

	// Saving email without a link keeps the address.
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	form := mailForm(f)
	delete(form, "link")
	form["on"] = false
	code, out := r.post(t, "/settings/alerts/email", form)
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, "http://192.168.1.10:8080", r.setting(t, alerts.SettingLinkAddress))
}

func TestAlertMethodsHTTP_SecretsStayOutOfBackups(t *testing.T) {
	t.Parallel()
	for _, s := range alerts.SealedSettings {
		assert.True(t, handlers.IsSecretSettingForTest(s), s)
	}
}
