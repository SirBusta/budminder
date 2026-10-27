//go:build windows

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// StartWithWindows is the tray's "Start with Windows" for this program: a
// BudMinder shortcut in the Startup folder, which Windows runs when you sign
// in. It starts BudMinder in the background (AtLoginFlag), without opening the
// browser. The user can also see and turn it off in Task Manager > Startup.
func StartWithWindows(exe string) *AtLogin {
	return startupShortcut(exe, "")
}

// startupShortcut: dir overrides the Startup folder (tests); "" asks Windows.
func startupShortcut(exe, dir string) *AtLogin {
	path := func() string {
		d := dir
		if d == "" {
			d, _ = windows.KnownFolderPath(windows.FOLDERID_Startup, 0)
		}
		if d == "" {
			return ""
		}
		return filepath.Join(d, "BudMinder.lnk")
	}
	a := &AtLogin{
		On: func() bool {
			p := path()
			_, err := os.Stat(p)
			return p != "" && err == nil
		},
	}
	a.Set = func(on bool) error {
		p := path()
		if p == "" {
			return os.ErrNotExist
		}
		if !on {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		ico, _ := filepath.Abs(filepath.Join(filepath.Dir(exe), "data", icoIcon)) // made with the desktop icon
		script := `$ErrorActionPreference = 'Stop'
$s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:BM_LNK)
$s.TargetPath = $env:BM_EXE
$s.Arguments = '` + AtLoginFlag + `'
$s.WorkingDirectory = Split-Path -Parent $env:BM_EXE
if (Test-Path $env:BM_ICO) { $s.IconLocation = $env:BM_ICO }
$s.WindowStyle = 7
$s.Description = 'Starts BudMinder, a cannabis grow journal, in the background'
$s.Save()
`
		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
		cmd.Env = append(os.Environ(), "BM_EXE="+exe, "BM_LNK="+p, "BM_ICO="+ico)
		hideWindow(cmd)
		return cmd.Run()
	}
	// The shortcut names the program by its full path, so after the folder
	// is moved it starts nothing (or an old copy) while the tray still shows
	// Start with Windows ticked: point it at this program again.
	a.Fix = func() error {
		if !a.On() {
			return nil
		}
		target, err := shortcutTarget(path())
		if err != nil {
			return err
		}
		if strings.EqualFold(filepath.Clean(target), filepath.Clean(exe)) {
			return nil
		}
		return a.Set(true)
	}
	return a
}

// shortcutTarget is the program a .lnk file starts.
func shortcutTarget(lnk string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command",
		`(New-Object -ComObject WScript.Shell).CreateShortcut($env:BM_LNK).TargetPath`)
	cmd.Env = append(os.Environ(), "BM_LNK="+lnk)
	hideWindow(cmd)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
