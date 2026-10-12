package handlers_test

// The single-strain page (handlers/strain_page.go, views/strain.html): your
// grows of a strain, the page in both layouts, and Seed Vault "−".

import (
	"database/sql"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/model/types"
	"isley/tests/testutil"
)

// stagedPlant adds a plant of a strain that started on start and went
// through the given stages ([status, date] pairs).
func stagedPlant(t *testing.T, e *sensorEnv, name string, strainID, zoneID int, start string, stages ...[2]string) int {
	t.Helper()
	id := testutil.SeedPlant(t, e.db, name, strainID, zoneID)
	e.exec(t, `UPDATE plant SET start_dt = $1 WHERE id = $2`, start, id)
	for _, st := range stages {
		e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)`, id, statusIDByName(t, e.db, st[0]), st[1])
	}
	return id
}

func TestLoadStrainHistory(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	zone := testutil.SeedZone(t, e.db, "History Tent")
	breeder := testutil.SeedBreeder(t, e.db, "History Breeder")
	strain := testutil.SeedStrain(t, e.db, breeder, "History Strain")
	e.exec(t, `UPDATE strain SET cycle_time = 63, cycle_time_min = 56 WHERE id = $1`, strain)
	other := testutil.SeedStrain(t, e.db, breeder, "Other Strain")

	living := stagedPlant(t, e, "Living #1", strain, zone, "2026-07-28",
		[2]string{"Veg", "2026-08-19"}, [2]string{"Flower", "2026-09-30"})
	early := stagedPlant(t, e, "Harvest #1", strain, zone, "2026-01-10",
		[2]string{"Flower", "2026-03-08"}, [2]string{"Drying", "2026-05-09"}, [2]string{"Success", "2026-06-15"})
	e.exec(t, `UPDATE plant SET harvest_weight = 118 WHERE id = $1`, early)
	stagedPlant(t, e, "Harvest #2", strain, zone, "2026-01-02",
		[2]string{"Flower", "2026-03-01"}, [2]string{"Curing", "2026-05-03"})
	stagedPlant(t, e, "Dead #1", strain, zone, "2026-10-02",
		[2]string{"Seedling", "2026-10-02"}, [2]string{"Dead", "2026-10-07"})
	otherPlant := stagedPlant(t, e, "Other #1", other, zone, "2026-09-01", [2]string{"Veg", "2026-09-01"})

	e.exec(t, `INSERT INTO plant_images (plant_id, image_path, image_date) VALUES ($1, 'uploads/plants/a.jpg', '2026-09-25')`, living)
	e.exec(t, `INSERT INTO plant_images (plant_id, image_path, image_date) VALUES ($1, 'uploads/plants/b.jpg', '2026-10-09T08:00:00')`, living)
	e.exec(t, `INSERT INTO plant_images (plant_id, image_path, image_date) VALUES ($1, 'uploads/plants/c.jpg', '2026-10-10')`, otherPlant)

	h, err := handlers.LoadStrainHistory(e.db, strain, "2026-10-11", nil)
	require.NoError(t, err)

	names := []string{}
	for _, g := range h.Grows {
		names = append(names, g.Name)
	}
	assert.Equal(t, []string{"Living #1", "Harvest #1", "Harvest #2", "Dead #1"}, names, "growing first, then the latest harvests, then the dead; other strains left out")
	assert.Equal(t, 1, h.Growing)
	assert.Equal(t, 2, h.Harvested)
	assert.Equal(t, 1, h.Dead)

	g := h.Grows[0]
	assert.Equal(t, 76, g.Day, "Jul 28 to Oct 11 is day 76")
	assert.Equal(t, 2, g.StageWeek, "12 days into flower is week 2")
	assert.Equal(t, "2026-11-25", g.EstMin)
	assert.Equal(t, "2026-12-02", g.EstMax)
	assert.Equal(t, 59, g.Progress, "75 of 127 days to the estimated harvest")

	assert.Equal(t, 62, h.Grows[1].FlowerDays, "Mar 8 to May 9")
	assert.Equal(t, "2026-05-09", h.Grows[1].End, "harvested on the first drying day")
	assert.Equal(t, 63, h.Grows[2].FlowerDays, "Mar 1 to May 3 (curing counts as harvested)")
	assert.Equal(t, "2026-10-07", h.Grows[3].End, "the day it died")
	assert.Equal(t, 118, h.AvgGrams, "only recorded weights count")
	assert.Equal(t, 63, h.AvgFlowerDays)
	assert.InDelta(t, 8.9, h.AvgFlowerWeeks, 0.001)

	require.Len(t, h.Photos, 2, "only this strain's photos")
	assert.Equal(t, "/uploads/plants/b.jpg", h.Photos[0].Path, "newest first")
	assert.Equal(t, "2026-10-09", h.Photos[0].Date)
	assert.Equal(t, "Living #1", h.Photos[0].Plant)
	assert.Equal(t, 2, h.PhotoCount)
}

func TestStrainPage_Layouts(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	zone := testutil.SeedZone(t, e.db, "Page Tent")
	breeder := testutil.SeedBreeder(t, e.db, "Page Breeder")
	grown := testutil.SeedStrain(t, e.db, breeder, "Grown Strain")
	e.exec(t, `UPDATE strain SET seed_count = 3, url = 'www.example.com/grown' WHERE id = $1`, grown)
	fresh := testutil.SeedStrain(t, e.db, breeder, "Fresh Strain")
	e.exec(t, `UPDATE strain SET seed_count = 0 WHERE id = $1`, fresh)
	stagedPlant(t, e, "Grown Strain #1", grown, zone, "2026-09-01", [2]string{"Veg", "2026-09-01"})

	body := e.page(t, "/strain/"+strconv.Itoa(grown))
	assert.Contains(t, body, `class="sd-grow"`, "the grows card")
	assert.Contains(t, body, "Grown Strain #1")
	assert.NotContains(t, body, "sd-nogrow")
	assert.Contains(t, body, `/strains?breeder=Page%20Breeder#all`, "breeder name opens your strains from that breeder")
	assert.Contains(t, body, "2 of your strains")
	assert.Contains(t, body, `href="https://www.example.com/grown"`, "the saved link, made a web address")
	assert.Contains(t, body, ">example.com ", "shown by its site")
	assert.Contains(t, body, "use_seed=1", "Start a grow uses a seed when there are some")
	assert.Contains(t, body, `name=Grown&#43;Strain&#43;%232`, "named after the next plant number")
	assert.Contains(t, body, `data-seeds="use"`, "Seed Vault −")

	body = e.page(t, "/strain/"+strconv.Itoa(fresh))
	assert.Contains(t, body, "sd-nogrow", "never grown: the banner")
	assert.Contains(t, body, `class="sd-cols even`, "and the balanced layout")
	assert.NotContains(t, body, `class="sd-grow"`)
	assert.Contains(t, body, `id="sdWanted"`, "out of stock: the wish-list button")
}

func TestUseStrainSeeds(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	strain := testutil.SeedStrain(t, e.db, testutil.SeedBreeder(t, e.db, "Seeds Breeder"), "Seeds Strain")
	e.exec(t, `UPDATE strain SET seed_count = 2, seeds_added_on = '2025-01-01' WHERE id = $1`, strain)
	url := "/strains/" + strconv.Itoa(strain) + "/seeds/use"

	use := func(count int) int {
		resp := e.client.SessionPostJSON(t, url, e.token, map[string]any{"count": count})
		testutil.DrainAndClose(resp)
		return resp.StatusCode
	}
	stock := func() (int, sql.NullString) {
		var n int
		var added sql.NullString
		require.NoError(t, e.db.QueryRow(`SELECT seed_count, seeds_added_on FROM strain WHERE id = $1`, strain).Scan(&n, &added))
		return n, added
	}

	require.Equal(t, http.StatusOK, use(1))
	n, added := stock()
	assert.Equal(t, 1, n)
	assert.Equal(t, "2025-01-01", added.String, "stock keeps its date")

	require.Equal(t, http.StatusOK, use(5))
	n, added = stock()
	assert.Equal(t, 0, n, "never below zero")
	assert.False(t, added.Valid && added.String != "", "no seeds, no stock date")

	assert.Equal(t, http.StatusBadRequest, use(0))
	resp := e.client.SessionPostJSON(t, "/strains/99999/seeds/use", e.token, map[string]any{"count": 1})
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestStrainLinks(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "seedjunky.com", handlers.StrainLinkHost("https://www.seedjunky.com/pm"))
	assert.Equal(t, "example.com", handlers.StrainLinkHost("example.com/a"))
	assert.Equal(t, "", handlers.StrainLinkHost("  "))
	assert.Equal(t, "https://example.com/a", handlers.SafeStrainLink("example.com/a"))
	assert.Equal(t, "", handlers.SafeStrainLink("ftp://example.com/a"), "web addresses only")
	assert.Equal(t, "", handlers.SafeStrainLink(""))
}

func TestStrainPotency(t *testing.T) {
	t.Parallel()
	f := func(v float64) *float64 { return &v }
	rows := handlers.StrainPotency(types.Strain{ThcMin: f(26), ThcMax: f(34), CbdMin: f(0), CbdMax: f(1), CbgMax: f(0.5)})
	require.Len(t, rows, 3)
	assert.Equal(t, "THC", rows[0].Label)
	assert.Equal(t, "26–34%", rows[0].Text)
	assert.InDelta(t, 26.0/35*100, rows[0].Left, 0.01)
	assert.Equal(t, "0–1%", rows[1].Text)
	assert.Equal(t, "CBG", rows[2].Label)
	assert.Equal(t, "0.5%", rows[2].Text)
	assert.Empty(t, handlers.StrainPotency(types.Strain{}))
}
