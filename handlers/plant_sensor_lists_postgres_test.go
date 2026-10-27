//go:build integration_postgres

// Postgres-only breakage the SQLite suite can't see: integer columns
// COALESCEd with a boolean (plant page, Plants list) and the sensors.show
// column migration 013 dropped on Postgres only (Manage sensors).
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./handlers/...

package handlers_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func TestPlantPagesAndLists_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	breederID := testutil.SeedBreeder(t, db, "PG Breeder")
	strainID := testutil.SeedStrain(t, db, breederID, "PG Strain")
	zoneID := testutil.SeedZone(t, db, "PG Tent")
	plantID := testutil.SeedPlant(t, db, "PG Plant", strainID, zoneID)
	_, err := db.Exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-01')`,
		plantID, plantStatusID(t, db, "Seedling"))
	require.NoError(t, err)

	p := handlers.GetPlant(db, strconv.Itoa(plantID))
	assert.EqualValues(t, plantID, p.ID, "plant page loads")

	rows, err := handlers.LoadPlantsList(db)
	require.NoError(t, err)
	require.Len(t, rows, 1, "Plants list")

	living := handlers.GetLivingPlants(db)
	require.Len(t, living, 1, "living plants")
}

func TestManagedSensors_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	id := testutil.SeedSensor(t, db, "ACI", "dev", "temp")
	_, err := db.Exec(`UPDATE sensors SET show = $1 WHERE id = $2`, false, id)
	require.NoError(t, err, "sensors.show exists on Postgres")

	list := handlers.ListManagedSensors(db, 60)
	require.Len(t, list, 1, "Manage sensors lists the sensor")
	assert.False(t, list[0].ShowEnv)
}
