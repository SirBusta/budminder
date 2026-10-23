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

func TestMakeIcons(t *testing.T) {
	if runtime.GOOS != "linux" || InDocker() {
		t.Skip("desktop icons are made on Linux, outside Docker")
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

	made, onDesktop, err := MakeIcons(exe, data, []byte("png"))
	if err != nil || !made || !onDesktop {
		t.Fatalf("first run: made=%v onDesktop=%v err=%v", made, onDesktop, err)
	}
	for _, p := range []string{filepath.Join(desk, "budminder.desktop"), filepath.Join(home, ".local", "share", "applications", "budminder.desktop")} {
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
		t.Error("HaveIcons: the icon from this run isn't found")
	}

	// Deleted icons stay deleted...
	if err := os.Remove(filepath.Join(desk, "budminder.desktop")); err != nil {
		t.Fatal(err)
	}
	if made, _, _ := MakeIcons(exe, data, []byte("png")); made {
		t.Error("icons made again after the user deleted them")
	}
	if err := os.Remove(filepath.Join(home, ".local", "share", "applications", "budminder.desktop")); err != nil {
		t.Fatal(err)
	}
	if HaveIcons(exe, data) {
		t.Error("HaveIcons: the app-menu icon was deleted, so there's none to point at")
	}
	// ...until the program moves, when they're made to point at the new place.
	if made, _, _ := MakeIcons("/home/dave/budminder/budminder-linux-amd64", data, []byte("png")); !made {
		t.Error("icons not remade after the program moved")
	}
}
