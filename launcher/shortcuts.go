package launcher

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// The icon's two jobs, as command-line flags on the program:
//
//	--open  start BudMinder in the background if it isn't running, then open the browser
//	--stop  stop the BudMinder started from this folder
const (
	OpenFlag = "--open"
	StopFlag = "--stop"
)

const (
	pidFile    = "budminder.pid" // in the data folder, while BudMinder runs
	markerFile = ".icons-made"   // in the data folder, once the icons were made
	iconFile   = "budminder-icon.png"
)

// MakeIcons adds a BudMinder icon to the desktop and the app menu, the first
// time BudMinder runs from a folder (Linux only). Deleted icons stay deleted:
// a marker in the data folder records that they were made. Run from another
// folder after a move, the program makes them again, pointing at its new home.
// Returns whether icons were made now.
func MakeIcons(exe, dataDir string, icon []byte) (bool, error) {
	if runtime.GOOS != "linux" || InDocker() {
		return false, nil
	}
	marker := filepath.Join(dataDir, markerFile)
	if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == exe {
		return false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	iconPath, err := filepath.Abs(filepath.Join(dataDir, iconFile))
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(iconPath, icon, 0o644); err != nil {
		return false, err
	}
	entry := DesktopEntry(exe, iconPath)

	// The app menu (Kubuntu's launcher, GNOME's app grid, ...).
	apps := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(apps, "budminder.desktop"), []byte(entry), 0o755); err != nil {
		return false, err
	}
	// The desktop, when there is one. Plasma runs desktop icons only when
	// they're marked executable.
	if desk := desktopDir(home); desk != "" {
		if err := os.WriteFile(filepath.Join(desk, "budminder.desktop"), []byte(entry), 0o755); err != nil {
			return false, err
		}
	}
	return true, os.WriteFile(marker, []byte(exe+"\n"), 0o644)
}

// DesktopEntry is the icon's .desktop file: double-click opens BudMinder
// (starting it first if needed); its right-click menu can stop it.
func DesktopEntry(exe, icon string) string {
	q := quoteExec(exe)
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=BudMinder\n" +
		"Comment=Cannabis grow journal\n" +
		"Exec=" + q + " " + OpenFlag + "\n" +
		"Icon=" + icon + "\n" +
		"Terminal=false\n" +
		"Categories=Utility;\n" +
		"StartupNotify=false\n" +
		"Actions=stop;\n" +
		"\n" +
		"[Desktop Action stop]\n" +
		"Name=Stop BudMinder\n" +
		"Exec=" + q + " " + StopFlag + "\n"
}

// quoteExec quotes a path for a .desktop Exec line (Desktop Entry spec:
// double quotes, with ", `, $ and \ escaped).
func quoteExec(p string) string {
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", "$", `\\$`)
	return `"` + r.Replace(p) + `"`
}

// desktopDir is the desktop folder (xdg-user-dir knows translated names such
// as ~/Schreibtisch), or "" when there's none.
func desktopDir(home string) string {
	if out, err := exec.Command("xdg-user-dir", "DESKTOP").Output(); err == nil {
		d := strings.TrimSpace(string(out))
		if d != "" && d != home {
			if st, err := os.Stat(d); err == nil && st.IsDir() {
				return d
			}
		}
	}
	d := filepath.Join(home, "Desktop")
	if st, err := os.Stat(d); err == nil && st.IsDir() {
		return d
	}
	return ""
}

// WritePID records this process in the data folder, for --stop.
func WritePID(dataDir string) {
	_ = os.WriteFile(filepath.Join(dataDir, pidFile), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// RemovePID removes the record on the way out.
func RemovePID(dataDir string) {
	_ = os.Remove(filepath.Join(dataDir, pidFile))
}

// Stop asks the BudMinder started from this folder to shut down, the same
// way Ctrl+C does.
func Stop(dataDir string) error {
	b, err := os.ReadFile(filepath.Join(dataDir, pidFile))
	if err != nil {
		return errors.New("BudMinder isn't running from this folder")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return fmt.Errorf("unreadable %s", pidFile)
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		RemovePID(dataDir) // left over from a run that didn't exit cleanly
		return errors.New("BudMinder isn't running from this folder")
	}
	return nil
}
