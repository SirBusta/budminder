package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 043: the climate_target table and the zone's light schedule,
// light check, followed plant and VPD source. A zone whose leaf offset
// switched on the calculated VPD keeps it as vpd_source 'calc'.

func TestMigration043_ClimateTargets(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)
	m := newSQLiteMigrator(t, db)
	require.NoError(t, m.Migrate(42))
	for _, q := range []string{
		`INSERT INTO zones (id, name, leaf_temp_offset) VALUES (1, 'Calc', -2)`,
		`INSERT INTO zones (id, name) VALUES (2, 'Plain')`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}

	require.NoError(t, m.Migrate(43))

	src := map[int]*string{}
	rows, err := db.Query(`SELECT id, vpd_source FROM zones`)
	require.NoError(t, err)
	for rows.Next() {
		var id int
		var s *string
		require.NoError(t, rows.Scan(&id, &s))
		src[id] = s
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.NotNil(t, src[1])
	assert.Equal(t, "calc", *src[1])
	assert.Nil(t, src[2])

	_, err = db.Exec(`UPDATE zones SET light_schedule = '{"veg":["06:00","00:00"]}', light_on_above = 0 WHERE id = 2`)
	require.NoError(t, err)
	ins := `INSERT INTO climate_target (status_id, phase, measure, lights, low, target, high)
		SELECT id, '', 'vpd', 'on', 0.8, 1.0, 1.2 FROM plant_status WHERE status = 'Veg'`
	_, err = db.Exec(ins)
	require.NoError(t, err)
	_, err = db.Exec(ins)
	assert.Error(t, err, "one row per stage, phase, measure and lights")

	require.NoError(t, m.Migrate(42))
	_, err = db.Exec(`SELECT COUNT(*) FROM climate_target`)
	assert.Error(t, err, "down removes the table")
	_, err = db.Exec(`SELECT light_schedule FROM zones`)
	assert.Error(t, err, "down removes the zone columns")
}
