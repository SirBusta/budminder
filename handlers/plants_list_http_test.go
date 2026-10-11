package handlers_test

// The Plants page (mockup PL3): handlers.LoadPlantsList and the page.

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func activityIDByName(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name = $1`, name).Scan(&id))
	return id
}

func TestLoadPlantsList(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	db := e.db
	zone := testutil.SeedZone(t, db, "List Tent")
	breeder := testutil.SeedBreeder(t, db, "List Breeder")

	photoStrain := testutil.SeedStrain(t, db, breeder, "Photo Strain")
	e.exec(t, `UPDATE strain SET cycle_time = 63, cycle_time_min = 56, autoflower = false WHERE id = $1`, photoStrain)
	autoStrain := testutil.SeedStrain(t, db, breeder, "Auto Strain")
	e.exec(t, `UPDATE strain SET cycle_time = 77, cycle_time_min = 0, autoflower = true WHERE id = $1`, autoStrain)

	flowering := testutil.SeedPlant(t, db, "Flowering", photoStrain, zone)
	e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-08-01')`, flowering, statusIDByName(t, db, "Veg"))
	e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-09-01')`, flowering, statusIDByName(t, db, "Flower"))
	water, feed := activityIDByName(t, db, "Water"), activityIDByName(t, db, "Feed")
	// Stored dates mix "T" and space formats; the latest must win anyway.
	testutil.SeedActivityAt(t, db, flowering, water, "2026-10-07 18:30:00")
	testutil.SeedActivityAt(t, db, flowering, water, "2026-10-05T08:10:00")
	testutil.SeedActivityAt(t, db, flowering, feed, "2026-10-06T09:00:00")
	e.exec(t, `INSERT INTO plant_images (plant_id, image_path, image_date) VALUES ($1, 'uploads/plants/a.jpg', '2026-10-01')`, flowering)

	auto := testutil.SeedPlant(t, db, "Auto", autoStrain, zone)
	e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-02')`, auto, statusIDByName(t, db, "Seedling"))
	vegging := testutil.SeedPlant(t, db, "Vegging", photoStrain, zone)
	e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-02')`, vegging, statusIDByName(t, db, "Veg"))
	dead := testutil.SeedPlant(t, db, "Gone", photoStrain, zone)
	e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-05')`, dead, statusIDByName(t, db, "Dead"))
	done := testutil.SeedPlant(t, db, "Done", photoStrain, zone)
	e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-05-05')`, done, statusIDByName(t, db, "Success"))
	e.exec(t, `UPDATE plant SET harvest_weight = 87.5 WHERE id = $1`, done)

	list, err := handlers.LoadPlantsList(db)
	require.NoError(t, err)
	by := map[int]handlers.PlantListRow{}
	for _, r := range list {
		by[r.ID] = r
	}

	f := by[flowering]
	assert.Equal(t, "living", f.Group)
	assert.Equal(t, "Flower", f.Stage)
	assert.Equal(t, "2026-09-01", f.StageSince)
	assert.Equal(t, "List Breeder", f.Breeder)
	assert.Equal(t, "List Tent", f.Zone)
	assert.False(t, f.Autoflower)
	assert.Equal(t, "2026-10-07", f.LastWater, "latest watering across date formats")
	assert.Equal(t, "2026-10-06", f.LastFeed)
	assert.Equal(t, "2026-10-27", f.EstMin, "photoperiod: first day of flower + shortest flowering time")
	assert.Equal(t, "2026-11-03", f.EstMax)
	assert.Equal(t, "/uploads/plants/a.jpg", f.Photo)

	a := by[auto]
	assert.True(t, a.Autoflower)
	assert.Equal(t, "2026-03-19", a.EstMax, "autoflower: start date + seed-to-harvest")
	assert.Equal(t, a.EstMax, a.EstMin, "no minimum set: the window is one date")
	assert.Empty(t, a.LastWater, "never watered")

	assert.Empty(t, by[vegging].EstMax, "no estimate for a photoperiod before flower")
	assert.Equal(t, "dead", by[dead].Group)
	assert.Equal(t, "harvested", by[done].Group)
	assert.Equal(t, 87.5, by[done].HarvestWeight)
}

func TestPlantsPage_Renders(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	livingPlant(t, e, "Shown Plant", testutil.SeedZone(t, e.db, "Z"), "Veg")

	body := e.page(t, "/plants")
	assert.Contains(t, body, `id="plantsPage"`)
	assert.Contains(t, body, "plants-page.js")
	assert.Contains(t, body, "Shown Plant", "the plants are in the page data")
	assert.Contains(t, body, `href="/plant/new"`, "Add plant for a logged-in user")
	assert.NotContains(t, body, "quickActivityModal", "the old quick windows are gone")
}
