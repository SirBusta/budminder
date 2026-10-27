package integration

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// Camera snapshots used to be served from the public /uploads folder under
// an easy-to-guess name. They now come from /streams/:id/snapshot: any
// camera when signed in, only visible cameras for guests.
func TestStreamSnapshot_WhoSeesWhat(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	streamDir := t.TempDir()
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode(), testutil.WithStreamDir(streamDir))
	testutil.SeedZone(t, db, "Z")
	testutil.SeedAdmin(t, db, "snapshot-pw")

	ids := map[bool]int64{}
	for _, visible := range []bool{true, false} {
		var id int64
		require.NoError(t, db.QueryRow(`INSERT INTO streams (name, url, zone_id, visible) VALUES ('Cam', 'http://cam.invalid/', 1, $1) RETURNING id`, visible).Scan(&id))
		ids[visible] = id
		require.NoError(t, os.WriteFile(filepath.Join(streamDir, fmt.Sprintf("stream_%d_latest.jpg", id)), []byte("jpeg-bytes"), 0o644))
	}

	get := func(c *testutil.Client, id int64) (int, string) {
		resp := c.Get(fmt.Sprintf("/streams/%d/snapshot", id))
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	guest := server.NewClient(t)
	code, body := get(guest, ids[true])
	assert.Equal(t, http.StatusOK, code, "a guest sees a visible camera")
	assert.Equal(t, "jpeg-bytes", body)
	code, _ = get(guest, ids[false])
	assert.Equal(t, http.StatusNotFound, code, "a guest doesn't see a hidden camera")
	code, _ = get(guest, 9999)
	assert.Equal(t, http.StatusNotFound, code)

	admin := server.LoginAsAdmin(t, "snapshot-pw")
	code, _ = get(admin, ids[false])
	assert.Equal(t, http.StatusOK, code, "signed in, every camera")
}

func TestStreamSnapshot_NeedsSignInWithoutGuestMode(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	streamDir := t.TempDir()
	server := testutil.NewTestServer(t, db, testutil.WithStreamDir(streamDir))
	testutil.SeedZone(t, db, "Z")
	testutil.SeedAdmin(t, db, "snapshot-pw-2")
	var id int64
	require.NoError(t, db.QueryRow(`INSERT INTO streams (name, url, zone_id, visible) VALUES ('Cam', 'http://cam.invalid/', 1, $1) RETURNING id`, true).Scan(&id))
	require.NoError(t, os.WriteFile(filepath.Join(streamDir, fmt.Sprintf("stream_%d_latest.jpg", id)), []byte("jpeg-bytes"), 0o644))

	resp := server.NewClient(t).Get(fmt.Sprintf("/streams/%d/snapshot", id))
	defer testutil.DrainAndClose(resp)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

// The public /uploads folder no longer serves anything under streams/,
// while plant photos and other uploads still load.
func TestUploads_StreamsFolderIsClosed(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	// /uploads serves ./uploads relative to the working directory.
	name := fmt.Sprintf("snapshot-test-%d.jpg", os.Getpid())
	for _, dir := range []string{"uploads/streams", "uploads/plants"} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		t.Cleanup(func() { _ = os.Remove(dir); _ = os.Remove("uploads") }) // only if left empty
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
		t.Cleanup(func() { _ = os.Remove(p) })
	}

	c := server.NewClient(t)
	for _, p := range []string{"/uploads/streams/" + name, "/uploads/plants/../streams/" + name, "/uploads//streams/" + name} {
		resp := c.Get(p)
		testutil.DrainAndClose(resp)
		assert.NotEqual(t, http.StatusOK, resp.StatusCode, p)
	}
	resp := c.Get("/uploads/plants/" + name)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "other uploads still load")
}

// The stream list a guest can read leaves out hidden streams and every
// stream's address (it can carry the camera's password) (M08-04).
func TestStreamList_GuestSeesVisibleWithoutAddresses(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())
	testutil.SeedZone(t, db, "Z")
	testutil.SeedAdmin(t, db, "stream-list-pw")
	_, err := db.Exec(`INSERT INTO streams (name, url, zone_id, visible) VALUES
		('Shown', 'rtsp://user:secret@cam1.invalid/', 1, $1), ('Hidden', 'rtsp://user:secret@cam2.invalid/', 1, $2)`, true, false)
	require.NoError(t, err)

	list := func(c *testutil.Client) string {
		resp := c.Get("/streams")
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}

	guest := list(server.NewClient(t))
	assert.Contains(t, guest, "Shown")
	assert.NotContains(t, guest, "Hidden")
	assert.NotContains(t, guest, "secret")

	admin := list(server.LoginAsAdmin(t, "stream-list-pw"))
	assert.Contains(t, admin, "Hidden")
	assert.Contains(t, admin, "cam2.invalid")
}
