// Package workdir picks the folder BudMinder runs in. Its data, uploads and
// logs folders are relative paths, so without this they'd be made in whatever
// folder the program was started from: a terminal elsewhere, a desktop
// launcher, or a double-click in macOS Finder (which starts programs in the
// home folder) would show a new, empty BudMinder. Running in the program's
// own folder keeps everything next to it, however it's started.
package workdir

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"isley/env"
)

// Choice is where BudMinder runs and why.
type Choice struct {
	Dir    string // the folder to run in
	Moved  bool   // Dir is the program's folder, not the one it was started in
	Reason string // why it stayed, for the log ("" when moved or nothing to do)
}

// Choose decides between the program's folder (exeDir) and the folder it was
// started in (cwd). It stays put when:
//   - BUDMINDER_DB_FILE is set: whoever set it manages the paths themselves;
//   - the program is a `go run` / `go test` build in a temporary folder;
//   - it was started in its own folder already (Docker, Windows double-click);
//   - the folder it was started in holds a database and the program's folder
//     doesn't: an install that has always run from there keeps its data.
func Choose(exeDir, cwd string, dbFileSet bool, exists func(string) bool) Choice {
	stay := func(reason string) Choice { return Choice{Dir: cwd, Reason: reason} }
	switch {
	case dbFileSet:
		return stay("BUDMINDER_DB_FILE is set")
	case strings.Contains(filepath.ToSlash(exeDir), "/go-build"):
		return stay("development build")
	case sameDir(exeDir, cwd):
		return stay("")
	case exists(filepath.Join(cwd, "data", "isley.db")) && !exists(filepath.Join(exeDir, "data", "isley.db")):
		return stay("its database is already here")
	}
	return Choice{Dir: exeDir, Moved: true}
}

func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Apply makes the choice and moves into the chosen folder. Call it first in
// main, before anything opens a relative path (the log file included).
func Apply() (Choice, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Choice{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return Choice{Dir: cwd}, err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real // a link in ~/bin still means the program's real folder
	}
	_, dbFileSet := env.Lookup("DB_FILE")
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	c := Choose(filepath.Dir(exe), cwd, dbFileSet, exists)
	if c.Moved {
		if err := os.Chdir(c.Dir); err != nil {
			return Choice{Dir: cwd}, err
		}
	}
	return c, nil
}
