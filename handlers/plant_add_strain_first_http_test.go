package handlers_test

// Add Plant / Edit plant send people to the Strains page for a new strain
// (web/static/js/strain-first.js) instead of a half-filled "Add New Strain".

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"isley/config"
	"isley/handlers"
	"isley/tests/testutil"
)

// strainFirstEnv: the pages list strains from the settings store, which
// main fills at startup (handlers.LoadSettings); here it's filled after seeding.
func strainFirstEnv(t *testing.T, seed func(db *sql.DB)) *sensorEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	seed(db)
	store := config.NewStore()
	store.SetStrains(handlers.GetStrains(db))
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(store))
	testutil.SeedAdmin(t, db, sensorManagePassword)
	c, token := server.LoginAndFetchCSRF(t, sensorManagePassword, "/settings")
	return &sensorEnv{db: db, server: server, client: c, token: token}
}

func TestPlantAddPage_NoStrainsYet(t *testing.T) {
	t.Parallel()
	e := strainFirstEnv(t, func(db *sql.DB) {
		_, err := db.Exec(`DELETE FROM strain`)
		assert.NoError(t, err)
	})

	body := e.page(t, "/plant/new")
	assert.Contains(t, body, `id="strainFirstNote"`)
	assert.Contains(t, body, "alert-warning", "no strains at all: the note is a warning")
	assert.Contains(t, body, `href="/strains"`)
}

func TestPlantAddPage_StrainFirst(t *testing.T) {
	t.Parallel()
	var strain int
	e := strainFirstEnv(t, func(db *sql.DB) {
		strain = testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "SF Breeder"), "SF Kush")
	})

	body := e.page(t, "/plant/new")
	assert.Contains(t, body, `id="strainFirstNote"`)
	assert.Contains(t, body, "alert-info")
	assert.Contains(t, body, `data-strain-first`, "the Strains link saves the form first")
	assert.Contains(t, body, fmt.Sprintf(`value="%d" data-name="SF Kush"`, strain), "options carry the bare name")
	assert.NotContains(t, body, `id="newStrainCard"`, "no half-filled Add New Strain form")
	assert.NotContains(t, body, `id="newStrainName"`)
	assert.Contains(t, body, "strain-first.js")
}

func TestPlantEditPage_StrainFirst(t *testing.T) {
	t.Parallel()
	var plant int
	e := strainFirstEnv(t, func(db *sql.DB) {
		strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "SF Breeder"), "SF Haze")
		plant = testutil.SeedPlant(t, db, "SF Plant", strain, testutil.SeedZone(t, db, "SF Tent"))
	})

	body := e.page(t, fmt.Sprintf("/plant/%d/edit", plant))
	assert.Contains(t, body, `data-name="SF Haze"`)
	assert.Contains(t, body, `id="strainFirstBack"`)
	assert.NotContains(t, body, `id="newStrainCard"`)
	assert.NotContains(t, body, `id="breederSelect"`)
}
