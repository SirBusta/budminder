package model

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 034 adds an optional "Water amount" measurement to the watering
// and feeding activity types.

func waterAmountLinks(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query(`
		SELECT a.name, am.required FROM activity_metric am
		JOIN activity a ON a.id = am.activity_id
		JOIN metric m ON m.id = am.metric_id
		WHERE m.name = 'Water amount'`)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		var required bool
		require.NoError(t, rows.Scan(&name, &required))
		out[name] = required
	}
	require.NoError(t, rows.Err())
	return out
}

func waterAmountUnit(t *testing.T, db *sql.DB) string {
	t.Helper()
	var unit string
	require.NoError(t, db.QueryRow(`SELECT unit FROM metric WHERE name = 'Water amount'`).Scan(&unit))
	return unit
}

func TestMigration034_WaterAmount(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, newSQLiteMigrator(t, db).Migrate(34))

	assert.Equal(t, "gal", waterAmountUnit(t, db))
	assert.Equal(t, map[string]bool{"Water": false, "Feed": false}, waterAmountLinks(t, db), "optional on Water and Feed only")
}

func TestMigration034_MetricUnitsGiveLiters(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(33))
	_, err := db.Exec(`INSERT INTO settings (name, value) VALUES ('units', 'metric')`)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(34))
	assert.Equal(t, "L", waterAmountUnit(t, db))
}

func TestMigration034_DownKeepsRecordedAmounts(t *testing.T) {
	t.Parallel()
	initTestLogger()

	// Unused: down removes it, up again works.
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(34))
	require.NoError(t, m.Migrate(33))
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM metric WHERE name = 'Water amount'`).Scan(&n))
	assert.Zero(t, n)
	require.NoError(t, m.Migrate(34))

	// Used: down leaves the metric and its recorded values alone.
	db2 := freshSQLite(t)
	m2 := newSQLiteMigrator(t, db2)
	require.NoError(t, m2.Migrate(34))
	for _, q := range []string{
		`INSERT INTO breeder (name) VALUES ('B')`,
		`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count) VALUES ('S', 1, 50, 50, 0, '', 0)`,
		`INSERT INTO plant (name, strain_id, description, clone, start_dt, sensors) VALUES ('P', 1, '', 0, '2026-01-01', '[]')`,
		`INSERT INTO plant_measurements (plant_id, metric_id, value, date) SELECT 1, id, 1.5, '2026-01-02' FROM metric WHERE name = 'Water amount'`,
	} {
		_, err := db2.Exec(q)
		require.NoError(t, err)
	}
	require.NoError(t, m2.Migrate(33))
	require.NoError(t, db2.QueryRow(`SELECT COUNT(*) FROM metric WHERE name = 'Water amount'`).Scan(&n))
	assert.Equal(t, 1, n, "recorded amounts keep their measurement")
}
