package launcher

// +parallel:serial — t.Setenv points HOME at a temporary folder, which changes
// the process environment.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMakeIcons_Linux(t *testing.T) {
	if runtime.GOOS != "linux" || InDocker() {
		t.Skip("Linux icons are made on Linux, outside Docker")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config")) // no real xdg-user-dirs
	desk := filepath.Join(home, "Desktop")
	if err := os.Mkdir(desk, 0o755); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(data, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := "/opt/budminder/budminder-linux-amd64"
	menu := filepath.Join(home, ".local", "share", "applications", "budminder.desktop")

	got, err := MakeIcons(exe, data, []byte("png"))
	if err != nil || !got.Made || !got.OnDesktop {
		t.Fatalf("first run: %+v err=%v", got, err)
	}
	for _, p := range []string{filepath.Join(desk, "budminder.desktop"), menu} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
		if st.Mode()&0o100 == 0 {
			t.Errorf("%s isn't executable, so Plasma won't run it", p)
		}
		b, _ := os.ReadFile(p)
		if !strings.Contains(string(b), exe) {
			t.Errorf("%s doesn't point at the program", p)
		}
	}
	if !HaveIcons(exe, data) {
		t.Error("HaveIcons: the icons from this run aren't found")
	}

	// Deleted icons stay deleted...
	if err := os.Remove(filepath.Join(desk, "budminder.desktop")); err != nil {
		t.Fatal(err)
	}
	if again, _ := MakeIcons(exe, data, []byte("png")); again.Made {
		t.Error("icons made again after the user deleted them")
	}
	if err := os.Remove(menu); err != nil {
		t.Fatal(err)
	}
	if HaveIcons(exe, data) {
		t.Error("HaveIcons: all icons were deleted, so there's none to point at")
	}
	// ...until the program moves, when they're made to point at the new place.
	if moved, _ := MakeIcons("/home/dave/budminder/budminder-linux-amd64", data, []byte("png")); !moved.Made {
		t.Error("icons not remade after the program moved")
	}
}

// Windows shortcuts, made in temporary folders instead of the real desktop
// and Start menu.
func TestMakeIcons_Windows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("Windows shortcuts are made on Windows")
	}
	desk, prog, data := t.TempDir(), t.TempDir(), t.TempDir()
	exe := `C:\BudMinder Test\budminder-windows-amd64.exe`
	paths, onDesktop, err := makeWindowsIcons(exe, data, []byte("png"), desk, prog, "")
	if err != nil {
		t.Fatal(err)
	}
	if !onDesktop || len(paths) != 3 {
		t.Fatalf("want 3 shortcuts with one on the desktop, got %v (desktop %v)", paths, onDesktop)
	}
	for _, p := range []string{filepath.Join(desk, "BudMinder.lnk"), filepath.Join(prog, "BudMinder.lnk"), filepath.Join(prog, "Stop BudMinder.lnk")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(data, icoIcon)); err != nil {
		t.Error("no .ico for the shortcuts")
	}
}

func TestPNGToICO(t *testing.T) {
	t.Parallel()
	ico := PNGToICO([]byte("PNGDATA"))
	if len(ico) != 22+7 || ico[2] != 1 || ico[4] != 1 || ico[18] != 22 || string(ico[22:]) != "PNGDATA" {
		t.Errorf("bad icon header: % x", ico[:22])
	}
}

func TestNote_WriteAndRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", home)         // Windows
	t.Setenv("XDG_CONFIG_HOME", home) // Linux
	t.Setenv("HOME", home)            // macOS
	if _, ok := ReadNote(); ok {
		t.Fatal("no note yet")
	}
	exe := filepath.Join(home, "BudMinder", "budminder.exe")
	if err := WriteNote(exe, "1.5.3"); err != nil {
		t.Fatal(err)
	}
	n, ok := ReadNote()
	if !ok || n.Exe != exe || n.Version != "1.5.3" {
		t.Errorf("ReadNote = %+v, %v", n, ok)
	}
	p, _ := NotePath()
	if !strings.HasPrefix(p, home) {
		t.Errorf("note at %s, want under %s", p, home)
	}
}
