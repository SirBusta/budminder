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
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"isley/app"
	"isley/config"
	"isley/demo"
	"isley/env"
	"isley/handlers"
	"isley/homeassistant"
	"isley/launcher"
	"isley/logger"
	"isley/model"
	"isley/updates"
	"isley/utils"
	"isley/watcher"
	"isley/workdir"
)

//go:embed model/migrations/sqlite/*.sql model/migrations/postgres/*.sql web/templates/**/*.html web/static/**/* web/manual/* web/manual/en/* utils/fonts/* VERSION LICENSE
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
	// The Windows download has no console of its own: from a terminal it
	// borrows that one; double-clicked, it talks through cards (launcher/card.go).
	launcher.AttachParentConsole()
	cardIcon, _ := embeddedFiles.ReadFile("web/static/img/brand/budminder-256.png")
	cardGlyphs, _ := embeddedFiles.ReadFile("web/static/vendor/fontawesome/webfonts/fa-solid-900.ttf")
	launcher.SetLook(cardIcon, cardGlyphs)

	// Run in the program's own folder, so data, uploads and logs sit next to
	// it however it was started (workdir explains when it stays put).
	where, whereErr := workdir.Apply()
	firstRunHome()
	logger.InitLogger()
	here, _ := os.Getwd()
	if launcher.Windowless() {
		// A start that fails says why in a card, not only in the log.
		var last string
		logger.Log.AddHook(fatalHook{&last})
		logger.Log.ExitFunc = func(code int) {
			launcher.RunCard(launcher.StartFailedCard(last), problemEvents(last, here))
			os.Exit(code)
		}
	}
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
		port = launcher.SavedPort("data") // chosen in the "port taken" card
	}
	if port == "" {
		port = "8080"
	}

	// The desktop icon's jobs (launcher.MakeIcons): --stop stops the BudMinder
	// running from this folder; --open opens the browser, starting BudMinder
	// first when it isn't running yet. On Windows that start happens in a
	// second, windowless copy (--background), so no console stays open.
	// Double-clicking the .exe itself also opens the browser once ready.
	openWhenReady := runtime.GOOS == "windows" && len(os.Args) == 1 && (launcher.IsTerminal(os.Stdout) || launcher.Windowless())
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
		case launcher.BackgroundFlag, launcher.InstalledFromFlag, launcher.UpdatedFromFlag:
			openWhenReady = true
		case launcher.RestartedFlag:
			// back after an update: the browser page is already open and waiting
		case launcher.AllowFirewallFlag:
			// The phone window's "Allow through firewall", run as administrator.
			if err := launcher.AllowFirewall(exePath()); err != nil {
				logger.Log.WithError(err).Error("Couldn't let BudMinder through the firewall")
				os.Exit(1)
			}
			logger.Log.Info("BudMinder is allowed through the firewall on private networks")
			return
		case launcher.AtLoginFlag:
			// "Start with Windows": the Startup shortcut runs this in a minimized
			// window, which hands over to a windowless copy. No browser.
			if launcher.Running(port) {
				return
			}
			if runtime.GOOS == "windows" && launcher.IsTerminal(os.Stdout) {
				if err := launcher.StartHidden(exePath(), launcher.AtLoginFlag); err == nil {
					return
				}
			}
		}
	}

	if launcher.Windowless() && env.Get("DEMO") == "" {
		var ok bool
		if port, ok = portForThisStart(port, here, openWhenReady); !ok {
			return
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

	// New releases: checked at start and daily (Settings can turn it off),
	// shown as "New release" in the top menu. The downloadable programs can
	// install one and restart; Docker installs are only told. Not the demo.
	var releases *handlers.Updates
	restartCh := make(chan struct{}, 1)
	if !demoMode {
		checker := updates.New(getVersion(), env.Get("UPDATE_URL"), func() bool { // UPDATE_URL: a stand-in release list, for testing
			v, _ := handlers.GetSetting(db, handlers.UpdateCheckDisabledSetting)
			return v == "1"
		})
		releases = &handlers.Updates{Checker: checker, DataDir: "data"}
		if !launcher.InDocker() {
			releases.Restart = func() {
				select {
				case restartCh <- struct{}{}:
				default:
				}
			}
			updates.RemoveLeftovers(exePath()) // a download an interrupted update left
		}
		bgWG.Add(1)
		go func() {
			defer bgWG.Done()
			checker.Run(ctx)
		}()
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
		Updates:               releases,
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
	restarting := false
	var stopRequested <-chan struct{} // "Stop BudMinder" (--stop); nil blocks forever
	trayStop := make(chan struct{})   // Stop BudMinder in the tray icon's menu
	removeTray := func() {}
	if !demoMode && !launcher.InDocker() {
		if ctl, err := launcher.StartControl("data", getVersion()); err != nil {
			logger.Log.WithError(err).Warn("Stop BudMinder won't work this time")
		} else {
			defer ctl.Close()
			stopRequested = ctl.Stop
		}
		// Only a normal run is an install: a set database file or a go run /
		// go test build (tests, the demo build, trying things out) never
		// writes the note or makes icons.
		_, dbFileSet := env.Lookup("DB_FILE")
		install := !dbFileSet && where.Reason != "development build"
		elsewhere, other := "", false
		if install {
			// A second copy leaves the note and the icons to the noted install.
			note, ok := launcher.ReadNote()
			if elsewhere, other = launcher.OtherInstall(exePath(), note, ok, fileExists); !other {
				writeNote()
			}
		}
		// The icon by the clock (Windows): only for a real install, or a test
		// run that asks with BUDMINDER_TRAY=1. Start with Windows only for the
		// noted install, never a second copy.
		showTray := false
		if install || env.Get("TRAY") == "1" {
			var atLogin *launcher.AtLogin
			if install && !other {
				atLogin = launcher.StartWithWindows(exePath())
			}
			icon, _ := embeddedFiles.ReadFile("web/static/img/brand/budminder-256.png")
			glyphs, _ := embeddedFiles.ReadFile("web/static/vendor/fontawesome/webfonts/fa-solid-900.ttf")
			var once sync.Once
			removeTray, showTray = launcher.RunTray(launcher.Tray{
				Version: getVersion(),
				Port:    port,
				Folder:  here,
				Icon:    icon,
				Glyphs:  glyphs,
				Exe:     exePath(),
				Update: func() string {
					if r := releases.Checker.Available(); r != nil {
						return r.Version
					}
					return ""
				},
				AtLogin: atLogin,
				Stop:    func() { once.Do(func() { close(trayStop) }) },
				Failed: func(what string, err error) {
					logger.Log.WithError(err).Warn("Tray icon: couldn't " + what)
				},
			})
		}
		time.Sleep(1500 * time.Millisecond) // let the startup jobs log first, so the box ends up last
		w := welcome(db, port, elsewhere, install, showTray)
		if openWhenReady {
			if err := launcher.OpenBrowser("http://localhost:" + port); err != nil {
				logger.Log.WithError(err).Error("Couldn't open the browser")
			}
		}
		// The welcome card: double-clicked, set up from Downloads, or updated
		// from a download. Never from the icon or Start with Windows.
		if launcher.Windowless() && (len(os.Args) == 1 || w.Installed != "" || w.Updated != "") {
			go showWelcome(w, showTray)
		}
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-stopRequested:
	case <-trayStop:
	case <-restartCh:
		restarting = true
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
	removeTray()
	if restarting {
		relaunch()
	}
}

// relaunch starts the program again after an update swapped it, the same
// way it was running: hidden on Windows when started from the icon, in the
// same terminal otherwise. Under systemd (the README's service) it exits
// instead, and Restart=on-failure starts the new program.
func relaunch() {
	if os.Getenv("INVOCATION_ID") != "" {
		logger.Log.Info("Exiting so systemd starts the new version")
		os.Exit(75)
	}
	exe := exePath()
	var err error
	if runtime.GOOS == "windows" && !launcher.IsTerminal(os.Stdout) {
		err = launcher.StartHidden(exe, launcher.RestartedFlag)
	} else {
		cmd := exec.Command(exe, launcher.RestartedFlag)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		err = cmd.Start()
	}
	if err != nil {
		logger.Log.WithError(err).Error("Couldn't start the new version; start BudMinder again yourself")
	}
}

// firstRunHome: a download double-clicked where it has no data of its own.
// When the note (launcher.NotePath) names an install elsewhere, a newer
// download offers to update it and an older or the same one opens it.
// Otherwise a program started from Downloads or a synced folder asks for a proper
// folder, copies itself into it and runs from there (launcher.ChooseHome).
// Cancel carries on here, with a warning.
func firstRunHome() {
	exe := exePath()
	_, dbFileSet := env.Lookup("DB_FILE")
	if exe == "" || len(os.Args) > 1 || dbFileSet || env.Get("DEMO") != "" || launcher.InDocker() ||
		strings.Contains(filepath.ToSlash(exe), "/go-build") {
		return
	}
	here, _ := os.Getwd()
	note, ok := launcher.ReadNote()
	windowless := launcher.Windowless()
	switch launcher.Decide(exe, here, getVersion(), note, ok, updates.Newer, fileExists) {
	case launcher.PlanUpdate:
		dir := filepath.Dir(note.Exe)
		var yes bool
		if windowless {
			yes = launcher.RunCard(launcher.UpdateCard(dir, note.Version, getVersion()), launcher.CardEvents{}) == launcher.CardUpdate
		} else {
			yes = launcher.ConfirmUpdate(dir, note.Version, getVersion())
		}
		if yes {
			fmt.Println("Updating BudMinder in", dir, "...")
			if err := launcher.UpdateInstall(exe, note, getVersion(), backupDB); err != nil {
				fmt.Println("The update didn't work, and your BudMinder is unchanged:", err)
				if windowless {
					launcher.RunCard(launcher.UpdateFailedCard(dir, err.Error()), problemEvents(err.Error(), dir))
				}
				waitForEnter()
				os.Exit(1)
			}
			os.Exit(launcher.RunUpdated(note.Exe, exe))
		}
		fmt.Println("Nothing was changed. Start BudMinder with its icon as usual.")
		os.Exit(0)
	case launcher.PlanOpen:
		fmt.Println("BudMinder", note.Version, "is installed in", filepath.Dir(note.Exe), "- opening that one.")
		if err := launcher.OpenInstalled(note.Exe); err == nil {
			os.Exit(0)
		}
	}
	dir := filepath.Dir(exe)
	risky := launcher.RiskyFolder(dir)
	if risky == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "isley.db")); err == nil {
		return // already running from there: the welcome box warns instead
	}
	for {
		chosen := launcher.ChooseHome(risky)
		if chosen == "" {
			return
		}
		installed, err := launcher.Install(exe, chosen)
		if err == nil {
			os.Exit(launcher.RunInstalled(installed, exe))
		}
		fmt.Println("Couldn't set BudMinder up there:", err)
		if !windowless || launcher.RunCard(launcher.SetupFailedCard(chosen, err.Error()), problemEvents(err.Error(), "")) != launcher.CardChooseFolder {
			return // carries on where it is, with the warning
		}
	}
}

// backupDB copies the SQLite database at src into one complete file at dst
// (VACUUM INTO, which also takes in its -wal journal).
func backupDB(src, dst string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(src))
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("VACUUM INTO ?", dst)
	return err
}

// argValue is the value after flag on the command line ("" when absent).
func argValue(flag string) string {
	for i, a := range os.Args {
		if a == flag && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

// waitForEnter keeps a double-clicked window open long enough to read it.
func waitForEnter() {
	if launcher.IsTerminal(os.Stdin) {
		fmt.Print("Press Enter to close this window.")
		_, _ = fmt.Scanln()
	}
}

// writeNote records this program in the note, so a later download finds it:
// only a real install, with its database next to it and not in Downloads
// or a synced folder.
func writeNote() {
	exe := exePath()
	if exe == "" || launcher.RiskyFolder(filepath.Dir(exe)) != "" {
		return
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "data", "isley.db")); err != nil {
		return
	}
	if err := launcher.WriteNote(exe, getVersion()); err != nil {
		logger.Log.WithError(err).Warn("Couldn't write the install note")
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

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
// prints the welcome box: where to open BudMinder, that the first page creates
// the account, and a warning when the program sits in Downloads or a synced folder.
func welcome(db *sql.DB, port, elsewhere string, install, tray bool) launcher.Welcome {
	here, _ := os.Getwd()
	w := launcher.Welcome{Folder: here, Port: port, Elsewhere: elsewhere, LANAddress: launcher.LANAddress(), FromConsole: launcher.IsTerminal(os.Stdout), OS: runtime.GOOS, Installed: argValue(launcher.InstalledFromFlag), Updated: argValue(launcher.UpdatedFromFlag), Tray: tray}
	w.Setup = handlers.NeedsSetup(db)
	if exe := exePath(); exe != "" {
		// No icons for a program in Downloads or a synced folder: they'd point at a
		// file that's about to be moved.
		if w.Risky = launcher.RiskyFolder(filepath.Dir(exe)); w.Risky != "" {
			w.Suggest = launcher.SuggestedHome()
		} else if elsewhere != "" || !install {
			// a second copy or a test run: the icons stay with the install they open
		} else if icon, err := embeddedFiles.ReadFile("web/static/img/brand/budminder-256.png"); err == nil {
			made, err := launcher.MakeIcons(exe, "data", icon)
			if err != nil {
				logger.Log.WithError(err).Warn("Couldn't add the desktop icon")
			}
			w.IconsMade, w.IconDesktop = made.Made, made.OnDesktop
			w.HasIcon = !made.Made && launcher.HaveIcons(exe, "data")
		}
	}
	if !launcher.Windowless() {
		fmt.Print(launcher.Box(w, w.FromConsole && launcher.EnableColor()))
	}
	return w
}

// showWelcome shows the welcome card (Windows download). Closing it, or Got
// it, sends it into the tray icon; the first time, a notification says where
// BudMinder went.
func showWelcome(w launcher.Welcome, tray bool) {
	icon, _ := embeddedFiles.ReadFile("web/static/img/brand/budminder-256.png")
	glyphs, _ := embeddedFiles.ReadFile("web/static/vendor/fontawesome/webfonts/fa-solid-900.ttf")
	small, _ := launcher.IconPNG(icon, 32)
	toTray := func(cw *launcher.CardWindow) {
		if !tray {
			cw.Close(launcher.CardGotIt)
			return
		}
		// The notification once per install: the marker file remembers it.
		hint := filepath.Join("data", ".tray-hint-shown")
		first := !fileExists(hint)
		if first {
			_ = os.WriteFile(hint, nil, 0o644)
		}
		launcher.SendToTray(cw, first)
	}
	launcher.RunWelcome(launcher.WelcomeCard(w, getVersion()), launcher.CardEvents{
		Button: func(cw *launcher.CardWindow, id int) bool {
			switch id {
			case launcher.CardPhone:
				launcher.ShowPhoneCode(launcher.PhoneCode{Address: launcher.Tray{Port: w.Port}.HomeNetworkURL(), Icon: small, Glyphs: glyphs, Exe: exePath()})
			case launcher.CardGotIt:
				toTray(cw)
			}
			return false
		},
		Check: func(_ *launcher.CardWindow, id int, on bool) {
			if id != launcher.CardDesktop {
				return
			}
			go func() {
				if err := launcher.SetDesktopIcon(exePath(), "data", icon, on); err != nil {
					logger.Log.WithError(err).Warn("Couldn't change the desktop icon")
				}
			}()
		},
		Close: toTray,
	})
}

// portForThisStart checks, before anything else starts, that BudMinder can
// have its port (Windows download, which has no console to explain in).
// The same version already running from this folder is simply opened; an
// older one can be stopped; another program can be dodged with a free port,
// remembered for next time. ok false: don't start.
func portForThisStart(port, folder string, open bool) (string, bool) {
	for {
		err := launcher.PortFree(port)
		if err == nil {
			return port, true
		}
		if v, running := launcher.RunningHere("data"); running {
			if v == getVersion() {
				if open {
					_ = launcher.OpenBrowser("http://localhost:" + port)
				}
				return port, false
			}
			older := v == "" || updates.Newer(getVersion(), v)
			if launcher.RunCard(launcher.OtherRunningCard(v, port, older), launcher.CardEvents{}) != launcher.CardStopOther {
				return port, false
			}
			if launcher.Stop("data") == nil {
				launcher.WaitStopped("data")
			}
			continue
		}
		alt := launcher.FreePortAfter(port)
		if launcher.RunCard(launcher.PortTakenCard(port, alt, err.Error()), problemEvents(err.Error(), folder)) != launcher.CardOtherPort || alt == "" {
			return port, false
		}
		if err := launcher.SavePort("data", alt); err != nil {
			fmt.Println("Couldn't save the port:", err)
		}
		port = alt
	}
}

// problemEvents: Copy details and Open BudMinder folder keep the card open.
func problemEvents(details, folder string) launcher.CardEvents {
	return launcher.CardEvents{Button: func(cw *launcher.CardWindow, id int) bool {
		switch id {
		case launcher.CardCopy:
			if launcher.CopyText(details) == nil {
				cw.SetLabel(id, "Copied")
			}
			return false
		case launcher.CardFolder:
			_ = launcher.OpenFolder(folder)
			return false
		case launcher.CardHelp:
			_ = launcher.OpenBrowser(launcher.HelpURL)
			return false
		}
		return true
	}}
}

// fatalHook keeps the last fatal message for the "couldn't start" card.
type fatalHook struct{ last *string }

func (fatalHook) Levels() []logrus.Level { return []logrus.Level{logrus.FatalLevel} }

func (h fatalHook) Fire(e *logrus.Entry) error {
	s := e.Message
	if err, ok := e.Data[logrus.ErrorKey]; ok {
		s += ": " + fmt.Sprint(err)
	}
	*h.last = s
	return nil
}

func getVersion() string {
	data, err := embeddedFiles.ReadFile("VERSION")
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(data))
}
