package launcher

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The icon's jobs, as command-line flags on the program:
//
//	--open  start BudMinder in the background if it isn't running, then open the browser
//	--stop  stop the BudMinder started from this folder
const (
	OpenFlag = "--open"
	StopFlag = "--stop"
	// BackgroundFlag is --open's hidden helper on Windows: the copy of the
	// program it starts with no window, which runs BudMinder and opens the
	// browser once ready. Not for people to type.
	BackgroundFlag = "--background"
	// RestartedFlag marks the restart after an update: no browser to open,
	// the page is already waiting. Not for people to type.
	RestartedFlag = "--restarted"
)

const (
	markerFile = ".icons-made" // in the data folder: the program, then each icon made
	pngIcon    = "budminder-icon.png"
	icoIcon    = "budminder-icon.ico"
)

// Icons says what MakeIcons did.
type Icons struct {
	Made      bool // icons were made now
	OnDesktop bool // one of them is on the desktop
}

// MakeIcons adds a BudMinder icon to the desktop and the app menu (Linux) or
// Start menu (Windows), the first time BudMinder runs from a folder. Deleted
// icons stay deleted: a marker in the data folder records that they were made.
// Run from another folder after a move, the program makes them again, pointing
// at its new home. Other systems (and Docker) get none.
func MakeIcons(exe, dataDir string, png []byte) (Icons, error) {
	if InDocker() || (runtime.GOOS != "linux" && runtime.GOOS != "windows") {
		return Icons{}, nil
	}
	if made, _ := readMarker(dataDir); len(made) > 0 && made[0] == exe {
		return Icons{}, nil
	}
	var paths []string
	var err error
	var onDesktop bool
	if runtime.GOOS == "windows" {
		paths, onDesktop, err = makeWindowsIcons(exe, dataDir, png, "", "")
	} else {
		paths, onDesktop, err = makeLinuxIcons(exe, dataDir, png)
	}
	if err != nil || len(paths) == 0 {
		return Icons{}, err
	}
	return Icons{Made: true, OnDesktop: onDesktop}, writeMarker(dataDir, exe, paths)
}

// HaveIcons reports whether this program's icons from an earlier run are
// still there (the user may have deleted them).
func HaveIcons(exe, dataDir string) bool {
	made, err := readMarker(dataDir)
	if err != nil || len(made) < 2 || made[0] != exe {
		return false
	}
	for _, p := range made[1:] {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func readMarker(dataDir string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, markerFile))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

func writeMarker(dataDir, exe string, paths []string) error {
	return os.WriteFile(filepath.Join(dataDir, markerFile), []byte(exe+"\n"+strings.Join(paths, "\n")+"\n"), 0o644)
}

// ---------------------------------------------------------------- Linux

func makeLinuxIcons(exe, dataDir string, png []byte) (paths []string, onDesktop bool, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, false, err
	}
	iconPath, err := filepath.Abs(filepath.Join(dataDir, pngIcon))
	if err != nil {
		return nil, false, err
	}
	if err := os.WriteFile(iconPath, png, 0o644); err != nil {
		return nil, false, err
	}
	entry := DesktopEntry(exe, iconPath)

	// The app menu (Kubuntu's launcher, GNOME's app grid, ...).
	apps := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		return nil, false, err
	}
	menu := filepath.Join(apps, "budminder.desktop")
	if err := os.WriteFile(menu, []byte(entry), 0o755); err != nil {
		return nil, false, err
	}
	paths = append(paths, menu)
	// The desktop, when there is one. Plasma runs desktop icons only when
	// they're marked executable.
	if desk := desktopDir(home); desk != "" {
		p := filepath.Join(desk, "budminder.desktop")
		if err := os.WriteFile(p, []byte(entry), 0o755); err != nil {
			return nil, false, err
		}
		paths, onDesktop = append(paths, p), true
	}
	return paths, onDesktop, nil
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

// ---------------------------------------------------------------- Windows

// makeWindowsIcons makes BudMinder.lnk on the desktop and in the Start menu,
// plus "Stop BudMinder" in the Start menu (Windows shortcuts have no
// right-click actions of their own). Windows shortcuts are made through
// PowerShell's WScript.Shell, which every Windows has. desktop and programs
// override the folders (tests); "" asks Windows, which also follows a
// desktop moved into OneDrive.
func makeWindowsIcons(exe, dataDir string, png []byte, desktop, programs string) (paths []string, onDesktop bool, err error) {
	ico, err := filepath.Abs(filepath.Join(dataDir, icoIcon))
	if err != nil {
		return nil, false, err
	}
	if err := os.WriteFile(ico, PNGToICO(png), 0o644); err != nil {
		return nil, false, err
	}
	// Paths go in through the environment, so nothing in them needs quoting.
	script := `$ErrorActionPreference = 'Stop'
$sh = New-Object -ComObject WScript.Shell
$desk = $env:BM_DESKTOP; if (-not $desk) { $desk = [Environment]::GetFolderPath('Desktop') }
$prog = $env:BM_PROGRAMS; if (-not $prog) { $prog = [Environment]::GetFolderPath('Programs') }
function Make($where, $path, $arguments) {
  $s = $sh.CreateShortcut($path)
  $s.TargetPath = $env:BM_EXE
  $s.Arguments = $arguments
  $s.WorkingDirectory = Split-Path -Parent $env:BM_EXE
  $s.IconLocation = $env:BM_ICO
  $s.WindowStyle = 7
  $s.Description = 'BudMinder, a cannabis grow journal'
  $s.Save()
  Write-Output ($where + '|' + $path)
}
if ($prog -and (Test-Path $prog)) {
  Make 'menu' (Join-Path $prog 'BudMinder.lnk') '` + OpenFlag + `'
  Make 'menu' (Join-Path $prog 'Stop BudMinder.lnk') '` + StopFlag + `'
}
if ($desk -and (Test-Path $desk)) { Make 'desktop' (Join-Path $desk 'BudMinder.lnk') '` + OpenFlag + `' }
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(os.Environ(), "BM_EXE="+exe, "BM_ICO="+ico, "BM_DESKTOP="+desktop, "BM_PROGRAMS="+programs)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, false, err
	}
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		where, path, ok := strings.Cut(l, "|")
		if !ok {
			continue
		}
		paths = append(paths, path)
		onDesktop = onDesktop || where == "desktop"
	}
	return paths, onDesktop, nil
}

// PNGToICO wraps a PNG (256x256) in an .ico file, which Windows shortcuts
// need; Windows Vista and later read PNG-compressed icons.
func PNGToICO(png []byte) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&b, le, [3]uint16{0, 1, 1})                  // reserved, type 1 = icon, one image
	b.Write([]byte{0, 0, 0, 0})                                   // 256x256 (0 means 256), no palette, reserved
	_ = binary.Write(&b, le, [2]uint16{1, 32})                    // planes, bits per pixel
	_ = binary.Write(&b, le, [2]uint32{uint32(len(png)), 6 + 16}) // size, offset of the image
	b.Write(png)
	return b.Bytes()
}
