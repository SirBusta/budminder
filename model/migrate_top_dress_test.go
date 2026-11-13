package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 050 makes Top Dress a standard activity type, skipping it when
// an install already has its own.

func TestMigration050_FreshInstall(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	icon, color := activityIconColor(t, db, "Top Dress")
	assert.Equal(t, "fa-trowel", icon)
	assert.Equal(t, "#ca8a04", color)

	var locked, watering, feeding bool
	require.NoError(t, db.QueryRow(`SELECT lock, is_watering, is_feeding FROM activity WHERE name = 'Top Dress'`).Scan(&locked, &watering, &feeding))
	assert.False(t, locked, "growers can rename or delete it")
	assert.False(t, watering)
	assert.False(t, feeding)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity_metric am JOIN activity a ON a.id = am.activity_id WHERE a.name = 'Top Dress'`).Scan(&n))
	assert.Zero(t, n, "no measurement: what was used goes in the note")
}

func TestMigration050_KeepsTheGrowersOwn(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(49))

	_, err := db.Exec(`INSERT INTO activity (name) VALUES ('Top dressing')`)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(50))

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity WHERE name = 'Top Dress'`).Scan(&n))
	assert.Zero(t, n, "an existing top dressing type is not duplicated")
}

func TestMigration050_Down(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(50))
	require.NoError(t, m.Migrate(49))

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity WHERE name = 'Top Dress'`).Scan(&n))
	assert.Zero(t, n, "an unused Top Dress is removed")
}
