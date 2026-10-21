package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 039: Feed stops asking for the water amount; Water keeps it.

func TestMigration039_FeedNoLongerAsksForWater(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(38))

	linked := func(activity string) int {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity_metric am
			JOIN activity a ON a.id = am.activity_id JOIN metric mt ON mt.id = am.metric_id
			WHERE a.name = $1 AND lower(mt.name) = 'water amount'`, activity).Scan(&n))
		return n
	}
	require.Equal(t, 1, linked("Feed"), "before: Feed asks for the water amount (034)")
	require.Equal(t, 1, linked("Water"))

	require.NoError(t, m.Migrate(39))
	assert.Zero(t, linked("Feed"), "Feed no longer asks")
	assert.Equal(t, 1, linked("Water"), "Water still does")

	require.NoError(t, m.Migrate(38))
	assert.Equal(t, 1, linked("Feed"), "down puts it back")
}
