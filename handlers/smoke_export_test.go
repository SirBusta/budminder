package handlers_test

// Smoke report export: the page data the Export window draws from
// (handlers/smoke_export.go), read back from the report's page.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// exportData reads the srExportData JSON from a report's page ("" when absent).
func (e *sensorEnv) exportData(t *testing.T, id int) (handlers.SmokeExport, bool) {
	t.Helper()
	html := e.page(t, fmt.Sprintf("/smoke/%d", id))
	const open = `<script type="application/json" id="srExportData">`
	i := strings.Index(html, open)
	if i < 0 {
		return handlers.SmokeExport{}, false
	}
	rest := html[i+len(open):]
	var out handlers.SmokeExport
	require.NoError(t, json.Unmarshal([]byte(rest[:strings.Index(rest, "</script>")]), &out))
	return out, true
}

func segText(s []handlers.SmokeExportSeg) (all string, bold []string) {
	for _, x := range s {
		all += x.T
		if x.B {
			bold = append(bold, x.T)
		}
	}
	return all, bold
}

func rowsHave(rows [][2]string, label, value string) bool {
	for _, r := range rows {
		if r[0] == label && (value == "" || r[1] == value) {
			return true
		}
	}
	return false
}

func TestSmokeExport_HomeGrownClone(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	strain := e.seedStrainWithLineage(t)
	zone := testutil.SeedZone(t, e.db, "3x3 Tent")
	plant := e.seedGrapePlant(t, strain, zone, true)

	code, out := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{
		"plant_id": plant, "strain_id": strain, "strain_name": "Grape OG", "breeder_name": "The Cali Connection", "smoked_on": "2025-04-12",
		"grow_space": "3x3 Tent", "harvested_on": "2025-02-01", "days_to_harvest": 134, "dry_days": 11, "cure_days": 20,
		"thc": "19–24", "lights": "LED 480", "method": "joint", "again": "yes", "score_bud": 9, "score_taste": 8,
		"notes": "Grape up front.\n\nSmooth smoke.",
		"tags":  []map[string]any{{"kind": "effect", "name": "relaxed"}, {"kind": "side", "name": "dry_mouth"}, {"kind": "smell", "name": "Grape soda", "custom": true}},
	})
	require.Equal(t, http.StatusOK, code, out)
	id := int(out["id"].(float64))

	x, ok := e.exportData(t, id)
	require.True(t, ok, "signed in: the page carries the export data")
	assert.Equal(t, "Grape OG", x.Name)
	assert.Equal(t, "2025-04-12", x.Smoked)
	assert.Equal(t, "The Cali Connection · Grape Ape × OG Kush", x.Kicker, "breeder, then the parents")
	all, bold := segText(x.Sub)
	assert.Equal(t, "from Grape OG #1, home grown in the 3x3 Tent", all)
	assert.Equal(t, []string{"Grape OG #1", "3x3 Tent"}, bold, "the names are the bold parts")
	assert.Equal(t, "Would grow again", x.Again)
	assert.Equal(t, "yes", x.AgainK)
	assert.Equal(t, "8.5", x.Score)
	require.NotEmpty(t, x.Chips)
	assert.Equal(t, handlers.SmokeExportSeg{T: "Home grown", B: true}, x.Chips[0])
	assert.True(t, rowsHave(x.Glance, "THC", "19–24%"))
	assert.True(t, rowsHave(x.Glance, "Date harvested", "@date:2025-02-01"), "dates go to the browser to format")
	assert.True(t, rowsHave(x.Glance, "Clone to harvest", "134 days"), "a clone counts from the cutting")
	assert.False(t, rowsHave(x.Glance, "Germination to harvest", ""))
	require.NotNil(t, x.Box)
	assert.Equal(t, "Grow details", x.Box.Title)
	assert.True(t, rowsHave(x.Box.Rows, "Lights", "LED 480"))
	assert.True(t, rowsHave(x.Box.Rows, "How it was smoked", "Joint"))
	require.NotNil(t, x.BoxC)
	assert.True(t, rowsHave(x.BoxC.Rows, "Grow space", "3x3 Tent"))
	assert.True(t, rowsHave(x.BoxC.Rows, "Clone to harvest", "134 days"))
	assert.Equal(t, "Grape up front.\n\nSmooth smoke.", x.Notes)
	require.Len(t, x.Tags, 3)
	assert.Equal(t, []string{"effect", "side", "smell"}, []string{x.Tags[0].Key, x.Tags[1].Key, x.Tags[2].Key})
	assert.Equal(t, []string{"Grape soda"}, x.Tags[2].Words, "the user's own words as typed")
	assert.Equal(t, "Grape OG smoke report 2025-04-12", x.File)
	assert.Equal(t, "Export smoke report", x.T["sr_x_title"])

	// Plant photos: one per day, none after the day smoked, five spread over the grow.
	require.NotNil(t, x.Strip)
	assert.Equal(t, "Grape OG #1 over time", x.Strip.Title)
	require.Len(t, x.Strip.Shots, 5)
	assert.Equal(t, "Day 1", x.Strip.Shots[0].Day, "days count from 1")
	assert.Equal(t, "Seedling", x.Strip.Shots[0].Stage)
	assert.Equal(t, "Flower", x.Strip.Shots[4].Stage)
	assert.True(t, strings.HasPrefix(x.Strip.Shots[0].URL, "/thumbs/plants/"), x.Strip.Shots[0].URL)

	// The report's page says "Clone to harvest" too.
	assert.Contains(t, e.page(t, fmt.Sprintf("/smoke/%d", id)), "Clone to harvest")

	// Hidden fields stay out of the export as they do on the page.
	code, _ = e.postSmoke(t, http.MethodPost, "/smoke/settings", map[string]any{
		"weights": map[string]int{"bud": 1, "taste": 1, "smell": 1, "effects": 1}, "hidden": []string{"lights", "thc_cbd", "side", "grow_space"}})
	require.Equal(t, http.StatusOK, code)
	x, _ = e.exportData(t, id)
	assert.False(t, rowsHave(x.Box.Rows, "Lights", ""))
	assert.False(t, rowsHave(x.Glance, "THC", ""))
	assert.False(t, rowsHave(x.BoxC.Rows, "Grow space", ""))
	all, _ = segText(x.Sub)
	assert.Equal(t, "from Grape OG #1, home grown", all)
	for _, g := range x.Tags {
		assert.NotEqual(t, "side", g.Key)
	}
	for _, c := range x.Chips {
		assert.NotContains(t, c.T, "THC")
	}
}

func TestSmokeExport_Bought(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	code, out := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{
		"source": "bought", "strain_name": "Mexican Wedding", "smoked_on": "2026-10-13", "dispensary": "Green Leaf",
		"price": 40, "amount_g": 3.5, "thc": "24", "method": "joint", "again": "no", "score_bud": 9,
	})
	require.Equal(t, http.StatusOK, code, out)
	id := int(out["id"].(float64))

	x, ok := e.exportData(t, id)
	require.True(t, ok)
	all, bold := segText(x.Sub)
	assert.Equal(t, "bought at Green Leaf", all)
	assert.Equal(t, []string{"Green Leaf"}, bold)
	assert.Equal(t, "Wouldn't buy again", x.Again)
	assert.Equal(t, []string{"THC 24%"}, x.Seal, "no strain type: THC goes beside the score")
	assert.True(t, rowsHave(x.Glance, "Price", "$40.00 · 3.5 g"))
	assert.True(t, rowsHave(x.Glance, "Price per gram", "$11.43/g"))
	assert.Nil(t, x.Box, "no Grow details for a bought report")
	require.NotNil(t, x.BoxC)
	assert.Equal(t, "Where it came from", x.BoxC.Title)
	assert.True(t, rowsHave(x.BoxC.Rows, "Dispensary", "Green Leaf"))
	assert.Nil(t, x.Strip)
	for _, c := range x.Chips {
		assert.False(t, c.B, "no Home grown chip")
	}
}

func TestSmokeExport_FileNameIsSafe(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	code, out := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{"strain_name": `Wedding "Cake": A/B?`, "smoked_on": "2026-01-02"})
	require.Equal(t, http.StatusOK, code, out)
	x, _ := e.exportData(t, int(out["id"].(float64)))
	assert.Equal(t, "Wedding  Cake  A B  smoke report 2026-01-02", x.File)
	all, _ := segText(x.Sub)
	assert.Equal(t, "Home grown", all, "no plant and no grow space")
}

func TestSmokeForm_PlantsSayWhichAreClones(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	strain := e.seedStrainWithLineage(t)
	zone := testutil.SeedZone(t, e.db, "Tent")
	clone := e.seedGrapePlant(t, strain, zone, true)
	form := e.page(t, "/smoke/new")
	assert.Contains(t, form, fmt.Sprintf(`&#34;id&#34;:%d,`, clone))
	assert.Contains(t, form, `&#34;clone&#34;:true`)
}

func (e *sensorEnv) seedStrainWithLineage(t *testing.T) int {
	t.Helper()
	strain := testutil.SeedStrain(t, e.db, testutil.SeedBreeder(t, e.db, "The Cali Connection"), "Grape OG")
	e.exec(t, `UPDATE strain SET indica = 60, sativa = 40 WHERE id = $1`, strain)
	e.exec(t, `INSERT INTO strain_lineage (strain_id, parent_name) VALUES ($1, 'Grape Ape'), ($1, 'OG Kush')`, strain)
	return strain
}

// seedGrapePlant: started 2024-09-20, Veg 2024-10-11, Flower 2024-12-01, Drying
// 2025-02-01, with seven photos (two on one day, one after the report's date).
func (e *sensorEnv) seedGrapePlant(t *testing.T, strain, zone int, clone bool) int {
	t.Helper()
	c := 0
	if clone {
		c = 1
	}
	var plant int
	require.NoError(t, e.db.QueryRow(`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		VALUES ('Grape OG #1', $1, $2, '', $3, '2024-09-20 09:00:00', '[]') RETURNING id`, zone, strain, c).Scan(&plant))
	for _, s := range [][2]string{{"Seedling", "2024-09-20"}, {"Veg", "2024-10-11"}, {"Flower", "2024-12-01"}, {"Drying", "2025-02-01"}} {
		e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)`, plant, statusIDByName(t, e.db, s[0]), s[1])
	}
	for i, d := range []string{"2024-09-20", "2024-09-20", "2024-10-20", "2024-11-15", "2024-12-10", "2025-01-15", "2025-06-01"} {
		e.exec(t, `INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date) VALUES ($1, $2, '', 0, $3)`,
			plant, fmt.Sprintf("uploads/plants/p%d.jpg", i), d)
	}
	return plant
}
