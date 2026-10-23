// Package updates checks GitHub for a newer BudMinder release (at start and
// once a day) and, for the downloadable programs, installs it: download the
// file for this computer, check it against the release's SHA256SUMS.txt,
// back up the database, swap the program (keeping the old one as a
// fallback), and let main restart BudMinder. Docker installs only get told.
package updates

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAPI is where releases are listed.
const DefaultAPI = "https://api.github.com/repos/SirBusta/budminder/releases/latest"

// Release is the newest release GitHub lists.
type Release struct {
	Version string  `json:"version"` // "1.5.4"
	URL     string  `json:"url"`     // its release page
	Notes   string  `json:"notes"`   // its release notes (Markdown)
	Assets  []Asset `json:"-"`
}

// Asset is a downloadable file of a release.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// Checker keeps the newest release it has seen.
type Checker struct {
	current  string
	api      string
	client   *http.Client
	disabled func() bool // Settings switch: true stops checks and hides the notice

	mu     sync.Mutex
	latest *Release
}

// New makes a checker for this version. api "" means GitHub.
func New(current, api string, disabled func() bool) *Checker {
	if api == "" {
		api = DefaultAPI
	}
	if disabled == nil {
		disabled = func() bool { return false }
	}
	return &Checker{current: strings.TrimPrefix(current, "v"), api: api, disabled: disabled,
		client: &http.Client{Timeout: 30 * time.Second}}
}

// Current is the running version.
func (c *Checker) Current() string { return c.current }

// Run checks now and then once a day until ctx ends.
func (c *Checker) Run(ctx context.Context) {
	for {
		if !c.disabled() {
			_ = c.Check(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
	}
}

// Check asks GitHub for the newest release.
func (c *Checker) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.api, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "BudMinder/"+c.current+" (+https://github.com/SirBusta/budminder)")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release check: %s", resp.Status)
	}
	var gh struct {
		Tag    string `json:"tag_name"`
		URL    string `json:"html_url"`
		Body   string `json:"body"`
		Draft  bool   `json:"draft"`
		Pre    bool   `json:"prerelease"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&gh); err != nil {
		return err
	}
	if gh.Draft || gh.Pre || gh.Tag == "" {
		return nil
	}
	rel := &Release{Version: strings.TrimPrefix(gh.Tag, "v"), URL: gh.URL, Notes: gh.Body}
	for _, a := range gh.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
	}
	c.mu.Lock()
	c.latest = rel
	c.mu.Unlock()
	return nil
}

// Available is the newer release to offer, or nil (none, or checks are off).
func (c *Checker) Available() *Release {
	if c == nil || c.disabled() {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.latest == nil || !Newer(c.latest.Version, c.current) {
		return nil
	}
	r := *c.latest
	return &r
}

// Newer reports whether version a is newer than b ("1.10.0" > "1.9.2").
// Anything that isn't a plain version number never counts as newer.
func Newer(a, b string) bool {
	pa, okA := parse(a)
	pb, okB := parse(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// AssetName is the release file for this computer, as the release workflow
// names them: budminder-linux-amd64, budminder-macos-arm64,
// budminder-windows-amd64.exe.
func AssetName(goos, goarch string) string {
	osName := goos
	if goos == "darwin" {
		osName = "macos"
	}
	name := "budminder-" + osName + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func (r *Release) asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// CanInstall reports whether this release has a file for this computer and
// the checksums to check it.
func (r *Release) CanInstall() bool {
	_, ok1 := r.asset(AssetName(runtime.GOOS, runtime.GOARCH))
	_, ok2 := r.asset("SHA256SUMS.txt")
	return ok1 && ok2
}

// maxDownload caps a program download (the releases are about 65 MB).
const maxDownload = 400 << 20

// Install replaces the program file exe with the release's version for this
// computer: download next to it, check the checksum, run beforeSwap (the
// database backup), then rename exe to exe.old and the download to exe.
// Windows lets a running program be renamed, just not overwritten. The new
// program takes over when BudMinder restarts.
func (c *Checker) Install(ctx context.Context, r *Release, exe string, beforeSwap func() error) error {
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	file, ok := r.asset(name)
	if !ok {
		return fmt.Errorf("this release has no %s", name)
	}
	sumsAsset, ok := r.asset("SHA256SUMS.txt")
	if !ok {
		return errors.New("this release has no SHA256SUMS.txt to check the download against")
	}
	sums, err := c.fetch(ctx, sumsAsset.URL, 1<<20)
	if err != nil {
		return fmt.Errorf("checksums: %w", err)
	}
	want := ""
	sc := bufio.NewScanner(strings.NewReader(string(sums)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return fmt.Errorf("SHA256SUMS.txt doesn't list %s", name)
	}

	tmp := exe + ".new"
	if err := c.download(ctx, file.URL, tmp, want); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if beforeSwap != nil {
		if err := beforeSwap(); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("backup before updating: %w", err)
		}
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // put the old one back
		return err
	}
	return nil
}

func (c *Checker) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func (c *Checker) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "BudMinder/"+c.current)
	dl := &http.Client{Timeout: 15 * time.Minute}
	resp, err := dl.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	return resp, nil
}

func (c *Checker) download(ctx context.Context, url, dst, wantSum string) error {
	resp, err := c.get(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, maxDownload+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > maxDownload {
		return errors.New("the download is larger than expected")
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSum {
		return errors.New("the download doesn't match its checksum, so it wasn't installed")
	}
	return os.Chmod(dst, 0o755)
}

// RemoveLeftovers deletes a half-finished download (exe.new) from an update
// that didn't complete. The previous version (exe.old) stays as a fallback
// until the next update replaces it, so going back is a rename away.
func RemoveLeftovers(exe string) {
	if exe != "" {
		_ = os.Remove(exe + ".new")
	}
}

// BackupDir is where the before-update database copies go.
func BackupDir(dataDir string) string { return filepath.Join(dataDir, "backups") }
