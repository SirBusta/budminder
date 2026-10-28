package launcher

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstall(t *testing.T) {
	t.Parallel()
	src := filepath.Join(t.TempDir(), "budminder-test.exe")
	if err := os.WriteFile(src, []byte("program"), 0o755); err != nil {
		t.Fatal(err)
	}
	name := "BudMinder"
	if runtime.GOOS == "linux" {
		name = "budminder"
	}

	// A folder is made inside the one picked...
	picked := t.TempDir()
	got, err := Install(src, picked)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(picked, name, "budminder-test.exe"); got != want {
		t.Errorf("copied to %s, want %s", got, want)
	}
	if b, _ := os.ReadFile(got); string(b) != "program" {
		t.Error("the copy isn't the program")
	}
	// ...unless the one picked already is BudMinder's folder.
	again, err := Install(src, filepath.Join(picked, name))
	if err != nil || again != got {
		t.Errorf("picking the BudMinder folder itself: %s, %v", again, err)
	}
	// Never into Downloads.
	if _, err := Install(src, filepath.Join(picked, "Downloads")); err == nil {
		t.Error("installed into a Downloads folder")
	}
	if _, err := Install(src, ""); err == nil {
		t.Error("installed with no folder chosen")
	}
}

func TestRiskyFolder(t *testing.T) {
	t.Parallel()
	for dir, want := range map[string]string{
		filepath.Join("home", "dave", "Downloads"):                                                        "Downloads",
		filepath.Join("C:", "Users", "dave", "downloads", "x"):                                            "Downloads",
		filepath.Join("C:", "Users", "dave", "OneDrive", "Documents"):                                     "OneDrive",
		filepath.Join("C:", "Users", "dave", "OneDrive - Acme", "Apps"):                                   "OneDrive",
		filepath.Join("C:", "Users", "dave", "BudMinder"):                                                 "",
		filepath.Join("C:", "Users", "dave", "Dropbox", "BudMinder"):                                      "Dropbox",
		filepath.Join("C:", "Users", "dave", "Dropbox (Acme)", "x"):                                       "Dropbox",
		filepath.Join("G:"+string(filepath.Separator), "My Drive", "BudMinder"):                           "Google Drive",
		filepath.Join("C:", "Users", "dave", "iCloudDrive", "x"):                                          "iCloud Drive",
		filepath.Join("Users", "dave", "Library", "Mobile Documents", "com~apple~CloudDocs", "BudMinder"): "iCloud Drive",
		filepath.Join("Users", "dave", "Library", "CloudStorage", "GoogleDrive-you@example.com", "x"):     "Google Drive",
		filepath.Join("Users", "dave", "Library", "CloudStorage", "Box-Box", "x"):                         "Box",
		filepath.Join("C:", "grow", "box", "BudMinder"):                                                   "", // a grow box, not Box Drive
	} {
		if got := RiskyFolder(dir); got != want {
			t.Errorf("RiskyFolder(%s) = %q, want %q", dir, got, want)
		}
	}
}

// TestTypedFolder: a relative folder typed at the prompt lands under the
// home folder, not the folder the program was started from.
func TestTypedFolder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	abs := filepath.Join(t.TempDir(), "elsewhere")
	for in, want := range map[string]string{
		"":          home,
		"cancel":    "",
		"~/apps":    filepath.Join(home, "apps"),
		"apps/grow": filepath.Join(home, "apps", "grow"),
		abs:         abs,
	} {
		if got := typedFolder(in+"\n", home); got != want {
			t.Errorf("typedFolder(%q) = %q, want %q", in, got, want)
		}
	}
}
