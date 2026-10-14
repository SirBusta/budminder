package handlers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// Without BUDMINDER_SESSION_SECRET, BudMinder makes a key once and reuses it,
// so logins survive a restart.
func TestLoadOrCreateSessionSecret_MadeOnceThenReused(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)

	first, err := handlers.LoadOrCreateSessionSecret(db)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(first), 32, "long enough for the cookie store")

	again, err := handlers.LoadOrCreateSessionSecret(db)
	require.NoError(t, err)
	assert.Equal(t, first, again, "the same key after a restart")

	other, err := handlers.LoadOrCreateSessionSecret(testutil.NewTestDB(t))
	require.NoError(t, err)
	assert.NotEqual(t, first, other, "each install gets its own key")
}
