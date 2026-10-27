package model

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A read-then-write transaction that starts while a background job is
// writing must wait for it, not fail with "database is locked" (M03-03).
func TestSQLiteOptions_ReadThenWriteWaitsForOtherWriter(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "lock.db")
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", path+"?"+SQLiteOptions)
		require.NoError(t, err)
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { db.Close() })
		return db
	}
	bg, app := open(), open()
	_, err := bg.Exec(`CREATE TABLE stock (id INTEGER PRIMARY KEY, n INTEGER); INSERT INTO stock VALUES (1, 10)`)
	require.NoError(t, err)

	// The "background job" holds the write lock for a moment.
	bgTx, err := bg.Begin()
	require.NoError(t, err)
	_, err = bgTx.Exec(`UPDATE stock SET n = n + 1 WHERE id = 1`)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		done <- bgTx.Commit()
	}()

	tx, err := app.Begin()
	require.NoError(t, err)
	var n int
	require.NoError(t, tx.QueryRow(`SELECT n FROM stock WHERE id = 1`).Scan(&n))
	_, err = tx.Exec(`UPDATE stock SET n = $1 WHERE id = 1`, n+5)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, <-done)

	require.NoError(t, app.QueryRow(`SELECT n FROM stock WHERE id = 1`).Scan(&n))
	require.Equal(t, 16, n, "both changes kept")
}
