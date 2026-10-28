package handlers_test

// Compress existing photos (handlers/photo_compress_service.go): the run,
// its backup folder, pausing and resuming, and the 7-day cleanup.

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
	"isley/utils"
)

type photoEnv struct {
	db      *sql.DB
	data    string
	uploads string
	plant   int
	svc     *handlers.PhotoCompressService
	now     time.Time
	mu      sync.Mutex
}

func newPhotoEnv(t *testing.T) *photoEnv {
	t.Helper()
	db := testutil.NewTestDBWithoutForeignKeys(t)
	e := &photoEnv{db: db, data: t.TempDir(), uploads: t.TempDir(), now: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	e.plant = testutil.SeedPlant(t, db, "P", testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"), testutil.SeedZone(t, db, "Z"))
	e.svc = e.reopen()
	return e
}

// reopen builds a new service on the same folders, like a restart.
func (e *photoEnv) reopen() *handlers.PhotoCompressService {
	s := handlers.NewPhotoCompressService(e.db, e.data, e.uploads)
	handlers.SetPhotoCompressClock(s, func() time.Time {
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.now
	})
	return s
}

func (e *photoEnv) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func (e *photoEnv) addPhoto(t *testing.T, name string, body []byte) (int, string) {
	t.Helper()
	p := filepath.Join(e.uploads, "plants", name)
	return testutil.SeedPlantImage(t, e.db, e.plant, p, body), p
}

func (e *photoEnv) result(t *testing.T, id int) (sql.NullInt64, sql.NullString) {
	t.Helper()
	var q sql.NullInt64
	var r sql.NullString
	require.NoError(t, e.db.QueryRow(`SELECT compress_quality, compress_result FROM plant_images WHERE id = $1`, id).Scan(&q, &r))
	return q, r
}

func fileSize(t *testing.T, p string) int {
	t.Helper()
	st, err := os.Stat(p)
	require.NoError(t, err)
	return int(st.Size())
}

func TestPhotoCompress_FullRunWithBackup(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	big := testutil.DetailedPhotoJPEG(t)
	small, err := utils.CompressJPEG(big, 50)
	require.NoError(t, err)
	id1, p1 := e.addPhoto(t, "a.jpg", big)
	id2, p2 := e.addPhoto(t, "b.jpg", small)
	id3, _ := e.addPhoto(t, "c.png", []byte("not a jpeg"))

	est, err := e.svc.Estimate(e.db, 75)
	require.NoError(t, err)
	assert.Equal(t, 2, est.Count, "JPEGs only")
	assert.Equal(t, int64(len(big)+len(small)), est.Bytes)
	assert.Positive(t, est.Saving)

	require.NoError(t, e.svc.Start(75, true))
	e.svc.Wait()

	j := e.svc.Job()
	require.NotNil(t, j)
	assert.Equal(t, handlers.PhotoJobFinished, j.State)
	assert.Equal(t, 2, j.Total)
	assert.Equal(t, 1, j.Compressed)
	assert.Equal(t, 1, j.Kept, "under 10% saving is skipped")
	assert.Equal(t, int64(len(big)-fileSize(t, p1)), j.SavedBytes)

	assert.Less(t, fileSize(t, p1), len(big))
	assert.Equal(t, len(small), fileSize(t, p2))
	q, r := e.result(t, id1)
	assert.Equal(t, "compressed", r.String)
	assert.Equal(t, int64(75), q.Int64)
	_, r = e.result(t, id2)
	assert.Equal(t, "kept", r.String)
	_, r = e.result(t, id3)
	assert.False(t, r.Valid, "PNG untouched")

	// The originals are in the backup folder.
	backed, err := os.ReadFile(filepath.Join(j.BackupDir, "plants", "a.jpg"))
	require.NoError(t, err)
	assert.Equal(t, big, backed)
	assert.Equal(t, filepath.Join(e.data, "backups", "photos-before-compression-2026-10-27"), j.BackupDir)

	st := e.svc.Status()
	assert.Equal(t, e.now.Add(handlers.PhotoBackupKeepFor), st.DeletesAt)
	assert.InDelta(t, 100, st.Percent, 0.01)

	// Nothing left at 75 or above; a lower quality retries only the kept photo.
	est, err = e.svc.Estimate(e.db, 75)
	require.NoError(t, err)
	assert.Equal(t, 0, est.Count, "never compressed twice")
	est, err = e.svc.Estimate(e.db, 60)
	require.NoError(t, err)
	assert.Equal(t, 1, est.Count, "the kept photo is tried again at a lower quality")
}

func TestPhotoCompress_StopPausesAndResumes(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	big := testutil.DetailedPhotoJPEG(t)
	var paths []string
	for _, n := range []string{"1.jpg", "2.jpg", "3.jpg", "4.jpg"} {
		_, p := e.addPhoto(t, n, big)
		paths = append(paths, p)
	}

	var once sync.Once
	gate := make(chan struct{})
	n := 0
	handlers.SetPhotoCompressBeforePhoto(e.svc, func() {
		n++
		if n == 3 {
			once.Do(func() { close(gate) })
			time.Sleep(50 * time.Millisecond) // Stop arrives while on photo 3
		}
	})
	require.NoError(t, e.svc.Start(80, false))
	<-gate
	e.svc.Stop()

	j := e.svc.Job()
	assert.Equal(t, handlers.PhotoJobPaused, j.State)
	assert.Equal(t, 3, j.Done, "the photo in hand is finished first")
	assert.True(t, e.svc.Locked())
	assert.ErrorIs(t, e.svc.Start(80, false), handlers.ErrPhotoJobExists)

	require.NoError(t, e.svc.Resume())
	e.svc.Wait()
	j = e.svc.Job()
	assert.Equal(t, handlers.PhotoJobFinished, j.State)
	assert.Equal(t, 4, j.Compressed)
	assert.Equal(t, 4, j.Done)
	for _, p := range paths {
		assert.Less(t, fileSize(t, p), len(big))
	}
	assert.False(t, e.svc.Locked())
}

func TestPhotoCompress_RestartTurnsRunningIntoPaused(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	big := testutil.DetailedPhotoJPEG(t)
	e.addPhoto(t, "1.jpg", big)
	e.addPhoto(t, "2.jpg", big)

	gate, release := make(chan struct{}), make(chan struct{})
	first := true
	handlers.SetPhotoCompressBeforePhoto(e.svc, func() {
		if first {
			first = false
			close(gate)
			<-release
		}
	})
	require.NoError(t, e.svc.Start(75, false))
	<-gate
	// BudMinder closes here: a new service reads the saved state.
	again := e.reopen()
	j := again.Job()
	require.NotNil(t, j)
	assert.Equal(t, handlers.PhotoJobPaused, j.State)
	assert.Equal(t, handlers.PhotoJobPaused, again.Status().State)
	close(release)
	e.svc.Wait()
}

func TestPhotoCompress_FinishWithoutRemaining(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	big := testutil.DetailedPhotoJPEG(t)
	_, p1 := e.addPhoto(t, "1.jpg", big)
	_, p2 := e.addPhoto(t, "2.jpg", big)

	gate := make(chan struct{})
	first := true
	handlers.SetPhotoCompressBeforePhoto(e.svc, func() {
		if first {
			first = false
			close(gate)
			time.Sleep(50 * time.Millisecond) // Stop arrives while on photo 1
		}
	})
	require.NoError(t, e.svc.Start(75, true))
	<-gate
	e.svc.Stop()
	require.NoError(t, e.svc.Finish())

	j := e.svc.Job()
	assert.Equal(t, handlers.PhotoJobFinished, j.State)
	assert.Equal(t, 1, j.Done)
	assert.Less(t, fileSize(t, p1), len(big))
	assert.Equal(t, len(big), fileSize(t, p2), "the remaining photo stays as it was")
	assert.DirExists(t, j.BackupDir, "the backup of the compressed photo is kept")
	assert.ErrorIs(t, e.svc.Finish(), handlers.ErrPhotoJobNotPaused)
}

func TestPhotoCompress_MissingFileIsKept(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	id, p := e.addPhoto(t, "gone.jpg", testutil.DetailedPhotoJPEG(t))
	require.NoError(t, os.Remove(p))
	outside := testutil.SeedPlantImage(t, e.db, e.plant, filepath.Join(t.TempDir(), "elsewhere.jpg"), testutil.DetailedPhotoJPEG(t))
	require.NoError(t, e.svc.Start(75, false))
	e.svc.Wait()
	_, r := e.result(t, id)
	assert.Equal(t, "kept", r.String)
	_, r = e.result(t, outside)
	assert.Equal(t, "kept", r.String, "files outside the upload folder are never written")
}

func TestPhotoCompress_BackupCleanupAfter7Days(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	e.addPhoto(t, "1.jpg", testutil.DetailedPhotoJPEG(t))
	require.NoError(t, e.svc.Start(75, true))
	e.svc.Wait()
	dir := e.svc.Job().BackupDir
	require.DirExists(t, dir)

	e.advance(6 * 24 * time.Hour)
	e.svc.CleanupBackups()
	assert.DirExists(t, dir, "kept for 7 days")

	e.advance(25 * time.Hour)
	e.svc.CleanupBackups()
	assert.NoDirExists(t, dir)
	assert.True(t, e.svc.Status().BackupGone)
}

func TestPhotoCompress_DeleteBackupNow(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	assert.ErrorIs(t, e.svc.DeleteBackup(), handlers.ErrPhotoNoBackup)
	e.addPhoto(t, "1.jpg", testutil.DetailedPhotoJPEG(t))
	require.NoError(t, e.svc.Start(75, true))
	e.svc.Wait()
	dir := e.svc.Job().BackupDir
	require.NoError(t, e.svc.DeleteBackup())
	assert.NoDirExists(t, dir)
	assert.ErrorIs(t, e.svc.DeleteBackup(), handlers.ErrPhotoNoBackup)
}

func TestPhotoCompress_EarlierBackupStillCleanedUp(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	e.addPhoto(t, "1.jpg", testutil.DetailedPhotoJPEG(t))
	require.NoError(t, e.svc.Start(80, true))
	e.svc.Wait()
	first := e.svc.Job().BackupDir

	// A second run the same day (a photo added since) gets its own folder.
	e.addPhoto(t, "2.jpg", testutil.DetailedPhotoJPEG(t))
	require.NoError(t, e.svc.Start(80, true))
	e.svc.Wait()
	second := e.svc.Job().BackupDir
	assert.Equal(t, first+"-2", second)

	e.advance(8 * 24 * time.Hour)
	e.svc.CleanupBackups()
	assert.NoDirExists(t, first)
	assert.NoDirExists(t, second)
}

func TestPhotoCompress_RestoreBackup(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	big := testutil.DetailedPhotoJPEG(t)
	small, err := utils.CompressJPEG(big, 50)
	require.NoError(t, err)
	id1, p1 := e.addPhoto(t, "a.jpg", big)
	id2, p2 := e.addPhoto(t, "b.jpg", small)
	id3, p3 := e.addPhoto(t, "c.jpg", big)

	require.NoError(t, e.svc.Start(75, true))
	e.svc.Wait()
	dir := e.svc.Job().BackupDir
	assert.Equal(t, int64(2*len(big)+len(small)), e.svc.Status().BackupBytes)
	require.Less(t, fileSize(t, p1), len(big))

	// c.jpg is deleted after the run: it stays deleted.
	require.NoError(t, os.Remove(p3))
	_, err = e.db.Exec(`DELETE FROM plant_images WHERE id = $1`, id3)
	require.NoError(t, err)

	require.NoError(t, e.svc.RestoreBackup())
	got, err := os.ReadFile(p1)
	require.NoError(t, err)
	assert.Equal(t, big, got, "the original is back")
	q, r := e.result(t, id1)
	assert.False(t, q.Valid)
	assert.False(t, r.Valid, "counts as never checked again")
	_, r = e.result(t, id2)
	assert.Equal(t, "kept", r.String, "a photo the run left alone isn't touched")
	assert.Equal(t, len(small), fileSize(t, p2))
	assert.NoFileExists(t, p3)
	assert.NoDirExists(t, dir)

	st := e.svc.Status()
	assert.True(t, st.Restored)
	assert.Equal(t, 1, st.RestoredN)
	assert.True(t, st.BackupGone)

	est, err := e.svc.Estimate(e.db, 75)
	require.NoError(t, err)
	assert.Equal(t, 1, est.Count, "the restored photo can be compressed again")

	assert.ErrorIs(t, e.svc.RestoreBackup(), handlers.ErrPhotoNoBackup, "only once")
	assert.True(t, e.reopen().Job().Restored, "remembered after a restart")
}

func TestPhotoCompress_RestoreNeedsABackup(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	assert.ErrorIs(t, e.svc.RestoreBackup(), handlers.ErrPhotoNoBackup, "no run yet")

	e.addPhoto(t, "a.jpg", testutil.DetailedPhotoJPEG(t))
	require.NoError(t, e.svc.Start(75, false))
	e.svc.Wait()
	assert.ErrorIs(t, e.svc.RestoreBackup(), handlers.ErrPhotoNoBackup, "run without a backup")

	e.addPhoto(t, "b.jpg", testutil.DetailedPhotoJPEG(t))
	require.NoError(t, e.svc.Start(75, true))
	e.svc.Wait()
	require.NoError(t, e.svc.DeleteBackup())
	assert.ErrorIs(t, e.svc.RestoreBackup(), handlers.ErrPhotoNoBackup, "backup deleted")
}

func TestPhotoCompress_RestoreSharedFile(t *testing.T) {
	t.Parallel()
	e := newPhotoEnv(t)
	big := testutil.DetailedPhotoJPEG(t)
	id1, p := e.addPhoto(t, "a.jpg", big)
	id2, _ := e.addPhoto(t, "a.jpg", big) // two rows, one file

	require.NoError(t, e.svc.Start(75, true))
	e.svc.Wait()
	assert.Equal(t, int64(len(big)), e.svc.Status().BackupBytes, "the file counts once")
	_, r1 := e.result(t, id1)
	_, r2 := e.result(t, id2)
	assert.Equal(t, []string{"compressed", "kept"}, []string{r1.String, r2.String})

	require.NoError(t, e.svc.RestoreBackup())
	assert.Equal(t, len(big), fileSize(t, p))
	_, r1 = e.result(t, id1)
	_, r2 = e.result(t, id2)
	assert.False(t, r1.Valid)
	assert.False(t, r2.Valid, "the other row is reset too")
	assert.Equal(t, 1, e.svc.Status().RestoredN)
}
