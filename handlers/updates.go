package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"

	"isley/launcher"
	"isley/logger"
	"isley/model"
	"isley/updates"
)

// The "New release" notice in the top menu (common/header2.html,
// web/static/js/update.js): what's new, and for the downloadable programs an
// Update now that installs it and restarts BudMinder (package updates).

// UpdateCheckDisabledSetting turns the release check off ("1"); unset or
// "0" means on, the default.
const UpdateCheckDisabledSetting = "update_check.disabled"

const contextKeyUpdates = "updates"

// Updates is what the update notice needs from the engine.
type Updates struct {
	Checker *updates.Checker
	Restart func() // restarts BudMinder into the new program; nil: can't install from the page
	DataDir string
	Exe     string // the program file to replace; "" is the running program (tests set a scratch file)

	installing sync.Mutex
}

// SetUpdatesOnContext binds the engine's update checker to a request.
func SetUpdatesOnContext(c *gin.Context, u *Updates) {
	c.Set(contextKeyUpdates, u)
}

func updatesFromContext(c *gin.Context) *Updates {
	if v, ok := c.Get(contextKeyUpdates); ok {
		if u, ok := v.(*Updates); ok && u != nil && u.Checker != nil {
			return u
		}
	}
	return nil
}

// UpdateStatusHandler: GET /updates/status, the window's contents.
func UpdateStatusHandler(c *gin.Context) {
	u := updatesFromContext(c)
	if u == nil {
		c.JSON(http.StatusOK, gin.H{"available": false})
		return
	}
	rel := u.Checker.Available()
	out := gin.H{"available": rel != nil, "current": u.Checker.Current(), "docker": launcher.InDocker()}
	if rel != nil {
		notes := bluemonday.UGCPolicy().SanitizeBytes(blackfriday.Run([]byte(rel.Notes)))
		out["latest"] = rel.Version
		out["url"] = rel.URL
		out["notes_html"] = string(notes)
		out["can_install"] = u.Restart != nil && !launcher.InDocker() && rel.CanInstall()
	}
	c.JSON(http.StatusOK, out)
}

// UpdateInstallHandler: POST /updates/install. Downloads and checks the new
// program, backs up the database, swaps the program file, answers, then
// restarts BudMinder. The page waits for the new version to answer.
func UpdateInstallHandler(c *gin.Context) {
	u := updatesFromContext(c)
	if u == nil || u.Restart == nil || launcher.InDocker() {
		apiError(c, http.StatusBadRequest, "update_cant_install")
		return
	}
	rel := u.Checker.Available()
	if rel == nil {
		apiError(c, http.StatusConflict, "update_none")
		return
	}
	if !u.installing.TryLock() {
		apiError(c, http.StatusConflict, "update_in_progress")
		return
	}
	defer u.installing.Unlock()

	exe, err := u.Exe, error(nil)
	if exe == "" {
		if exe, err = os.Executable(); err == nil {
			exe, err = filepath.EvalSymlinks(exe)
		}
	}
	if err != nil {
		apiInternalError(c, "update_failed")
		return
	}
	db := DBFromContext(c)
	backup := func() error {
		if !model.IsSQLite() {
			return nil // PostgreSQL keeps its own data; nothing to copy here
		}
		dir := updates.BackupDir(u.DataDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, fmt.Sprintf("before-%s-%s.db", rel.Version, time.Now().Format("20060102-150405")))
		_, err := db.Exec("VACUUM INTO $1", path)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute) // not the request's: a closed tab mustn't stop a half-done update
	defer cancel()
	if err := u.Checker.Install(ctx, rel, exe, backup); err != nil {
		logger.Log.WithError(err).Error("Update failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": T(c, "update_failed"), "detail": err.Error()})
		return
	}
	logger.Log.WithField("version", rel.Version).Info("Update installed; restarting")
	c.JSON(http.StatusOK, gin.H{"version": rel.Version})
	go func() {
		time.Sleep(time.Second) // let the answer reach the page first
		u.Restart()
	}()
}
