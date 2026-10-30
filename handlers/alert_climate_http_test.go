package handlers_test

// Settings > Alerts, Climate alerts and Recent alerts, and the box on Grow
// setup, through the real engine with a signed-in session.

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
)

func climateForm() map[string]any {
	return map[string]any{"mode": "email", "safety_readings": 3, "window_minutes": 15, "drift_hours": 4,
		"pause_minutes": 5, "repeat_critical_minutes": 0, "repeat_warning_hours": 6}
}

func (r *alertRoutes) settingsPage(t *testing.T) string {
	t.Helper()
	resp := r.c.Get("/settings")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

func TestAlertClimateHTTP_DefaultsThenSave(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)

	cfg, err := alerts.LoadClimateConfig(r.server.DB)
	require.NoError(t, err)
	assert.Equal(t, alerts.ModeDryRun, cfg.Mode, "upgrades start in dry run")
	assert.Equal(t, alerts.DefaultTiming(), cfg.Timing)
	page := r.settingsPage(t)
	assert.Contains(t, page, "Climate alerts: Dry run")
	assert.Contains(t, page, "alert after 2 readings in a row")
	assert.Contains(t, page, "No alerts yet.")

	code, _ := r.post(t, "/settings/alerts/climate", climateForm())
	require.Equal(t, http.StatusOK, code)
	cfg, err = alerts.LoadClimateConfig(r.server.DB)
	require.NoError(t, err)
	assert.Equal(t, alerts.ModeEmail, cfg.Mode)
	assert.Equal(t, 3, cfg.Timing.SafetyReadings)
	assert.Equal(t, 15.0, cfg.Timing.Window.Minutes())
	assert.Equal(t, 4.0, cfg.Timing.Drift.Hours())
	assert.Equal(t, 5.0, cfg.Timing.Pause.Minutes())
	assert.Zero(t, cfg.Timing.RepeatCritical)
	assert.Equal(t, 6.0, cfg.Timing.RepeatWarning.Hours())
	assert.Equal(t, "en", cfg.Language)
	page = r.settingsPage(t)
	assert.Contains(t, page, "Climate alerts: Send email")
	assert.Contains(t, page, "alert after 3 readings in a row")
	assert.Contains(t, page, `data-email-ready="false"`)
}

func TestAlertClimateHTTP_Refuses(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	for what, mod := range map[string]func(m map[string]any){
		"bad mode":        func(m map[string]any) { m["mode"] = "loud" },
		"no readings":     func(m map[string]any) { m["safety_readings"] = 0 },
		"long window":     func(m map[string]any) { m["window_minutes"] = 61 },
		"short window":    func(m map[string]any) { m["window_minutes"] = 4 },
		"no drift":        func(m map[string]any) { m["drift_hours"] = 0 },
		"long pause":      func(m map[string]any) { m["pause_minutes"] = 61 },
		"negative repeat": func(m map[string]any) { m["repeat_critical_minutes"] = -1 },
		"long repeat":     func(m map[string]any) { m["repeat_warning_hours"] = 73 },
	} {
		form := climateForm()
		mod(form)
		code, _ := r.post(t, "/settings/alerts/climate", form)
		assert.Equal(t, http.StatusBadRequest, code, what)
	}
	cfg, err := alerts.LoadClimateConfig(r.server.DB)
	require.NoError(t, err)
	assert.Equal(t, alerts.ModeDryRun, cfg.Mode, "nothing saved")
}

func TestAlertClimateHTTP_RecentAlertsOnThePage(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	_, err := r.server.DB.Exec(`INSERT INTO alert_event (kind, zone_id, zone_name, measure, side, value, limit_value, started_at, notified_at, delivery, delivery_error)
		VALUES ('safety', 1, 'Tent <1>', 'temp', 'high', 35, 32.2, '2026-10-30 14:00:00', '2026-10-30 14:00:00', 'failed', 'mail server said no')`)
	require.NoError(t, err)
	_, err = r.server.DB.Exec(`INSERT INTO alert_event (kind, zone_id, zone_name, measure, side, value, limit_value, started_at, notified_at, resolved_at, delivery)
		VALUES ('stage', 1, 'Tent <1>', 'vpd', 'low', 1.1, 1.2, '2026-10-30 12:00:00', '2026-10-30 12:00:00', '2026-10-30 13:00:00', 'dry_run')`)
	require.NoError(t, err)

	page := r.settingsPage(t)
	assert.Contains(t, page, "Temperature high in Tent &lt;1&gt;", "names are escaped")
	assert.Contains(t, page, "95 °F, at or above the safety limit of 90 °F")
	assert.Contains(t, page, "mail server said no")
	assert.Contains(t, page, "Email failed")
	assert.Contains(t, page, "VPD low in Tent")
	assert.Contains(t, page, "Dry run")
	assert.Contains(t, page, "Ended")
}
