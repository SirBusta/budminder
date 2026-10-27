package handlers

// Compress existing photos (Settings > Photos): one background run per
// install, one photo at a time, that re-encodes plant photos which were
// never compressed. Optionally it first copies the originals to
// data/backups/photos-before-compression-<date>, which BudMinder deletes 7
// days after the run finishes. The run's state lives in a small file in the
// data folder (not in the database, so a backup restore or a database switch
// can't bring back a stale run). BudMinder closed mid-run, or Stop, leaves it
// paused until Resume.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"isley/config"
	"isley/logger"
	"isley/utils"
)

const contextKeyPhotoCompress = "photoCompressService"

// Photo compression run states.
const (
	PhotoJobRunning  = "running"
	PhotoJobPaused   = "paused"
	PhotoJobFinished = "finished"
)

const (
	photoJobFile         = "photo-compression.json"
	photoBackupPrefix    = "photos-before-compression-"
	PhotoBackupKeepFor   = 7 * 24 * time.Hour
	photoEstimateSamples = 6
	maxCompressPhotoSize = 80 << 20 // larger files are left as they are
)

// Errors the routes turn into messages.
var (
	ErrPhotoJobExists    = errors.New("photo compression: a run already exists")
	ErrPhotoJobNotPaused = errors.New("photo compression: no paused run")
	ErrPhotoJobRestore   = errors.New("photo compression: a restore is running")
	ErrPhotoNoBackup     = errors.New("photo compression: no backup folder")
)

// PhotoBackup is a folder of original photos copied before a run.
type PhotoBackup struct {
	Dir        string    `json:"dir"` // under <data>/backups
	FinishedAt time.Time `json:"finished_at"`
}

// PhotoJob is the saved state of a run.
type PhotoJob struct {
	State       string        `json:"state"`
	Quality     int           `json:"quality"`
	Backup      bool          `json:"backup"`
	BackupDir   string        `json:"backup_dir,omitempty"`
	BackupDone  bool          `json:"backup_done,omitempty"` // originals copied
	BackupGone  bool          `json:"backup_gone,omitempty"` // deleted (by the user or after 7 days)
	Total       int           `json:"total"`
	Done        int           `json:"done"`
	Compressed  int           `json:"compressed"`
	Kept        int           `json:"kept"`
	BytesBefore int64         `json:"bytes_before"`
	SavedBytes  int64         `json:"saved_bytes"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  time.Time     `json:"finished_at,omitempty"`
	Error       string        `json:"error,omitempty"` // why the run paused itself (e.g. disk full)
	Interrupted bool          `json:"interrupted,omitempty"` // paused because BudMinder was closed
	Earlier     []PhotoBackup `json:"earlier,omitempty"`
}

// PhotoEstimate is what compressing the remaining photos would do.
type PhotoEstimate struct {
	Count  int   `json:"count"`
	Bytes  int64 `json:"bytes"`
	Saving int64 `json:"estimated_saving"`
}

// PhotoCompressService owns the run. One per engine (app.NewEngine), reached
// through the Gin context, never a package global.
type PhotoCompressService struct {
	db        *sql.DB
	dataDir   string
	uploadDir string
	// Desktop is true for the downloadable programs (not Docker): the
	// backup path is shown in full and Open folder is offered.
	Desktop bool

	now func() time.Time
	// beforePhoto, when set (tests), runs before each photo of phase 2.
	beforePhoto func()

	mu       sync.Mutex
	job      *PhotoJob
	stopReq  bool
	active   bool          // the run goroutine is working
	finished chan struct{} // closed when the run goroutine ends
	runStart time.Time     // this goroutine's start, for the time left
	runFrom  int           // job.Done when it started
	backupN  int           // phase 1 progress (memory only)
	backupOf int

	compare *photoComparer // the compare page's cache

	estMu    sync.Mutex
	estKey   string
	estCache map[int]PhotoEstimate
}

// NewPhotoCompressService loads the saved run, if any. A run saved as
// running means BudMinder was closed during it: it becomes paused.
func NewPhotoCompressService(db *sql.DB, dataDir, uploadDir string) *PhotoCompressService {
	if dataDir == "" {
		dataDir = "data"
	}
	if uploadDir == "" {
		uploadDir = DefaultUploadDir
	}
	s := &PhotoCompressService{db: db, dataDir: dataDir, uploadDir: uploadDir, now: time.Now, compare: newPhotoComparer()}
	if b, err := os.ReadFile(s.jobPath()); err == nil {
		var j PhotoJob
		if json.Unmarshal(b, &j) == nil && j.State != "" {
			s.job = &j
			if j.State == PhotoJobRunning {
				j.State, j.Interrupted = PhotoJobPaused, true
				s.saveLocked()
			}
		}
	}
	return s
}

func (s *PhotoCompressService) jobPath() string { return filepath.Join(s.dataDir, photoJobFile) }

// saveLocked writes the run's state (call with mu held).
func (s *PhotoCompressService) saveLocked() {
	if s.job == nil {
		_ = os.Remove(s.jobPath())
		return
	}
	b, _ := json.MarshalIndent(s.job, "", "  ")
	if err := os.MkdirAll(s.dataDir, 0o755); err == nil {
		tmp := s.jobPath() + ".tmp"
		if err = os.WriteFile(tmp, b, 0o644); err == nil {
			err = os.Rename(tmp, s.jobPath())
		}
		if err != nil {
			logger.Log.WithError(err).Warn("Photo compression: couldn't save the run's state")
		}
	}
}

// Job returns a copy of the run's state, or nil.
func (s *PhotoCompressService) Job() *PhotoJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return nil
	}
	j := *s.job
	return &j
}

// Locked reports whether a run is running or paused; the photo settings
// can't change then.
func (s *PhotoCompressService) Locked() bool {
	j := s.Job()
	return j != nil && j.State != PhotoJobFinished
}

// ---------------------------------------------------------------- candidates

// photoCandidatesSQL selects the plant photos a run at quality $1 works on:
// never checked, or kept at a higher quality (still an untouched original).
// Compressed photos are never compressed again. JPEG files only.
const photoCandidatesSQL = `FROM plant_images
	WHERE (compress_result IS NULL OR (compress_result = 'kept' AND compress_quality > $1))
	AND (LOWER(image_path) LIKE '%.jpg' OR LOWER(image_path) LIKE '%.jpeg')`

type photoCandidate struct {
	id   int
	path string
}

func photoCandidates(db *sql.DB, q int) ([]photoCandidate, error) {
	rows, err := db.Query(`SELECT id, image_path `+photoCandidatesSQL+` ORDER BY id`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []photoCandidate
	for rows.Next() {
		var p photoCandidate
		if err := rows.Scan(&p.id, &p.path); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// photoFile resolves a stored image path to a file inside the upload
// folder, or "" when it points anywhere else.
func photoFile(uploadDir, stored string) string {
	root, err1 := filepath.Abs(uploadDir)
	p, err2 := filepath.Abs(filepath.FromSlash(stored))
	if err1 != nil || err2 != nil || !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return ""
	}
	return p
}

// Estimate counts the photos a run at quality q would work on, their size,
// and the likely saving from compressing a few of them in memory. db is the
// request's database (a demo visitor's own copy).
func (s *PhotoCompressService) Estimate(db *sql.DB, q int) (PhotoEstimate, error) {
	var n int
	var maxID sql.NullInt64
	if err := db.QueryRow(`SELECT COUNT(*), MAX(id) `+photoCandidatesSQL, q).Scan(&n, &maxID); err != nil {
		return PhotoEstimate{}, err
	}
	key := fmt.Sprintf("%p/%d/%d", db, n, maxID.Int64)
	s.estMu.Lock()
	if s.estKey != key {
		s.estKey, s.estCache = key, map[int]PhotoEstimate{}
	}
	if e, ok := s.estCache[q]; ok {
		s.estMu.Unlock()
		return e, nil
	}
	s.estMu.Unlock()

	list, err := photoCandidates(db, q)
	if err != nil {
		return PhotoEstimate{}, err
	}
	e := PhotoEstimate{}
	var files []string
	for _, p := range list {
		f := photoFile(s.uploadDir, p.path)
		if f == "" {
			continue
		}
		if st, err := os.Stat(f); err == nil {
			e.Count++
			e.Bytes += st.Size()
			files = append(files, f)
		}
	}
	rand.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
	var before, after int64
	for _, f := range files[:min(len(files), photoEstimateSamples)] {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		before += int64(len(b))
		if comp, err := utils.CompressJPEG(b, q); err == nil && utils.ShouldReplace(len(b), len(comp)) {
			after += int64(len(comp))
		} else {
			after += int64(len(b))
		}
	}
	if before > 0 {
		e.Saving = int64(float64(e.Bytes) * float64(before-after) / float64(before))
	}
	s.estMu.Lock()
	if s.estKey == key {
		s.estCache[q] = e
	}
	s.estMu.Unlock()
	return e, nil
}

// ---------------------------------------------------------------- run control

// Start begins a run at quality q, with or without copying the originals
// first. Refused while a run is running or paused, or during a restore.
func (s *PhotoCompressService) Start(q int, backup bool) error {
	if config.RestoreInProgress.Load() {
		return ErrPhotoJobRestore
	}
	list, err := photoCandidates(s.db, q)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active || (s.job != nil && s.job.State != PhotoJobFinished) {
		return ErrPhotoJobExists
	}
	j := &PhotoJob{State: PhotoJobRunning, Quality: q, Backup: backup, Total: len(list), StartedAt: s.now()}
	if old := s.job; old != nil {
		j.Earlier = old.Earlier
		if old.BackupDir != "" && !old.BackupGone {
			j.Earlier = append(j.Earlier, PhotoBackup{Dir: old.BackupDir, FinishedAt: old.FinishedAt})
		}
	}
	for _, p := range list {
		if f := photoFile(s.uploadDir, p.path); f != "" {
			if st, err := os.Stat(f); err == nil {
				j.BytesBefore += st.Size()
			}
		}
	}
	if backup {
		j.BackupDir = s.newBackupDir()
	}
	s.job = j
	s.saveLocked()
	s.launchLocked()
	return nil
}

// newBackupDir picks data/backups/photos-before-compression-<date>[-n].
func (s *PhotoCompressService) newBackupDir() string {
	base := filepath.Join(s.dataDir, "backups", photoBackupPrefix+s.now().Format("2006-01-02"))
	dir := base
	for n := 2; ; n++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return dir
		}
		dir = fmt.Sprintf("%s-%d", base, n)
	}
}

// Resume continues a paused run.
func (s *PhotoCompressService) Resume() error {
	if config.RestoreInProgress.Load() {
		return ErrPhotoJobRestore
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.State != PhotoJobPaused || s.active {
		return ErrPhotoJobNotPaused
	}
	s.job.State, s.job.Error, s.job.Interrupted = PhotoJobRunning, "", false
	s.saveLocked()
	s.launchLocked()
	return nil
}

// Stop pauses a running run after the photo it's on and waits for that.
func (s *PhotoCompressService) Stop() {
	s.mu.Lock()
	done := s.finished
	if s.active {
		s.stopReq = true
	}
	s.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
		}
	}
}

// Finish ends a paused run without the remaining photos.
func (s *PhotoCompressService) Finish() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.State != PhotoJobPaused || s.active {
		return ErrPhotoJobNotPaused
	}
	s.finishLocked()
	return nil
}

func (s *PhotoCompressService) finishLocked() {
	s.job.State, s.job.FinishedAt, s.job.Error, s.job.Interrupted = PhotoJobFinished, s.now(), "", false
	if s.job.BackupDir != "" && !s.job.BackupDone {
		// Stopped while copying: no photo was compressed, so the partial
		// copy isn't needed.
		_ = os.RemoveAll(s.job.BackupDir)
		s.job.BackupGone = true
	}
	s.saveLocked()
}

// Wait blocks until the run goroutine (if any) ends. For tests and shutdown.
func (s *PhotoCompressService) Wait() {
	s.mu.Lock()
	done := s.finished
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (s *PhotoCompressService) launchLocked() {
	s.active, s.stopReq = true, false
	s.finished = make(chan struct{})
	s.runStart, s.runFrom = s.now(), s.job.Done
	s.backupN, s.backupOf = 0, 0
	go s.run(s.finished)
}

// stopping reports (and handles) a Stop or a restore between photos.
func (s *PhotoCompressService) stopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopReq || config.RestoreInProgress.Load() {
		s.job.State = PhotoJobPaused
		s.saveLocked()
		return true
	}
	return false
}

func (s *PhotoCompressService) pauseWith(err error) {
	logger.Log.WithError(err).Warn("Photo compression paused")
	s.mu.Lock()
	s.job.State, s.job.Error = PhotoJobPaused, err.Error()
	s.saveLocked()
	s.mu.Unlock()
}

func (s *PhotoCompressService) run(done chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			s.pauseWith(fmt.Errorf("unexpected error: %v", r))
		}
		s.mu.Lock()
		s.active = false
		s.mu.Unlock()
		close(done)
	}()
	j := s.Job()
	list, err := photoCandidates(s.db, j.Quality)
	if err != nil {
		s.pauseWith(err)
		return
	}

	// Phase 1: copy the originals (files already copied are skipped, so a
	// resumed run picks up where it left off).
	if j.Backup && !j.BackupDone {
		s.mu.Lock()
		s.backupOf = len(list)
		s.mu.Unlock()
		dst := filepath.Join(j.BackupDir, "plants")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			s.pauseWith(err)
			return
		}
		for i, p := range list {
			if s.stopping() {
				return
			}
			if f := photoFile(s.uploadDir, p.path); f != "" {
				if err := copyIfChanged(f, filepath.Join(dst, filepath.Base(f))); err != nil && !os.IsNotExist(err) {
					s.pauseWith(err)
					return
				}
			}
			s.mu.Lock()
			s.backupN = i + 1
			s.mu.Unlock()
		}
		s.mu.Lock()
		s.job.BackupDone = true
		s.saveLocked()
		s.mu.Unlock()
	}

	// Phase 2: compress, one photo at a time.
	for _, p := range list {
		if s.stopping() {
			return
		}
		if s.beforePhoto != nil {
			s.beforePhoto()
		}
		result, saved, err := s.compressOne(p, j.Quality)
		if err != nil {
			s.pauseWith(err)
			return
		}
		if _, err := s.db.Exec(`UPDATE plant_images SET compress_quality = $1, compress_result = $2 WHERE id = $3`,
			j.Quality, result, p.id); err != nil {
			s.pauseWith(err)
			return
		}
		s.mu.Lock()
		s.job.Done++
		if result == PhotoCompressed {
			s.job.Compressed++
			s.job.SavedBytes += saved
		} else {
			s.job.Kept++
		}
		if s.job.Done > s.job.Total {
			s.job.Total = s.job.Done
		}
		s.saveLocked()
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.finishLocked()
	compressed, kept := s.job.Compressed, s.job.Kept
	s.mu.Unlock()
	logger.Log.WithField("compressed", compressed).WithField("kept", kept).Info("Photo compression finished")
}

// compressOne compresses one photo in place. A missing, unreadable or
// non-compressible photo is "kept"; an error means the run must pause
// (e.g. the disk is full).
func (s *PhotoCompressService) compressOne(p photoCandidate, q int) (string, int64, error) {
	f := photoFile(s.uploadDir, p.path)
	if f == "" {
		return PhotoKept, 0, nil
	}
	st, err := os.Stat(f)
	if err != nil || st.Size() > maxCompressPhotoSize {
		return PhotoKept, 0, nil
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return PhotoKept, 0, nil
	}
	comp, err := utils.CompressJPEG(b, q)
	if err != nil || !utils.ShouldReplace(len(b), len(comp)) {
		return PhotoKept, 0, nil
	}
	if err := replaceFile(f, comp); err != nil {
		return "", 0, err
	}
	return PhotoCompressed, int64(len(b) - len(comp)), nil
}

// replaceFile writes b over path through a temporary file in the same
// folder, so the photo is either the old one or the new one, never half.
func replaceFile(path string, b []byte) error {
	tmp := path + ".compress-tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// copyIfChanged copies src to dst unless dst already has the same size.
func copyIfChanged(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if di, err := os.Stat(dst); err == nil && di.Size() == si.Size() {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		_ = os.Chtimes(dst, si.ModTime(), si.ModTime())
	}
	return err
}

// ---------------------------------------------------------------- backups

// isPhotoBackupDir reports whether dir is a photo backup folder in this
// install's data/backups (nothing else is ever deleted).
func (s *PhotoCompressService) isPhotoBackupDir(dir string) bool {
	root, err1 := filepath.Abs(filepath.Join(s.dataDir, "backups"))
	d, err2 := filepath.Abs(dir)
	return err1 == nil && err2 == nil && filepath.Dir(d) == root &&
		strings.HasPrefix(filepath.Base(d), photoBackupPrefix)
}

// DeleteBackup deletes the finished run's backup folder now.
func (s *PhotoCompressService) DeleteBackup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.State != PhotoJobFinished || s.job.BackupDir == "" || s.job.BackupGone ||
		!s.isPhotoBackupDir(s.job.BackupDir) {
		return ErrPhotoNoBackup
	}
	if err := os.RemoveAll(s.job.BackupDir); err != nil {
		return err
	}
	s.job.BackupGone = true
	s.saveLocked()
	return nil
}

// CleanupBackups deletes backup folders whose run finished more than 7
// days ago.
func (s *PhotoCompressService) CleanupBackups() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return
	}
	now, changed := s.now(), false
	expired := func(b PhotoBackup) bool {
		return !b.FinishedAt.IsZero() && now.Sub(b.FinishedAt) > PhotoBackupKeepFor
	}
	remove := func(dir string) bool {
		if !s.isPhotoBackupDir(dir) {
			return true // not ours to delete: forget it
		}
		if err := os.RemoveAll(dir); err != nil {
			logger.Log.WithError(err).WithField("dir", dir).Warn("Photo compression: couldn't delete an old backup folder")
			return false
		}
		logger.Log.WithField("dir", dir).Info("Photo compression: deleted the 7-day backup folder")
		return true
	}
	kept := s.job.Earlier[:0]
	for _, b := range s.job.Earlier {
		if expired(b) && remove(b.Dir) {
			changed = true
			continue
		}
		kept = append(kept, b)
	}
	s.job.Earlier = kept
	if s.job.State == PhotoJobFinished && s.job.BackupDir != "" && !s.job.BackupGone &&
		expired(PhotoBackup{Dir: s.job.BackupDir, FinishedAt: s.job.FinishedAt}) && remove(s.job.BackupDir) {
		s.job.BackupGone, changed = true, true
	}
	if changed {
		s.saveLocked()
	}
}

// RunCleanup deletes expired backup folders now and then hourly until ctx
// ends (started from main, like the update checker).
func (s *PhotoCompressService) RunCleanup(done <-chan struct{}) {
	s.CleanupBackups()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			s.CleanupBackups()
		}
	}
}

// ---------------------------------------------------------------- status

// PhotoJobStatus is what Settings and the top line show.
type PhotoJobStatus struct {
	State       string    `json:"state"` // "", running, paused, finished
	Phase       string    `json:"phase,omitempty"`
	Quality     int       `json:"quality,omitempty"`
	Total       int       `json:"total"`
	Done        int       `json:"done"`
	Left        int       `json:"left"`
	Compressed  int       `json:"compressed"`
	Kept        int       `json:"kept"`
	BytesBefore int64     `json:"bytes_before"`
	SavedBytes  int64     `json:"saved_bytes"`
	BackupDone  int       `json:"backup_done"`
	BackupTotal int       `json:"backup_total"`
	Percent     float64   `json:"percent"`
	SecondsLeft int       `json:"seconds_left"`
	BackupPath  string    `json:"backup_path,omitempty"`
	BackupGone  bool      `json:"backup_gone"`
	DeletesAt   time.Time `json:"deletes_at,omitempty"`
	CanOpen     bool      `json:"can_open"`
	Error       string    `json:"error,omitempty"`
	Interrupted bool      `json:"interrupted"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
}

// Status describes the run for the pages.
func (s *PhotoCompressService) Status() PhotoJobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return PhotoJobStatus{}
	}
	j := s.job
	st := PhotoJobStatus{
		State: j.State, Quality: j.Quality, Total: j.Total, Done: j.Done, Left: max(0, j.Total-j.Done),
		Compressed: j.Compressed, Kept: j.Kept, BytesBefore: j.BytesBefore, SavedBytes: j.SavedBytes,
		BackupGone: j.BackupGone, Error: j.Error, Interrupted: j.Interrupted, FinishedAt: j.FinishedAt,
	}
	if j.State == PhotoJobRunning {
		st.Phase = "compress"
		if j.Backup && !j.BackupDone {
			st.Phase = "backup"
			st.BackupDone, st.BackupTotal = s.backupN, s.backupOf
		}
	}
	// Overall progress: copying counts as one more pass over the photos.
	steps, at := float64(j.Total), float64(j.Done)
	if j.Backup {
		steps *= 2
		if j.BackupDone {
			at += float64(j.Total)
		} else {
			at = float64(s.backupN)
		}
	}
	if steps > 0 {
		st.Percent = min(100, at/steps*100)
	} else if j.State == PhotoJobFinished {
		st.Percent = 100
	}
	if j.State == PhotoJobRunning && st.Phase == "compress" {
		if n := j.Done - s.runFrom; n > 0 {
			per := s.now().Sub(s.runStart) / time.Duration(n)
			st.SecondsLeft = int((per * time.Duration(st.Left)).Seconds())
		}
	}
	if j.BackupDir != "" {
		st.BackupPath = j.BackupDir
		if s.Desktop {
			if abs, err := filepath.Abs(j.BackupDir); err == nil {
				st.BackupPath = abs
			}
			st.CanOpen = !j.BackupGone
		}
		if j.State == PhotoJobFinished && !j.BackupGone {
			st.DeletesAt = j.FinishedAt.Add(PhotoBackupKeepFor)
		}
	}
	return st
}

// BackupDirToOpen returns the finished run's backup folder, for Open folder.
func (s *PhotoCompressService) BackupDirToOpen() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.BackupDir == "" || s.job.BackupGone || !s.isPhotoBackupDir(s.job.BackupDir) {
		return "", ErrPhotoNoBackup
	}
	return filepath.Abs(s.job.BackupDir)
}

// PhotoCompressFromContext returns the engine's photo compression service.
func PhotoCompressFromContext(c *gin.Context) *PhotoCompressService {
	v, _ := c.Get(contextKeyPhotoCompress)
	s, _ := v.(*PhotoCompressService)
	return s
}

// SetPhotoCompressOnContext binds the service to a request (engine middleware).
func SetPhotoCompressOnContext(c *gin.Context, s *PhotoCompressService) {
	c.Set(contextKeyPhotoCompress, s)
}
