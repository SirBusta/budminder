//go:build integration_postgres

// Seed counts are read, changed and written back in one transaction; on
// Postgres the read must lock the row or simultaneous changes are lost
// (M07-06).
//
//	go test -tags=integration_postgres -run Postgres ./handlers/...

package handlers_test

import (
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
