package handlers_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// A restore that fails during the sensor_data bulk load must still put
// back the two sensor_data indexes it dropped for the load (M06-05).
func TestBackupHTTP_FailedRestoreKeepsSensorIndexes(t *testing.T) {
	t.Parallel()

	src := testutil.NewTestDB(t)
	seedSampleData(t, src)
	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)
	require.NotEmpty(t, payload.SensorData)
	// The same reading twice: the bulk insert hits the primary key.
	payload.SensorData = append(payload.SensorData, payload.SensorData[0])

	js, err := json.Marshal(payload)
	require.NoError(t, err)
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("backup.json")
	require.NoError(t, err)
	_, err = w.Write(js)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))
	const apiKey = "restore-fail-key"
	testutil.SeedAPIKey(t, db, apiKey)

	body, ct := testutil.MultipartBody(t, "backup", "dup.zip", zipBuf.Bytes())
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
	require.NotEmpty(t, status.Error, "the duplicate reading should fail the restore")

	for _, idx := range []string{"idx_sensor_data_sensor_id", "idx_sensor_data_create_dt"} {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = $1`, idx).Scan(&n))
		assert.Equalf(t, 1, n, "index %s should be back after the failed restore", idx)
	}
}
