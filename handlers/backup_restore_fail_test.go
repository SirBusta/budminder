package handlers_test

// +parallel:serial — a real restore sets config.RestoreInProgress, which is
// process-wide: parallel tests (photo compression, ingest) would see it.

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// restoreViaHTTP restores payload into db the way Backup & data does
// (POST /settings/backup/restore, the real restore path) and waits for it
// to finish.
func restoreViaHTTP(t *testing.T, db *sql.DB, payload handlers.BackupPayload) handlers.RestoreStatus {
	t.Helper()
	js, err := json.Marshal(payload)
	require.NoError(t, err)
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("backup.json")
	require.NoError(t, err)
	_, err = w.Write(js)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))
	const apiKey = "restore-helper-key"
	testutil.SeedAPIKey(t, db, apiKey)

	body, ct := testutil.MultipartBody(t, "backup", "backup.zip", zipBuf.Bytes())
	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	deadline := time.Now().Add(20 * time.Second)
	var status handlers.RestoreStatus
	for time.Now().Before(deadline) {
		if status = server.BackupService.RestoreSnapshot(); !status.InProgress {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.False(t, status.InProgress, "restore did not finish")
	return status
}

// A restore that fails during the sensor_data bulk load must still put
// back the two sensor_data indexes it dropped for the load (M06-05).
func TestBackupHTTP_FailedRestoreKeepsSensorIndexes(t *testing.T) {
	src := testutil.NewTestDB(t)
	seedSampleData(t, src)
	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)
	require.NotEmpty(t, payload.SensorData)
	// The same reading twice: the bulk insert hits the primary key.
	payload.SensorData = append(payload.SensorData, payload.SensorData[0])

	db := testutil.NewTestDB(t)
	status := restoreViaHTTP(t, db, payload)
	require.NotEmpty(t, status.Error, "the duplicate reading should fail the restore")

	for _, idx := range []string{"idx_sensor_data_sensor_id", "idx_sensor_data_create_dt"} {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = $1`, idx).Scan(&n))
		assert.Equalf(t, 1, n, "index %s should be back after the failed restore", idx)
	}
}

// Restored reading times keep SQLite's own format, so they sort with the
// readings that arrive after the restore (M06-04).
func TestBackupHTTP_RestoreKeepsSQLiteReadingTimeFormat(t *testing.T) {
	src := testutil.NewTestDB(t)
	seedSampleData(t, src)
	_, err := src.Exec(`UPDATE sensor_data SET create_dt = '2026-10-20 14:05:00'`)
	require.NoError(t, err)

	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)

	dst := testutil.NewTestDB(t)
	_, err = dst.Exec(`INSERT INTO settings (name, value) VALUES ('stale', 'should-be-gone')`)
	require.NoError(t, err)
	status := restoreViaHTTP(t, dst, payload)
	require.Empty(t, status.Error)

	rows, err := dst.Query(`SELECT CAST(create_dt AS TEXT) FROM sensor_data`)
	require.NoError(t, err)
	defer rows.Close()
	n := 0
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		assert.Equal(t, "2026-10-20 14:05:00", s)
		n++
	}
	require.NoError(t, rows.Err())
	assert.Positive(t, n)

	var stale int
	require.NoError(t, dst.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'stale'`).Scan(&stale))
	assert.Zero(t, stale, "rows from before the restore are gone")
}

// Backups leave out the mail password (Settings > Alerts), so a restore
// keeps the one already saved instead of signing the mail server out.
func TestBackupHTTP_RestoreKeepsMailPassword(t *testing.T) {
	src := testutil.NewTestDB(t)
	seedSampleData(t, src)
	_, err := src.Exec(`INSERT INTO settings (name, value) VALUES ('alert_email_username', 'grower@example.com'), ('alert_email_password', 'old-pass')`)
	require.NoError(t, err)
	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)

	dst := testutil.NewTestDB(t)
	_, err = dst.Exec(`INSERT INTO settings (name, value) VALUES ('alert_email_password', 'live-pass')`)
	require.NoError(t, err)
	status := restoreViaHTTP(t, dst, payload)
	require.Empty(t, status.Error)

	var user, pass string
	require.NoError(t, dst.QueryRow(`SELECT value FROM settings WHERE name = 'alert_email_username'`).Scan(&user))
	require.NoError(t, dst.QueryRow(`SELECT value FROM settings WHERE name = 'alert_email_password'`).Scan(&pass))
	assert.Equal(t, "grower@example.com", user, "the rest of the mail settings come from the backup")
	assert.Equal(t, "live-pass", pass)
}

// Climate targets travel with a backup. A backup made before them (no
// climate_target rows) gets them filled from its own VPD ranges.
func TestBackupHTTP_RestoreAlertRules(t *testing.T) {
	src := testutil.NewTestDB(t)
	seedSampleData(t, src)
	sensor := testutil.SeedSensor(t, src, "acinfinity", "D9", "CO2")
	_, err := src.Exec(`INSERT INTO alert_rule (sensor_id, side, limit_value, hold_minutes, enabled) VALUES ($1, 'low', 12.5, 20, 0)`, sensor)
	require.NoError(t, err)
	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)
	require.Len(t, payload.AlertRules, 1)

	dst := testutil.NewTestDB(t)
	status := restoreViaHTTP(t, dst, payload)
	require.Empty(t, status.Error)
	var gotSensor, hold, enabled int
	var side string
	var limit float64
	require.NoError(t, dst.QueryRow(`SELECT sensor_id, side, limit_value, hold_minutes, enabled FROM alert_rule`).Scan(&gotSensor, &side, &limit, &hold, &enabled))
	assert.Equal(t, sensor, gotSensor)
	assert.Equal(t, "low", side)
	assert.Equal(t, 12.5, limit)
	assert.Equal(t, 20, hold)
	assert.Zero(t, enabled)
}

func TestBackupHTTP_RestoreClimateTargets(t *testing.T) {
	src := testutil.NewTestDB(t)
	seedSampleData(t, src)
	require.NoError(t, handlers.EnsureClimateTargets(src))
	_, err := src.Exec(`UPDATE climate_target SET target = 1.05 WHERE measure = 'vpd' AND lights = 'on'
		AND phase = '' AND status_id = (SELECT id FROM plant_status WHERE status = 'Veg')`)
	require.NoError(t, err)
	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)
	require.NotEmpty(t, payload.ClimateTargets)

	vegTarget := func(db *sql.DB) float64 {
		var v float64
		require.NoError(t, db.QueryRow(`SELECT target FROM climate_target WHERE measure = 'vpd' AND lights = 'on'
			AND phase = '' AND status_id = (SELECT id FROM plant_status WHERE status = 'Veg')`).Scan(&v))
		return v
	}
	dst := testutil.NewTestDB(t)
	status := restoreViaHTTP(t, dst, payload)
	require.Empty(t, status.Error)
	assert.Equal(t, 1.05, vegTarget(dst))

	// An older backup: no climate targets, its own Veg VPD range.
	payload.ClimateTargets = nil
	for _, row := range payload.PlantStatuses {
		if row["status"] == "Veg" {
			row["vpd_low"], row["vpd_high"] = 0.9, 1.3
		}
	}
	old := testutil.NewTestDB(t)
	status = restoreViaHTTP(t, old, payload)
	require.Empty(t, status.Error)
	assert.InDelta(t, 1.1, vegTarget(old), 1e-9)
}
