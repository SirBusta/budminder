//go:build integration_postgres

// The real connection string against real PostgreSQL.
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./model/...

package model

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openWithPostgresDSN(t *testing.T, user, password, database string) *sql.DB {
	t.Helper()
	env := loadPGEnv()
	vals := map[string]string{
		"DB_HOST": env.Host, "DB_PORT": env.Port, "DB_USER": user, "DB_PASSWORD": password, "DB_NAME": database,
	}
	db, err := sql.Open("postgres", postgresDSN(func(k string) string { return vals[k] }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Readings take their time from CURRENT_TIMESTAMP and are read back as UTC, so
// a server set to a local time zone must not shift them.
func TestPostgresDSN_SessionRunsInUTC_Postgres(t *testing.T) {
	t.Parallel()
	db := freshPostgresDB(t, false)
	var name string
	require.NoError(t, db.QueryRow(`SELECT current_database()`).Scan(&name))
	_, err := db.Exec(`ALTER DATABASE ` + quoteIdent(name) + ` SET timezone TO 'America/New_York'`)
	require.NoError(t, err)

	env := loadPGEnv()
	conn := openWithPostgresDSN(t, env.User, env.Password, name)
	var zone string
	require.NoError(t, conn.QueryRow(`SHOW timezone`).Scan(&zone))
	assert.Equal(t, "UTC", zone)
	var stamp time.Time
	require.NoError(t, conn.QueryRow(`SELECT CAST(CURRENT_TIMESTAMP AS TIMESTAMP)`).Scan(&stamp))
	assert.WithinDuration(t, time.Now(), stamp, time.Minute)
}

func TestPostgresDSN_PasswordReachesServerAsTyped_Postgres(t *testing.T) {
	t.Parallel()
	db := freshPostgresDB(t, false)
	var name string
	require.NoError(t, db.QueryRow(`SELECT current_database()`).Scan(&name))

	role := name + "_role"
	password := `back\slash it's`
	literal := "'" + strings.ReplaceAll(password, "'", "''") + "'"
	_, err := db.Exec(`CREATE ROLE ` + quoteIdent(role) + ` LOGIN PASSWORD ` + literal)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec(`DROP ROLE IF EXISTS ` + quoteIdent(role)) })

	conn := openWithPostgresDSN(t, role, password, name)
	require.NoError(t, conn.Ping())
	require.NoError(t, conn.Close(), "a closed session before the role is dropped")
}
