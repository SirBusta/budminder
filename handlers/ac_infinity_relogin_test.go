package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/handlers"
	"isley/secretbox"
	"isley/tests/testutil"
)

// "Stay signed in": the password is stored encrypted (never readable in the
// database), and the automatic sign-in uses it to get and save a new token.
func TestStaySignedIn_SaveThenAutomaticSignIn(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	store := config.NewStore()
	dataDir := t.TempDir()
	aci := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("appEmail") == "grower@example.com" && r.PostForm.Get("appPasswordl") == "s3cret-pass" {
			_, _ = w.Write([]byte(`{"code":200,"msg":"success","data":{"appId":"FRESHTOKEN"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":10001,"msg":"Incorrect account or password","data":{}}`))
	}))
	t.Cleanup(aci.Close)

	relogin := handlers.NewACIRelogin(db, store, dataDir, aci.URL)
	attempted, err := relogin(context.Background())
	assert.False(t, attempted, "nothing saved yet: no sign-in attempted")
	require.NoError(t, err)

	require.NoError(t, handlers.SaveACIPassword(db, store, dataDir, "grower@example.com", "s3cret-pass"))
	saved, err := handlers.GetSetting(db, "aci.password")
	require.NoError(t, err)
	assert.NotEmpty(t, saved)
	assert.NotContains(t, saved, "s3cret", "the database holds only scrambled text")
	_, err = os.Stat(filepath.Join(dataDir, secretbox.KeyFile))
	require.NoError(t, err, "the key is in its own file in the data folder")

	attempted, err = relogin(context.Background())
	assert.True(t, attempted)
	require.NoError(t, err)
	token, err := handlers.GetSetting(db, "aci.token")
	require.NoError(t, err)
	assert.Equal(t, "FRESHTOKEN", token)
	assert.Equal(t, "FRESHTOKEN", store.ACIToken(), "the store has the new token for the next poll")

	// A backup restored elsewhere has the scrambled password but not the key.
	attempted, err = handlers.NewACIRelogin(db, store, t.TempDir(), aci.URL)(context.Background())
	assert.True(t, attempted)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "can't be read on this computer"), err.Error())

	// A changed AC Infinity password: AC Infinity says why.
	require.NoError(t, handlers.SaveACIPassword(db, store, dataDir, "grower@example.com", "old-pass"))
	_, err = relogin(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Incorrect account or password")

	handlers.ForgetACIPassword(db, store)
	attempted, _ = relogin(context.Background())
	assert.False(t, attempted, "forgotten: no more automatic sign-ins")
}
