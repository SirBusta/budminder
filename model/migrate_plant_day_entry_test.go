package model

import (
	"database/sql"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 032 adds plant_day_entry (one note per plant per day), an icon
// and color per activity type, and a Height activity type that records the
// Height metric.

func newSQLiteMigrator(t *testing.T, db *sql.DB) *migrate.Migrate {
	t.Helper()
	src, err := iofs.New(MigrationsFS, "migrations/sqlite")
	require.NoError(t, err)
	drv, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	require.NoError(t, err)
	return m
}

func activityIconColor(t *testing.T, db *sql.DB, name string) (string, string) {
	t.Helper()
	var icon, color string
	require.NoError(t, db.QueryRow(`SELECT icon, color FROM activity WHERE name = $1`, name).Scan(&icon, &color))
	return icon, color
}

func TestMigration032_FreshInstall(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	assert.Contains(t, userTablesT(t, db), "plant_day_entry")

	icon, color := activityIconColor(t, db, "Water")
	assert.Equal(t, "fa-droplet", icon)
	assert.Equal(t, "#2f80ed", color)
	icon, _ = activityIconColor(t, db, "Feed")
	assert.Equal(t, "fa-flask", icon)
	icon, _ = activityIconColor(t, db, "Note")
	assert.Equal(t, "fa-note-sticky", icon)

	// Height is an activity type linked (required) to the Height metric.
	var required bool
	require.NoError(t, db.QueryRow(`
		SELECT am.required FROM activity_metric am
		JOIN activity a ON a.id = am.activity_id
		JOIN metric m ON m.id = am.metric_id
		WHERE a.name = 'Height' AND m.name = 'Height'`).Scan(&required))
	assert.True(t, required)
	icon, _ = activityIconColor(t, db, "Height")
	assert.Equal(t, "fa-ruler", icon)

	// One note per plant per day.
	_, err := db.Exec(`INSERT INTO breeder (name) VALUES ('B')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count) VALUES ('S', 1, 50, 50, 0, '', 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO plant (name, strain_id, description, clone, start_dt, sensors) VALUES ('P', 1, '', 0, '2026-01-01', '[]')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES (1, '2026-01-02', 'a')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES (1, '2026-01-02', 'b')`)
	assert.Error(t, err, "a second entry for the same plant and day must be rejected")
}

func TestMigration032_ExistingActivities(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(31))

	// A user who already made their own "height" and "Training" types.
	_, err := db.Exec(`INSERT INTO activity (name) VALUES ('height'), ('Training'), ('Defoliate'), ('Flush')`)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(32))

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity WHERE lower(name) = 'height'`).Scan(&n))
	assert.Equal(t, 1, n, "an existing height type is reused, not duplicated")
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*) FROM activity_metric am JOIN activity a ON a.id = am.activity_id
		WHERE a.name = 'height'`).Scan(&n))
	assert.Equal(t, 1, n, "the existing height type gets linked to the Height metric")

	icon, _ := activityIconColor(t, db, "height")
	assert.Equal(t, "fa-ruler", icon)
	icon, _ = activityIconColor(t, db, "Training")
	assert.Equal(t, "fa-hand", icon)
	icon, _ = activityIconColor(t, db, "Defoliate")
	assert.Equal(t, "fa-scissors", icon)
	icon, color := activityIconColor(t, db, "Flush")
	assert.Equal(t, "fa-tag", icon, "unrecognized types get the generic tag")
	assert.Equal(t, "#64748b", color)
}

func TestMigration032_Down(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(32))
	require.NoError(t, m.Migrate(31))

	assert.NotContains(t, userTablesT(t, db), "plant_day_entry")
	_, err := db.Exec(`SELECT icon FROM activity`)
	assert.Error(t, err, "icon column is dropped")
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity WHERE name = 'Height'`).Scan(&n))
	assert.Equal(t, 1, n, "the Height type stays: it may have been logged against")

	require.NoError(t, m.Migrate(32), "migrating up again works")
}
