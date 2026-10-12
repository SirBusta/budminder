package model

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 035 turns the StrainCompass and CannaDB lookups on where no
// choice was ever saved.

func lookupSetting(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var v string
	require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE name = $1`, name).Scan(&v))
	return v
}

func TestMigration035_LookupsOnForNewInstalls(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, newSQLiteMigrator(t, db).Migrate(35))
	assert.Equal(t, "1", lookupSetting(t, db, "straincompass.enabled"))
	assert.Equal(t, "1", lookupSetting(t, db, "cannadb.enabled"))
}

func TestMigration035_KeepsAChoiceMadeEarlier(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(34))
	_, err := db.Exec(`INSERT INTO settings (name, value) VALUES ('straincompass.enabled', '0')`)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(35))
	assert.Equal(t, "0", lookupSetting(t, db, "straincompass.enabled"), "turned off before: stays off")
	assert.Equal(t, "1", lookupSetting(t, db, "cannadb.enabled"), "never set: on")
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'straincompass.enabled'`).Scan(&n))
	assert.Equal(t, 1, n, "no duplicate row")
}
