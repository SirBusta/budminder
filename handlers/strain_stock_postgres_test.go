//go:build integration_postgres

// Seed counts are read, changed and written back in one transaction; on
// Postgres the read must lock the row or simultaneous changes are lost
// (M07-06).
//
//	go test -tags=integration_postgres -run Postgres ./handlers/...

package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

func TestAddSeedsConcurrently_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "pg-seeds-pw")
	breederID := testutil.SeedBreeder(t, db, "PG Seeds Breeder")
	strainID := testutil.SeedStrain(t, db, breederID, "PG Seeds Strain")
	_, err := db.Exec(`UPDATE strain SET seed_count = 0 WHERE id = $1`, strainID)
	require.NoError(t, err)

	c, token := server.LoginAndFetchCSRF(t, "pg-seeds-pw", "/strains")
	const n = 20
	var wg sync.WaitGroup
	codes := make(chan int, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := c.SessionPostJSON(t, fmt.Sprintf("/strains/%d/seeds", strainID), token, map[string]any{"count": 1})
			testutil.DrainAndClose(resp)
			codes <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		require.Equal(t, http.StatusOK, code)
	}

	var count int
	require.NoError(t, db.QueryRow(`SELECT seed_count FROM strain WHERE id = $1`, strainID).Scan(&count))
	assert.Equal(t, n, count, "every added seed counted")
}

// Packs go through the strain list on Postgres too.
func TestStrainPack_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "pg-pack-pw")
	breederID := testutil.SeedBreeder(t, db, "PG Pack Breeder")
	strainID := testutil.SeedStrain(t, db, breederID, "PG Pack Strain")
	_, err := db.Exec(`UPDATE strain SET seed_count = 0 WHERE id = $1`, strainID)
	require.NoError(t, err)

	c, token := server.LoginAndFetchCSRF(t, "pg-pack-pw", "/strains")
	for _, body := range []map[string]any{
		{"count": 10, "pack": map[string]any{"price_cents": 18000, "free": false}},
		{"count": 1},
		{"count": 5, "pack": map[string]any{"price_cents": 15000, "free": true, "added_on": "2026-10-02"}},
	} {
		resp := c.SessionPostJSON(t, fmt.Sprintf("/strains/%d/seeds", strainID), token, body)
		testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	list := c.Get("/strains/all")
	defer testutil.DrainAndClose(list)
	require.Equal(t, http.StatusOK, list.StatusCode)
	var strains []struct {
		Packs []struct {
			Seeds   int    `json:"seeds"`
			Price   *int   `json:"price_cents"`
			Free    bool   `json:"free"`
			AddedOn string `json:"added_on"`
		} `json:"packs"`
	}
	require.NoError(t, json.NewDecoder(list.Body).Decode(&strains))
	require.Len(t, strains, 1)
	require.Len(t, strains[0].Packs, 2)
	assert.Equal(t, 11, strains[0].Packs[0].Seeds)
	assert.Equal(t, 15000, *strains[0].Packs[1].Price)
	assert.True(t, strains[0].Packs[1].Free)
	assert.Equal(t, "2026-10-02", strains[0].Packs[1].AddedOn)
}
