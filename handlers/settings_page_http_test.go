package handlers_test

// The Settings page (mockup ST1): sections in a side menu, the settings
// that moved between sections, zone counts, and the side menu on Manage
// sensors.

import (
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

func (e *sensorEnv) page(t *testing.T, path string) string {
	t.Helper()
	resp := e.client.Get(path)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, path)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

func TestSettingsPage_SectionsAndMovedSettings(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	body := e.page(t, "/settings")

	for _, sec := range []string{"general", "integrations", "grow", "appearance", "cameras", "backup", "logs"} {
		assert.Containsf(t, body, `data-sec="`+sec+`"`, "section %s", sec)
	}
	assert.Contains(t, body, `href="/sensors/manage"`, "Sensors & devices is in the menu")
	assert.Contains(t, body, "settings-page.js")
	assert.NotContains(t, body, `id="settingsTabs"`, "the old tabs are gone")

	// Every setting the auto-save sends is on the page and marked for it.
	for _, id := range []string{"aciEnabled", "ecEnabled", "cannadbEnabled", "cannadbBaseUrl", "straincompassEnabled",
		"straincompassBaseUrl", "straincompassApiKey", "pollingInterval", "guestMode", "streamGrabEnabled",
		"streamGrabInterval", "disableApiIngest", "sensorRetentionDays", "logLevel", "maxBackupSizeMB", "timezone", "units"} {
		re := regexp.MustCompile(`id="` + id + `"[^>]*data-autosave`)
		assert.Truef(t, re.MatchString(body), "%s is auto-saved", id)
	}
	assert.NotContains(t, body, `type="submit" form="settingsForm"`, "no Save buttons")
	assert.NotContains(t, body, "Save Settings", "no Save buttons")

	// Settings that moved: polling into Integrations, retention into Backup,
	// log level into Logs.
	sectionOf := func(id string) string {
		m := regexp.MustCompile(`<section class="st-sec" data-sec="([a-z]+)"`).FindAllStringSubmatchIndex(body, -1)
		at := regexp.MustCompile(`id="` + id + `"`).FindStringIndex(body)
		require.NotNil(t, at, id)
		sec := ""
		for _, s := range m {
			if s[0] < at[0] {
				sec = body[s[2]:s[3]]
			}
		}
		return sec
	}
	assert.Equal(t, "integrations", sectionOf("pollingInterval"))
	assert.Equal(t, "backup", sectionOf("sensorRetentionDays"))
	assert.Equal(t, "logs", sectionOf("logLevel"))
	assert.Equal(t, "general", sectionOf("units"))
}

func TestSettingsPage_ZoneCounts(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	// Added through the app so its zone list (kept in memory) includes it.
	resp := e.client.SessionPostJSON(t, "/zones", e.token, map[string]any{"zone_name": "Count Tent"})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var tent int
	require.NoError(t, e.db.QueryRow(`SELECT id FROM zones WHERE name = 'Count Tent'`).Scan(&tent))
	livingPlant(t, e, "Growing One", tent, "Veg")
	livingPlant(t, e, "Growing Two", tent, "Flower")
	livingPlant(t, e, "Finished", tent, "Dead")
	for _, kind := range []string{"a", "b", "c"} {
		sid := testutil.SeedSensor(t, e.db, "acinfinity", "D", kind)
		e.exec(t, `UPDATE sensors SET zone_id = $1 WHERE id = $2`, tent, sid)
	}

	body := e.page(t, "/settings")
	row := regexp.MustCompile(`(?s)<b>Count Tent</b></td>\s*<td>(\d+)</td>\s*<td>(\d+)</td>`).FindStringSubmatch(body)
	require.NotNil(t, row, "zone row with counts")
	assert.Equal(t, "2", row[1], "living plants only")
	assert.Equal(t, "3", row[2], "sensors in the zone")
}

func TestManageSensors_HasSettingsMenu(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	body := e.page(t, "/sensors/manage")
	assert.Contains(t, body, `id="stSide"`)
	assert.Contains(t, body, `href="/settings#grow"`)
	assert.Contains(t, body, `action="/settings"`, "searching from here goes to Settings")
}
