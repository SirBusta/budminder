//go:build windows

package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

// Start with Windows makes and removes a Startup shortcut (here in a scratch
// folder, never the real Startup folder).
func TestStartupShortcut_OnOff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := filepath.Join(t.TempDir(), "budminder.exe")
	a := startupShortcut(exe, dir)
	if a.On() {
		t.Fatal("off to begin with")
	}
	if err := a.Set(true); err != nil {
		t.Fatal(err)
	}
	if !a.On() {
		t.Fatal("on after Set(true)")
	}
	lnk, err := os.ReadFile(filepath.Join(dir, "BudMinder.lnk"))
	if err != nil {
		t.Fatal(err)
	}
	// .lnk files keep their arguments as UTF-16.
	if !strings.Contains(string(lnk), strings.Join(strings.Split(AtLoginFlag, ""), "\x00")) {
		t.Error("the shortcut doesn't pass " + AtLoginFlag)
	}
	if err := a.Set(false); err != nil {
		t.Fatal(err)
	}
	if a.On() {
		t.Fatal("off after Set(false)")
	}
	if err := a.Set(false); err != nil {
		t.Errorf("turning it off twice: %v", err)
	}
}

// After the folder is moved, the shortcut still starts the old path; Fix
// points it at the program that's running now.
func TestStartupShortcut_FixAfterMove(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	oldExe := filepath.Join(t.TempDir(), "budminder.exe")
	newExe := filepath.Join(t.TempDir(), "budminder.exe")
	if err := startupShortcut(oldExe, dir).Set(true); err != nil {
		t.Fatal(err)
	}
	a := startupShortcut(newExe, dir)
	if err := a.Fix(); err != nil {
		t.Fatal(err)
	}
	got, err := shortcutTarget(filepath.Join(dir, "BudMinder.lnk"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got, newExe) {
		t.Errorf("shortcut starts %q, want %q", got, newExe)
	}
	// Off stays off.
	if err := a.Set(false); err != nil {
		t.Fatal(err)
	}
	if err := a.Fix(); err != nil || a.On() {
		t.Errorf("Fix turned it on (err %v)", err)
	}
}

// shellExecuteInfo must match Windows' SHELLEXECUTEINFOW, or the elevated
// start reads garbage.
func TestShellExecuteInfoSize(t *testing.T) {
	t.Parallel()
	want := map[uintptr]uintptr{8: 112, 4: 60}[unsafe.Sizeof(uintptr(0))]
	if got := unsafe.Sizeof(shellExecuteInfo{}); got != want {
		t.Errorf("SHELLEXECUTEINFOW is %d bytes here, Windows expects %d", got, want)
	}
}
