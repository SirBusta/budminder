package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"isley/app"
	"isley/config"
	"isley/demo"
	"isley/env"
	"isley/handlers"
	"isley/homeassistant"
	"isley/launcher"
	"isley/logger"
	"isley/model"
	"isley/utils"
	"isley/watcher"
	"isley/workdir"
)

//go:embed model/migrations/sqlite/*.sql model/migrations/postgres/*.sql web/templates/**/*.html web/static/**/* web/manual/* web/manual/en/* utils/fonts/* VERSION
var embeddedFiles embed.FS

// trustedProxies trusts loopback, RFC-1918 private ranges, and the IPv6
// unique-local block (fc00::/7) so that c.ClientIP() returns the real
// client IP behind a reverse proxy. Users running without a proxy are
// unaffected.
var trustedProxies = []string{
	"127.0.0.1",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"::1",
	"fc00::/7",
}

func main() {
	// Run in the program's own folder, so data, uploads and logs sit next to
	// it however it was started (workdir explains when it stays put).
	where, whereErr := workdir.Apply()
	firstRunHome()
	logger.InitLogger()
	if whereErr != nil {
		logger.Log.WithError(whereErr).Warn("Couldn't move to the program's folder; data stays in the current folder")
	} else if where.Moved {
		logger.Log.Info("Running in the program's folder: ", where.Dir)
	} else if where.Reason != "" {
		logger.Log.Info("Running in ", where.Dir, " (", where.Reason, ")")
	}

	version := fmt.Sprintf("BudMinder %s", getVersion())
	logger.Log.Info("Starting application version:", version)

	// Expose the bare version to handlers for outbound User-Agent strings
	// (e.g. the CannaDB import client).
	handlers.Version = getVersion()
	homeassistant.UserAgent = fmt.Sprintf("BudMinder/%s (+https://github.com/SirBusta/budminder)", handlers.Version)

	port := env.Get("PORT")
	if port == "" {
		port = "8080"
	}

	// The desktop icon's jobs (launcher.MakeIcons): --stop stops the BudMinder
	// running from this folder; --open opens the browser, starting BudMinder
	// first when it isn't running yet. On Windows that start happens in a
	// second, windowless copy (--background), so no console stays open.
	// Double-clicking the .exe itself also opens the browser once ready.
	openWhenReady := runtime.GOOS == "windows" && len(os.Args) == 1 && launcher.IsTerminal(os.Stdout)
	for _, a := range os.Args[1:] {
		switch a {
		case launcher.StopFlag:
			if err := launcher.Stop("data"); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Println("BudMinder is stopping.")
			return
		case launcher.OpenFlag:
			if launcher.Running(port) {
				if err := launcher.OpenBrowser("http://localhost:" + port); err != nil {
					logger.Log.WithError(err).Error("Couldn't open the browser")
				}
				return
			}
			if runtime.GOOS == "windows" {
				if err := launcher.StartHidden(exePath(), launcher.BackgroundFlag); err == nil {
					return
				}
			}
			openWhenReady = true
		case launcher.BackgroundFlag, launcher.InstalledFromFlag:
			openWhenReady = true
		}
	}

	model.MigrateDB()
	model.InitDB()
	model.RunStartupMaintenance()
	dbDriver := model.GetDriver()
	version = fmt.Sprintf("%s-%s", version, dbDriver)

	utils.Init("en")

	db, err := model.GetDB()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to open database for credential init")
	}

	// Seed default admin credentials if absent. Pass nil for the store
	// because the engine (and its Store) hasn't been constructed yet —
	// these rows are loaded into the Store by the LoadSettings call below.
	if err := handlers.SeedDefaultAdmin(db); err != nil {
		logger.Log.WithError(err).Error("Error checking if default admin credentials are present")
	}
	if n, err := handlers.RepairLineageBrackets(db); err != nil {
		logger.Log.WithError(err).Error("Failed to repair split parent strain names")
	} else if n > 0 {
		logger.Log.WithField("strains", n).Info("Repaired parent strain names split inside brackets")
	}

	// Construct the per-process configuration store and load DB-backed
	// settings into it before any background work runs. The same store is
	// threaded into app.NewEngine, the watcher, and the grabber.
	configStore := config.NewStore()
	handlers.LoadSettings(db, configStore)

	if configStore.SensorRetention() <= 0 {
		logger.Log.Warn("Sensor data retention is disabled (sensor_retention_days = 0). " +
			"Sensor data will grow indefinitely. Consider setting a retention period " +
			"(e.g. 90 days) in Settings to prevent unbounded database growth.")
	}

	// Cancellable context for graceful shutdown of background goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Background services run inside this WaitGroup so shutdown can wait
	// for in-flight iterations to complete before exiting.
	var bgWG sync.WaitGroup

	// Demo mode: a public, locked-down demo (handlers/demo.go). It runs no
	// background jobs that reach outside: no sensor polling, camera
	// snapshots or breeder-list refresh.
	demoMode := env.Get("DEMO") == "1" || strings.EqualFold(env.Get("DEMO"), "true")
	var demoCopies *demo.Copies
	if demoMode {
		logger.Log.Warn("Demo mode is on: visitors are signed in automatically and changes are limited")
		if !model.IsSQLite() {
			logger.Log.Fatal("Demo mode needs SQLite (each visitor gets a copy of the database file)")
		}
		maxCopies, _ := strconv.Atoi(env.Get("DEMO_MAX_COPIES"))
		basePath, _, _ := strings.Cut(model.DbPath(), "?")
		demoCopies, err = demo.New(db, basePath, configStore, filepath.Join("data", "demo-copies"), maxCopies)
		if err != nil {
			logger.Log.WithError(err).Fatal("Demo mode: couldn't prepare visitor copies")
		}
		bgWG.Add(1)
		go func() {
			defer bgWG.Done()
			demoCopies.Start(ctx)
		}()
	}

	w := watcher.New(db, configStore)
	w.ACIRelogin = handlers.NewACIRelogin(db, configStore, "data", "") // same data folder as the engine

	if !demoMode {
		// Prune old sensor data once before the watcher loop kicks in.
		if err := w.PruneSensorData(); err != nil {
			logger.Log.WithError(err).Error("Initial sensor data prune failed")
		} else {
			logger.Log.Info("Initial sensor data prune completed")
		}

		bgWG.Add(1)
		go func() {
			defer bgWG.Done()
			w.Run(ctx)
		}()
	}

	// Resolve the session secret: BUDMINDER_SESSION_SECRET if set, otherwise
	// a key BudMinder made once and keeps in its database, so logins survive
	// restarts. Only if that fails is a throwaway key used.
	sessionSecret := []byte(env.Get("SESSION_SECRET"))
	if len(sessionSecret) == 0 {
		saved, err := handlers.LoadOrCreateSessionSecret(db)
		if err == nil {
			sessionSecret = saved
			logger.Log.Info("Using the session key saved in the database (set BUDMINDER_SESSION_SECRET to choose your own)")
		} else {
			logger.Log.WithError(err).Warn("Couldn't save a session key; using a temporary one (logins will not survive a restart)")
			sessionSecret = make([]byte, 32)
			if _, err := rand.Read(sessionSecret); err != nil {
				logger.Log.WithError(err).Fatal("Failed to generate random session key")
			}
		}
	}

	if os.Getenv("GIN_MODE") == "" {
		// Default to release mode in production; override with GIN_MODE=debug.
		gin.SetMode(gin.ReleaseMode)
	}

	// Resolve the engine's path defaults up front so the grabber and the
	// HTTP handlers see exactly the same on-disk directories. NewEngine
	// applies the same rules internally; we apply them here too so we
	// can read the resolved FrameDir before constructing the engine.
	hstsMaxAge := 0
	if v := env.Get("HSTS_MAX_AGE"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			logger.Log.WithField("BUDMINDER_HSTS_MAX_AGE", v).Warn("Ignoring invalid BUDMINDER_HSTS_MAX_AGE; expected non-negative integer (seconds)")
		} else {
			hstsMaxAge = parsed
		}
	}

	engineCfg := app.ResolvePathDefaults(app.Config{
		DB:                    db,
		Assets:                embeddedFiles,
		Version:               version,
		SessionSecret:         sessionSecret,
		SecureCookies:         handlers.SecureCookies,
		HSTSMaxAge:            hstsMaxAge,
		HSTSIncludeSubdomains: strings.EqualFold(env.Get("HSTS_INCLUDE_SUBDOMAINS"), "true"),
		HSTSPreload:           strings.EqualFold(env.Get("HSTS_PRELOAD"), "true"),
		GuestMode:             configStore.GuestMode() == 1,
		Demo:                  demoMode,
		TrustedProxies:        trustedProxies,
		DataDir:               "data",
		ConfigStore:           configStore,
	})
	if demoCopies != nil { // never a typed nil in the interface
		engineCfg.DemoCopies = demoCopies
	}
	engine, err := app.NewEngine(engineCfg)
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to construct HTTP engine")
	}

	if !demoMode {
		grabber := watcher.NewGrabber(configStore, engineCfg.FrameDir)
		bgWG.Add(1)
		go func() {
			defer bgWG.Done()
			grabber.Run(ctx)
		}()

		bgWG.Add(1)
		go func() {
			defer bgWG.Done()
			handlers.RunBreederReferenceRefresher(ctx, db, embeddedFiles)
		}()
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: engine,
	}

	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		logger.Log.WithError(err).Fatal("HTTP server error")
	}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Log.WithError(err).Fatal("HTTP server error")
		}
	}()

	logger.Log.WithField("port", port).Info("Server started")

	// The downloadable program: the welcome box, the desktop icon, and the
	// browser when the icon started BudMinder. Not in Docker or the demo.
	var stopRequested <-chan struct{} // "Stop BudMinder" (--stop); nil blocks forever
	if !demoMode && !launcher.InDocker() {
		if ctl, err := launcher.StartControl("data"); err != nil {
			logger.Log.WithError(err).Warn("Stop BudMinder won't work this time")
		} else {
			defer ctl.Close()
			stopRequested = ctl.Stop
		}
		time.Sleep(1500 * time.Millisecond) // let the startup jobs log first, so the box ends up last
		welcome(db, port)
		if openWhenReady {
			if err := launcher.OpenBrowser("http://localhost:" + port); err != nil {
				logger.Log.WithError(err).Error("Couldn't open the browser")
			}
		}
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-stopRequested:
	}

	logger.Log.Info("Shutdown signal received, stopping gracefully...")

	cancel()
	bgWG.Wait()
	logger.Log.Info("Background goroutines stopped")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Log.WithError(err).Error("HTTP server forced to shutdown")
	}

	logger.Log.Info("Server exited cleanly")
}

// firstRunHome: a program started from Downloads or OneDrive with no data
// there yet asks for a proper folder, copies itself into it and runs from
// there (launcher.ChooseHome). Cancel carries on here, with a warning.
func firstRunHome() {
	exe := exePath()
	_, dbFileSet := env.Lookup("DB_FILE")
	if exe == "" || len(os.Args) > 1 || dbFileSet || env.Get("DEMO") != "" || launcher.InDocker() {
		return
	}
	dir := filepath.Dir(exe)
	risky := launcher.RiskyFolder(dir)
	if risky == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "isley.db")); err == nil {
		return // already running from there: the welcome box warns instead
	}
	chosen := launcher.ChooseHome(risky)
	if chosen == "" {
		return
	}
	installed, err := launcher.Install(exe, chosen)
	if err != nil {
		fmt.Println("Couldn't set BudMinder up there:", err)
		return
	}
	os.Exit(launcher.RunInstalled(installed, exe))
}

// installedFrom is the download this copy was set up from (--installed-from).
func installedFrom() string {
	for i, a := range os.Args {
		if a == launcher.InstalledFromFlag && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

// exePath is this program's file, links followed.
func exePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return exe
}

// welcome makes the desktop and menu icons on first run (Linux, Windows) and
// prints the welcome box: where to open BudMinder, the first sign-in while it
// still works, and a warning when the program sits in Downloads or OneDrive.
func welcome(db *sql.DB, port string) {
	w := launcher.Welcome{Port: port, LANAddress: launcher.LANAddress(), FromConsole: launcher.IsTerminal(os.Stdout), OS: runtime.GOOS, Installed: installedFrom()}
	if force, _ := handlers.GetSetting(db, "force_password_change"); force == "true" {
		w.Username, _ = handlers.GetSetting(db, "auth_username")
		w.Password = handlers.DefaultAdminPassword
	}
	if exe := exePath(); exe != "" {
		// No icons for a program in Downloads or OneDrive: they'd point at a
		// file that's about to be moved.
		if w.Risky = launcher.RiskyFolder(filepath.Dir(exe)); w.Risky != "" {
			w.Suggest = launcher.SuggestedHome()
		} else if icon, err := embeddedFiles.ReadFile("web/static/img/brand/budminder-256.png"); err == nil {
			made, err := launcher.MakeIcons(exe, "data", icon)
			if err != nil {
				logger.Log.WithError(err).Warn("Couldn't add the desktop icon")
			}
			w.IconsMade, w.IconDesktop = made.Made, made.OnDesktop
			w.HasIcon = !made.Made && launcher.HaveIcons(exe, "data")
		}
	}
	fmt.Print(launcher.Box(w, w.FromConsole))
}

func getVersion() string {
	data, err := embeddedFiles.ReadFile("VERSION")
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(data))
}
