package handlers_test

// Settings > Alerts: the page's cards, Recent alerts' Clear and Restore, and
// refusing to turn notifications on with nothing to send, through the real
// engine with a signed-in session.

import (
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
	"isley/handlers"
	"isley/tests/testutil/fakes"
)

func (r *alertRoutes) settingsPage(t *testing.T) string {
	t.Helper()
	resp := r.c.Get("/settings")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

func TestAlertsPage_Cards(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	page := r.settingsPage(t)
	for _, want := range []string{
		"What sends an alert notification",
		"Climate reading reaches a Low or High limit for 15 consecutive minutes",
		"How often alert notifications are sent",
		"Zone status",
		"Notification methods",
		"No readings received for 15 minutes",
		"Climate targets and limits",
		"Default Targets and limits are guidelines only.",
		"If you receive too many alert notifications, widen your limits.",
		`id="alEvData"`,
	} {
		assert.Contains(t, page, want)
	}
	for _, gone := range []string{"Dry run", "Drifting notice", "Safety limit readings", "Suggested targets", "alClimateCard"} {
		assert.NotContains(t, page, gone)
	}
}

func TestAlertsPage_RecentAlerts(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	_, err := r.server.DB.Exec(`INSERT INTO alert_event (kind, zone_id, zone_name, measure, side, value, limit_value, started_at, notified_at, delivery, delivery_error)
		VALUES ('stage', 1, 'Tent <1>', 'temp', 'high', 35, 32.2, '2026-10-30 14:00:00', '2026-10-30 14:00:00', 'failed', 'mail server said no')`)
	require.NoError(t, err)
	_, err = r.server.DB.Exec(`INSERT INTO alert_event (kind, zone_id, zone_name, measure, side, value, limit_value, started_at, notified_at, resolved_at, delivery)
		VALUES ('stage', 1, 'Tent', 'vpd', 'low', 1.1, 1.2, '2026-10-30 12:00:00', '2026-10-30 12:00:00', '2026-10-30 13:00:00', 'off')`)
	require.NoError(t, err)
	_, err = r.server.DB.Exec(`INSERT INTO alert_event (kind, zone_id, zone_name, measure, side, value, limit_value, started_at, notified_at, resolved_at, delivery)
		VALUES ('stage', 1, 'Tent', 'rh', 'high', 75, 70, '2026-10-30 10:00:00', '2026-10-30 10:00:00', '2026-10-30 11:00:00', 'paused')`)
	require.NoError(t, err)

	page := r.settingsPage(t)
	assert.Contains(t, page, "Not sent: notifications are paused")
	assert.NotContains(t, page, "Tent <1>", "names are escaped")
	assert.Contains(t, page, "Temperature high in Tent")
	assert.Contains(t, page, "High limit: 90 °F.")
	assert.Contains(t, page, "mail server said no")
	assert.Contains(t, page, "Email failed")
	assert.Contains(t, page, "VPD low in Tent")
	assert.Contains(t, page, "Not sent: notifications are off")
	assert.Contains(t, page, "Ended")
}

func TestAlertsPage_ClearAndRestore(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	db := r.server.DB
	ins := `INSERT INTO alert_event (kind, zone_id, measure, side, started_at, notified_at, resolved_at) VALUES ('stage', 1, $1, 'low', '2026-10-30 12:00:00', '2026-10-30 12:00:00', $2) RETURNING id`
	var active, ended, other int
	require.NoError(t, db.QueryRow(ins, "temp", nil).Scan(&active))
	require.NoError(t, db.QueryRow(ins, "rh", "2026-10-30 13:00:00").Scan(&ended))
	require.NoError(t, db.QueryRow(ins, "vpd", "2026-10-30 13:00:00").Scan(&other))
	cleared := func() int {
		list, err := handlers.RecentAlerts(db, true, 10)
		require.NoError(t, err)
		return len(list)
	}
	post := func(path string) {
		code, _ := r.post(t, path, map[string]any{})
		require.Equal(t, http.StatusOK, code, path)
	}

	post("/settings/alerts/events/" + strconv.Itoa(active) + "/clear")
	assert.Zero(t, cleared(), "an active alert can't be cleared")

	post("/settings/alerts/events/" + strconv.Itoa(ended) + "/clear")
	assert.Equal(t, 1, cleared())
	post("/settings/alerts/events/" + strconv.Itoa(ended) + "/restore")
	assert.Zero(t, cleared())

	post("/settings/alerts/clear-ended")
	assert.Equal(t, 2, cleared(), "both ended alerts, not the active one")

	code, _ := r.post(t, "/settings/alerts/events/nope/clear", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestAlertsPage_RefusesToTurnOnWithNothingSet(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	db := r.server.DB
	_, err := db.Exec(`UPDATE climate_target SET low = NULL, high = NULL`)
	require.NoError(t, err)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)

	code, out := r.post(t, "/settings/alerts/email", mailForm(f))
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "nothing_set", out["code"])
	assert.NotEqual(t, "1", r.setting(t, alerts.SettingEmailOn))

	// Off can still be saved, and a limit makes on work.
	form := mailForm(f)
	form["on"] = false
	code, _ = r.post(t, "/settings/alerts/email", form)
	assert.Equal(t, http.StatusOK, code)
	_, err = db.Exec(`UPDATE climate_target SET high = 90 WHERE measure = 'temp' AND phase = '' AND lights = 'on' AND status_id = (SELECT id FROM plant_status WHERE status = 'Veg')`)
	require.NoError(t, err)
	code, _ = r.post(t, "/settings/alerts/email", mailForm(f))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "1", r.setting(t, alerts.SettingEmailOn))
	assert.Equal(t, "en", r.setting(t, alerts.SettingLanguage), "emails use the language in use")

	// Already on: other fields still save, with a warning on the page instead.
	_, err = db.Exec(`UPDATE climate_target SET low = NULL, high = NULL`)
	require.NoError(t, err)
	code, _ = r.post(t, "/settings/alerts/email", mailForm(f))
	assert.Equal(t, http.StatusOK, code)
}
