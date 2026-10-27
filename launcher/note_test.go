package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newerStub: plain dotted versions, enough for these cases.
func newerStub(a, b string) bool { return a > b }

func TestDecide(t *testing.T) {
	t.Parallel()
	inst := filepath.Join("home", "BudMinder", "budminder.exe")
	dl := filepath.Join("home", "Downloads", "budminder.exe")
	files := map[string]bool{
		inst: true,
		filepath.Join("home", "BudMinder", "data", "isley.db"): true,
	}
	exists := func(p string) bool { return files[p] }
	note := Note{Exe: inst, Version: "1.5.2"}
	cases := []struct {
		name, exe, here, version string
		note                     Note
		ok                       bool
		want                     Plan
	}{
		{"no note", dl, filepath.Dir(dl), "1.5.3", Note{}, false, PlanNone},
		{"newer download", dl, filepath.Dir(dl), "1.5.3", note, true, PlanUpdate},
		{"same version", dl, filepath.Dir(dl), "1.5.2", note, true, PlanOpen},
		{"older download", dl, filepath.Dir(dl), "1.5.1", note, true, PlanOpen},
		{"the install itself", inst, filepath.Dir(inst), "1.5.3", note, true, PlanNone},
		{"install gone", dl, filepath.Dir(dl), "1.5.3", Note{Exe: filepath.Join("gone", "budminder.exe"), Version: "1.5.2"}, true, PlanNone},
	}
	for _, c := range cases {
		if got := Decide(c.exe, c.here, c.version, c.note, c.ok, newerStub, exists); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	// A download that has its own data is an install of its own.
	files[filepath.Join("home", "Downloads", "data", "isley.db")] = true
	if got := Decide(dl, filepath.Dir(dl), "1.5.3", note, true, newerStub, exists); got != PlanNone {
		t.Errorf("download with data: got %v, want none", got)
	}
}

func TestUpdateInstall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "budminder.exe")
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(exe, "old program")
	write(filepath.Join(data, "isley.db"), "the grow journal")
	download := filepath.Join(t.TempDir(), "budminder.exe")
	write(download, "new program")

	// The install is running: it's asked to stop and shuts down.
	ctl, err := StartControl(data, "1.5.8")
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() {
		<-ctl.Stop
		time.Sleep(200 * time.Millisecond)
		ctl.Close()
		close(stopped)
	}()

	if err := UpdateInstall(download, Note{Exe: exe, Version: "1.5.2"}, "1.5.3", copyFile); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Error("the running install wasn't stopped first")
	}
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	if got := read(exe); got != "new program" {
		t.Errorf("program = %q, want the new one", got)
	}
	if got := read(exe + ".old"); got != "old program" {
		t.Errorf("fallback = %q, want the old program", got)
	}
	backups, _ := filepath.Glob(filepath.Join(data, "backups", "before-1.5.3-*.db"))
	if len(backups) != 1 || read(backups[0]) != "the grow journal" {
		t.Errorf("backup = %v, want one copy of the database", backups)
	}
	if read(download) != "new program" {
		t.Error("the download should be left as it was")
	}
}

func TestNotePath(t *testing.T) {
	t.Parallel()
	p, err := NotePath()
	if err != nil {
		t.Skip("no settings folder here:", err)
	}
	if !strings.Contains(strings.ToLower(p), "budminder") {
		t.Errorf("NotePath = %q", p)
	}
}

func TestOtherInstall(t *testing.T) {
	t.Parallel()
	inst := filepath.Join("home", "BudMinder", "budminder.exe")
	files := map[string]bool{inst: true, filepath.Join("home", "BudMinder", "data", "isley.db"): true}
	exists := func(p string) bool { return files[p] }
	note := Note{Exe: inst, Version: "1.5.3"}
	if dir, ok := OtherInstall(filepath.Join("tmp", "test", "budminder.exe"), note, true, exists); !ok || dir != filepath.Dir(inst) {
		t.Errorf("a second copy: got %q, %v", dir, ok)
	}
	if _, ok := OtherInstall(inst, note, true, exists); ok {
		t.Error("the install itself isn't another install")
	}
	if _, ok := OtherInstall(filepath.Join("tmp", "test", "budminder.exe"), Note{Exe: filepath.Join("moved", "budminder.exe")}, true, exists); ok {
		t.Error("a noted install that's gone (moved) gives way")
	}
	if _, ok := OtherInstall(filepath.Join("tmp", "test", "budminder.exe"), Note{}, false, exists); ok {
		t.Error("no note, no other install")
	}
}

func TestVirtualAdapter(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"vEthernet (WSL (Hyper-V firewall))": true, "docker0": true, "br-3f2a": true, "VirtualBox Host-Only Network": true,
		"Ethernet": false, "Wi-Fi": false, "eth0": false, "wlp2s0": false, "en0": false,
	} {
		if got := virtualAdapter(name); got != want {
			t.Errorf("virtualAdapter(%q) = %v, want %v", name, got, want)
		}
	}
	t.Log("this computer's address for phones:", LANAddress())
}
