package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 038 makes Training and Defoliate standard activity types,
// skipping either one when an install already has its own.

func TestMigration038_FreshInstall(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	icon, color := activityIconColor(t, db, "Training")
	assert.Equal(t, "fa-hand", icon)
	assert.Equal(t, "#e67e22", color)
	icon, _ = activityIconColor(t, db, "Defoliate")
	assert.Equal(t, "fa-scissors", icon)

	var locked bool
	require.NoError(t, db.QueryRow(`SELECT lock FROM activity WHERE name = 'Training'`).Scan(&locked))
	assert.False(t, locked, "growers can rename or delete them")
}

func TestMigration038_KeepsTheGrowersOwn(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(37))

	_, err := db.Exec(`INSERT INTO activity (name) VALUES ('LST'), ('Defoliation')`)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(38))

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity WHERE name IN ('Training', 'Defoliate')`).Scan(&n))
	assert.Zero(t, n, "an existing training or defoliation type is not duplicated")
}

func TestMigration038_Down(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(38))

	// Defoliate was logged once, so it stays; the unused Training goes.
	_, err := db.Exec(`INSERT INTO breeder (name) VALUES ('B')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count) VALUES ('S', 1, 50, 50, 0, '', 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO plant (name, strain_id, description, clone, start_dt, sensors) VALUES ('P', 1, '', 0, '2026-01-01', '[]')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO plant_activity (plant_id, activity_id, date, note)
		SELECT 1, id, '2026-01-02', '' FROM activity WHERE name = 'Defoliate'`)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(37))

	var names []string
	rows, err := db.Query(`SELECT name FROM activity WHERE name IN ('Training', 'Defoliate')`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		names = append(names, s)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"Defoliate"}, names)
}
