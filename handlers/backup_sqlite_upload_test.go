package handlers_test

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/model"
	"isley/tests/testutil"
)

// openFileDB opens a migrated SQLite file the way a real install does (WAL,
// busy timeout), not the in-memory database most tests use.
func openFileDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, model.MigrateSQLiteHandle(db))
	return db
}

// TestBackupHTTP_UploadSQLiteDB_RestoresIntoOpenDatabase: "Upload SQLite
// database" used to close the database handle the whole app shares, so every
// request failed until a restart. The upload's data must replace the live
// data and the app must keep answering on the same handle.
func TestBackupHTTP_UploadSQLiteDB_RestoresIntoOpenDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	live := openFileDB(t, filepath.Join(dir, "live.db"))
	const apiKey = "sqlite-up-restore-key"
	testutil.SeedAPIKey(t, live, apiKey)
	testutil.SeedZone(t, live, "Old Tent")

	srcPath := filepath.Join(dir, "upload.db")
	src := openFileDB(t, srcPath)
	testutil.SeedAPIKey(t, src, apiKey) // the restored settings replace the live key
	testutil.SeedZone(t, src, "Restored Tent")
	require.NoError(t, src.Close()) // checkpoints the WAL into the file
	upload, err := os.ReadFile(srcPath)
	require.NoError(t, err)

	server := testutil.NewTestServer(t, live, testutil.WithDataDir(dir))
	c := server.NewClient(t)
	body, ct := testutil.MultipartBody(t, "database", "upload.db", upload)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	require.Eventually(t, func() bool { return !server.BackupService.RestoreSnapshot().InProgress },
		10*time.Second, 20*time.Millisecond)
	assert.Empty(t, server.BackupService.RestoreSnapshot().Error)

	var zones []string
	rows, err := live.Query(`SELECT name FROM zones ORDER BY name`)
	require.NoError(t, err, "the shared handle still works")
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		zones = append(zones, n)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	assert.Contains(t, zones, "Restored Tent")
	assert.NotContains(t, zones, "Old Tent")

	resp, err = c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/restore/status", apiKey, nil, ""))
	require.NoError(t, err)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "requests still work after the restore")
}
