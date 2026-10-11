package handlers_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/model/types"
	"isley/tests/testutil"
)

type notebookFixture struct {
	Plant1, Plant2                    int
	WaterID, FeedID, NoteID, HeightID int
	HeightMetric                      int
}

func seedNotebookFixture(t *testing.T, db *sql.DB) notebookFixture {
	t.Helper()
	var f notebookFixture
	breeder := testutil.SeedBreeder(t, db, "B")
	strain := testutil.SeedStrain(t, db, breeder, "S")
	zone := testutil.SeedZone(t, db, "Tent")
	f.Plant1 = testutil.SeedPlant(t, db, "Plant 1", strain, zone)
	f.Plant2 = testutil.SeedPlant(t, db, "Plant 2", strain, zone)
	for name, id := range map[string]*int{"Water": &f.WaterID, "Feed": &f.FeedID, "Note": &f.NoteID, "Height": &f.HeightID} {
		require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name = $1`, name).Scan(id))
	}
	require.NoError(t, db.QueryRow(`SELECT id FROM metric WHERE name = 'Height'`).Scan(&f.HeightMetric))
	return f
}

func findDay(days []types.NotebookDay, plantID int, date string) *types.NotebookDay {
	for i := range days {
		if days[i].PlantID == plantID && days[i].Date == date {
			return &days[i]
		}
	}
	return nil
}

func typeIDs(d *types.NotebookDay) []int {
	ids := []int{}
	for _, ty := range d.Types {
		ids = append(ids, ty.ActivityID)
	}
	return ids
}

func TestLoadPlantNotebook_OldDaysAreJoined(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	f := seedNotebookFixture(t, db)

	// Two separate old-style entries on one day, plus a repeat water.
	testutil.MustExec(t, db, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'Gave a gallon.', '2026-09-30T08:10:00')`, f.Plant1, f.WaterID)
	testutil.MustExec(t, db, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'Flipped to 12/12.', '2026-09-30T09:00:00')`, f.Plant1, f.NoteID)
	testutil.MustExec(t, db, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, '', '2026-09-30 18:30:00')`, f.Plant1, f.WaterID)

	days, err := handlers.LoadPlantNotebook(db, []int{f.Plant1})
	require.NoError(t, err)
	require.Len(t, days, 1)
	d := days[0]
	assert.Equal(t, "2026-09-30", d.Date)
	assert.False(t, d.HasEntry)
	assert.Equal(t, "Gave a gallon.\nFlipped to 12/12.", d.Note)
	assert.Equal(t, []int{f.WaterID, f.NoteID}, typeIDs(&d))
	assert.Equal(t, 2, d.Types[0].Count, "watered twice")
	assert.Equal(t, "fa-droplet", d.Types[0].Icon)
	assert.Len(t, d.Rows, 3)
	assert.Equal(t, "08:10:00", d.Rows[0].Time)
}

func TestLoadPlantNotebook_DayEntryNoteWins(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	f := seedNotebookFixture(t, db)

	testutil.MustExec(t, db, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, '', '2026-10-05T07:00:00')`, f.Plant1, f.WaterID)
	testutil.MustExec(t, db, `INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES ($1, '2026-10-05', 'Reservoir dry, gave 1 gallon.')`, f.Plant1)
	// A note-only day with no ticks.
	testutil.MustExec(t, db, `INSERT INTO plant_day_entry (plant_id, entry_date, note) VALUES ($1, '2026-10-06', 'Looking good.')`, f.Plant1)

	days, err := handlers.LoadPlantNotebook(db, []int{f.Plant1})
	require.NoError(t, err)
	require.Len(t, days, 2)
	assert.Equal(t, "2026-10-06", days[0].Date, "newest first")
	assert.Equal(t, "Looking good.", days[0].Note)
	assert.Empty(t, days[0].Types)
	d := findDay(days, f.Plant1, "2026-10-05")
	require.NotNil(t, d)
	assert.True(t, d.HasEntry)
	assert.Equal(t, "Reservoir dry, gave 1 gallon.", d.Note)
}

func TestLoadPlantNotebook_Measurements(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	f := seedNotebookFixture(t, db)

	// Linked to an activity row.
	rowID := testutil.SeedActivityAt(t, db, f.Plant1, f.HeightID, "2026-10-07T10:00:00")
	testutil.MustExec(t, db, `INSERT INTO plant_measurements (plant_id, metric_id, value, date, plant_activity_id) VALUES ($1, $2, 16, '2026-10-07T10:00:00', $3)`, f.Plant1, f.HeightMetric, rowID)
	// Standalone (old Add measurement button) on a day with nothing else.
	testutil.MustExec(t, db, `INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, 11, '2026-09-30 12:00:00')`, f.Plant1, f.HeightMetric)

	days, err := handlers.LoadPlantNotebook(db, []int{f.Plant1})
	require.NoError(t, err)

	linked := findDay(days, f.Plant1, "2026-10-07")
	require.NotNil(t, linked)
	require.Len(t, linked.Measurements, 1)
	m := linked.Measurements[0]
	assert.Equal(t, 16.0, m.Value)
	assert.Equal(t, "in", m.Unit, "measurements carry their unit")
	assert.Equal(t, f.HeightID, m.ActivityID)
	assert.False(t, m.Standalone)

	alone := findDay(days, f.Plant1, "2026-09-30")
	require.NotNil(t, alone)
	require.Len(t, alone.Measurements, 1)
	assert.True(t, alone.Measurements[0].Standalone)
	assert.Equal(t, f.HeightID, alone.Measurements[0].ActivityID, "a standalone Height counts as the Height type")
	assert.Equal(t, []int{f.HeightID}, typeIDs(alone))
	assert.Empty(t, alone.Rows)
}

func TestLoadPlantNotebook_PhotosAndPlants(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	f := seedNotebookFixture(t, db)

	testutil.MustExec(t, db, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'p1', '2026-09-25T08:00:00')`, f.Plant1, f.WaterID)
	testutil.MustExec(t, db, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'p2', '2026-09-25T08:00:00')`, f.Plant2, f.WaterID)
	// Photo on an entry's day and a photo-only day; backslash paths are normalized.
	testutil.MustExec(t, db, `INSERT INTO plant_images (plant_id, image_path, image_description, image_date) VALUES ($1, 'uploads\plants\a.jpg', NULL, '2026-09-25')`, f.Plant1)
	testutil.MustExec(t, db, `INSERT INTO plant_images (plant_id, image_path, image_description, image_date) VALUES ($1, 'uploads/plants/b.jpg', 'top view', '2026-09-26')`, f.Plant1)

	days, err := handlers.LoadPlantNotebook(db, []int{f.Plant1, f.Plant2})
	require.NoError(t, err)
	require.Len(t, days, 3)
	assert.Equal(t, "2026-09-26", days[0].Date)
	assert.Equal(t, "/uploads/plants/b.jpg", days[0].Photos[0].Path)
	assert.Equal(t, "top view", days[0].Photos[0].Description)
	assert.Empty(t, days[0].Types, "photo-only day")
	assert.Equal(t, f.Plant1, days[1].PlantID, "same day: lower plant id first")
	assert.Equal(t, f.Plant2, days[2].PlantID)
	assert.Equal(t, "/uploads/plants/a.jpg", days[1].Photos[0].Path)

	none, err := handlers.LoadPlantNotebook(db, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
