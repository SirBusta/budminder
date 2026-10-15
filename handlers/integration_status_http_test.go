package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/handlers"
	"isley/tests/testutil"
)

// When AC Infinity refuses the saved sign-in, Today, Manage sensors and
// Settings all say so, instead of showing "Connected".
func TestIntegrationStatus_RefusedSignInShowsEverywhere(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	require.NoError(t, handlers.UpdateSetting(db, nil, "aci.enabled", "1"))
	require.NoError(t, handlers.UpdateSetting(db, nil, "aci.token", "expired-token"))
	store := config.NewStore()
	store.SetACIEnabled(1)
	store.SetACIToken("expired-token")
	store.ReportIntegration("acinfinity", "auth", "100001 Token invalid", time.Now())

	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(store))
	testutil.SeedAdmin(t, db, sensorManagePassword)
	c, _ := server.LoginAndFetchCSRF(t, sensorManagePassword, "/settings")

	get := func(path string) string {
		resp := c.Get(path)
		defer testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(b)
	}

	var home struct {
		Integrations map[string]handlers.IntegrationStatus `json:"integrations"`
	}
	require.NoError(t, json.Unmarshal([]byte(get("/home/data")), &home))
	assert.Equal(t, "auth", home.Integrations["acinfinity"].State)
	assert.Equal(t, "100001 Token invalid", home.Integrations["acinfinity"].Detail)
	assert.Equal(t, "off", home.Integrations["ecowitt"].State)

	assert.Contains(t, get("/settings"), "Sign-in expired", "the AC Infinity badge")
	assert.Contains(t, get("/sensors/manage"), `&#34;acinfinity&#34;:{&#34;state&#34;:&#34;auth&#34;`, "Manage sensors page data (quotes escaped in the attribute)")
}
