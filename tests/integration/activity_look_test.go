package integration

// Activity type icon and color (Settings → Activities; shown in the Plant
// page notebook): defaults, allowed values, keep-on-empty update, and the
// built-in Height name being reserved.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

func TestActivityLook_AddUpdateAndValidate(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := testutil.SeedAPIKey(t, db, "test-activity-look-key")
	c := server.NewClient(t)
	look := func(id int) (string, string) {
		var icon, color string
		require.NoError(t, db.QueryRow(`SELECT icon, color FROM activity WHERE id = $1`, id).Scan(&icon, &color))
		return icon, color
	}

	// No icon/color given: generic tag, slate.
	resp := c.APIPostJSON(t, "/activities", apiKey, map[string]interface{}{"activity_name": "Flush"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	plain := readID(t, resp)
	resp.Body.Close()
	icon, color := look(plain)
	assert.Equal(t, "fa-tag", icon)
	assert.Equal(t, "#64748b", color)

	// Chosen icon/color are saved.
	resp = c.APIPostJSON(t, "/activities", apiKey, map[string]interface{}{"activity_name": "Training", "icon": "fa-hand", "color": "#e67e22"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	id := readID(t, resp)
	resp.Body.Close()
	icon, color = look(id)
	assert.Equal(t, "fa-hand", icon)
	assert.Equal(t, "#e67e22", color)

	// Update without icon/color keeps them; with new ones changes them.
	resp = apiPutJSON(t, c, "/activities/"+strconv.Itoa(id), apiKey, map[string]interface{}{"activity_name": "Training"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	icon, color = look(id)
	assert.Equal(t, "fa-hand", icon)
	assert.Equal(t, "#e67e22", color)
	resp = apiPutJSON(t, c, "/activities/"+strconv.Itoa(id), apiKey, map[string]interface{}{"activity_name": "Training", "icon": "fa-scissors", "color": "#9B59B6"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	icon, color = look(id)
	assert.Equal(t, "fa-scissors", icon)
	assert.Equal(t, "#9B59B6", color)

	// Rejected: unknown icon, bad color, the reserved Height name.
	for _, body := range []map[string]interface{}{
		{"activity_name": "X", "icon": "fa-not-an-icon"},
		{"activity_name": "X", "icon": "fa-tag\" onmouseover=\"alert(1)"},
		{"activity_name": "X", "color": "red"},
		{"activity_name": "Height"},
	} {
		resp := c.APIPostJSON(t, "/activities", apiKey, body)
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
	}
	resp = apiPutJSON(t, c, "/activities/"+strconv.Itoa(id), apiKey, map[string]interface{}{"activity_name": "Training", "color": "#12345"})
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
