package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// Wrong API keys are counted per address; past the limit they're refused
// before the slow bcrypt check (M01-03).
func TestAPIKey_WrongKeysAreRateLimited(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAPIKey(t, db, "the-right-key")
	c := server.NewClient(t)

	get := func(key string) int {
		resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/api/overlay", key, nil, ""))
		require.NoError(t, err)
		testutil.DrainAndClose(resp)
		return resp.StatusCode
	}
	require.Equal(t, http.StatusOK, get("the-right-key"))
	for i := range handlers.MaxBadAPIKeysPerMinute {
		assert.Equalf(t, http.StatusUnauthorized, get(fmt.Sprintf("guess-%d", i)), "guess %d", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, get("guess-next"))
}
