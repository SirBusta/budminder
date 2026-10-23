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
		filepath.Join("home", "dave", "Downloads"):                      "Downloads",
		filepath.Join("C:", "Users", "dave", "downloads", "x"):          "Downloads",
		filepath.Join("C:", "Users", "dave", "OneDrive", "Documents"):   "OneDrive",
		filepath.Join("C:", "Users", "dave", "OneDrive - Acme", "Apps"): "OneDrive",
		filepath.Join("C:", "Users", "dave", "BudMinder"):               "",
	} {
		if got := RiskyFolder(dir); got != want {
			t.Errorf("RiskyFolder(%s) = %q, want %q", dir, got, want)
		}
	}
}
