package handlers_test

// Settings → Grow setup → Breeders: a breeder your strains use shows how many,
// and Edit Breeder only lets you rename it (DELETE /breeders/:id refuses).

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"isley/config"
	"isley/handlers"
	"isley/tests/testutil"
)

func TestSettingsPage_BreederStrainCounts(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	used := testutil.SeedBreeder(t, db, "Used Breeder")
	testutil.SeedStrain(t, db, used, "Strain One")
	testutil.SeedStrain(t, db, used, "Strain Two")
	unused := testutil.SeedBreeder(t, db, "Unused Breeder")

	store := config.NewStore()
	store.SetBreeders(handlers.GetBreeders(db))
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(store))
	testutil.SeedAdmin(t, db, sensorManagePassword)
	c, token := server.LoginAndFetchCSRF(t, sensorManagePassword, "/settings")
	e := &sensorEnv{db: db, server: server, client: c, token: token}

	assert.Equal(t, map[int]int{used: 2}, handlers.BreederStrainCounts(db))

	body := e.page(t, "/settings")
	assert.Contains(t, body, fmt.Sprintf(`&#34;id&#34;:%d,&#34;name&#34;:&#34;Used Breeder&#34;}' data-strains="2"`, used))
	assert.Contains(t, body, fmt.Sprintf(`&#34;id&#34;:%d,&#34;name&#34;:&#34;Unused Breeder&#34;}' data-strains="0"`, unused))
	assert.Contains(t, body, `id="breederInUse"`)
}
