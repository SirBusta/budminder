package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.5.4", "1.5.3", true}, {"1.10.0", "1.9.9", true}, {"v2.0.0", "1.99.99", true},
		{"1.5.3", "1.5.3", false}, {"1.5.2", "1.5.3", false}, {"dev", "1.5.3", false}, {"1.5.4", "dev", false},
		{"1.5", "1.4.9", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// fakeGitHub serves a release whose program for this computer is prog.
func fakeGitHub(t *testing.T, tag string, prog []byte, sum string) *httptest.Server {
	t.Helper()
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag, "html_url": "https://example.test/r/" + tag, "body": "## What's new",
			"assets": []map[string]any{
				{"name": name, "browser_download_url": srv.URL + "/dl/" + name, "size": len(prog)},
				{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/dl/SHA256SUMS.txt", "size": 100},
			},
		})
	})
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(prog) })
	mux.HandleFunc("/dl/SHA256SUMS.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sum + "  " + name + "\nffff  some-other-file\n"))
	})
	return srv
}

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestCheckAndInstall(t *testing.T) {
	t.Parallel()
	prog := []byte("the new program")
	srv := fakeGitHub(t, "v1.5.4", prog, sha(prog))
	c := New("1.5.3", srv.URL+"/latest", nil)
	if c.Available() != nil {
		t.Fatal("something available before any check")
	}
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	rel := c.Available()
	if rel == nil || rel.Version != "1.5.4" || !rel.CanInstall() {
		t.Fatalf("want 1.5.4 installable, got %+v", rel)
	}

	exe := filepath.Join(t.TempDir(), AssetName(runtime.GOOS, runtime.GOARCH))
	if err := os.WriteFile(exe, []byte("the old program"), 0o755); err != nil {
		t.Fatal(err)
	}
	backedUp := false
	if err := c.Install(context.Background(), rel, exe, func() error { backedUp = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "the new program" {
		t.Errorf("program not replaced: %q", got)
	}
	if got, _ := os.ReadFile(exe + ".old"); string(got) != "the old program" {
		t.Errorf("old program not kept as a fallback: %q", got)
	}
	if !backedUp {
		t.Error("no backup before the swap")
	}
	if err := os.WriteFile(exe+".new", []byte("half a download"), 0o644); err != nil {
		t.Fatal(err)
	}
	RemoveLeftovers(exe)
	if _, err := os.Stat(exe + ".new"); err == nil {
		t.Error("RemoveLeftovers left the half-finished download")
	}
	if _, err := os.Stat(exe + ".old"); err != nil {
		t.Error("the fallback must stay until the next update")
	}
}

func TestInstall_RefusesABadChecksum(t *testing.T) {
	t.Parallel()
	srv := fakeGitHub(t, "v1.5.4", []byte("tampered"), sha([]byte("the real program")))
	c := New("1.5.3", srv.URL+"/latest", nil)
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "budminder")
	if err := os.WriteFile(exe, []byte("the old program"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := c.Install(context.Background(), c.Available(), exe, nil); err == nil {
		t.Fatal("installed a download that doesn't match its checksum")
	}
	if got, _ := os.ReadFile(exe); string(got) != "the old program" {
		t.Errorf("the old program was touched: %q", got)
	}
	if _, err := os.Stat(exe + ".new"); err == nil {
		t.Error("the bad download was left behind")
	}
}

func TestAvailable_OffOrNotNewer(t *testing.T) {
	t.Parallel()
	srv := fakeGitHub(t, "v1.5.3", []byte("x"), sha([]byte("x")))
	same := New("1.5.3", srv.URL+"/latest", nil)
	_ = same.Check(context.Background())
	if same.Available() != nil {
		t.Error("offered the version already running")
	}
	off := true
	newer := fakeGitHub(t, "v9.0.0", []byte("x"), sha([]byte("x")))
	c := New("1.5.3", newer.URL+"/latest", func() bool { return off })
	_ = c.Check(context.Background())
	if c.Available() != nil {
		t.Error("offered an update with checks turned off")
	}
	off = false
	if c.Available() == nil {
		t.Error("not offered once checks are on again")
	}
}

func TestAssetName(t *testing.T) {
	t.Parallel()
	for goos, want := range map[string]string{"linux": "budminder-linux-amd64", "darwin": "budminder-macos-amd64", "windows": "budminder-windows-amd64.exe"} {
		if got := AssetName(goos, "amd64"); got != want {
			t.Errorf("AssetName(%s) = %s, want %s", goos, got, want)
		}
	}
}
