package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 041: hours the old rollup rewrote from their last few minutes are
// rebuilt from all their raw readings; hours with no raw readings left stay.

func TestMigration041_RebuildsHourlyFromRawReadings(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(40))

	for _, q := range []string{
		`INSERT INTO sensors (id, name, source, device, type) VALUES (1, 'A', 's', 'd', 'a')`,
		`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES (1, 10, '2026-10-01 14:00:00'), (1, 30, '2026-10-01 14:59:00')`,
		// what the old rollup left: only the hour's last reading
		`INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES (1, '2026-10-01 14:00:00', 30, 30, 30, 1)`,
		// raw readings pruned long ago: keep this row
		`INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES (1, '2025-01-01 08:00:00', 5, 7, 6, 60)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}

	require.NoError(t, m.Migrate(41))

	var minV, maxV, avgV float64
	var n int
	require.NoError(t, db.QueryRow(`SELECT min_val, max_val, avg_val, sample_count FROM sensor_data_hourly
		WHERE sensor_id = 1 AND bucket = '2026-10-01 14:00:00'`).Scan(&minV, &maxV, &avgV, &n))
	assert.Equal(t, 2, n)
	assert.InDelta(t, 10.0, minV, 0.0001)
	assert.InDelta(t, 30.0, maxV, 0.0001)
	assert.InDelta(t, 20.0, avgV, 0.0001)

	require.NoError(t, db.QueryRow(`SELECT sample_count FROM sensor_data_hourly
		WHERE sensor_id = 1 AND bucket = '2025-01-01 08:00:00'`).Scan(&n))
	assert.Equal(t, 60, n, "an hour with no raw readings left is kept")

	var rows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data_hourly`).Scan(&rows))
	assert.Equal(t, 2, rows, "no duplicate buckets")
}
