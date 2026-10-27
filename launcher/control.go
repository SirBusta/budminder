package launcher

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// How "Stop BudMinder" reaches the running BudMinder, the same way on every
// system: while it runs, BudMinder listens on a private port that only this
// computer can reach, and writes that port and a random key into the data
// folder. --stop reads them and asks it to shut down. Without the key nothing
// happens, so another program on the computer can't stop BudMinder by
// guessing the port.

const runFile = "budminder.run" // in the data folder, while BudMinder runs

type runInfo struct {
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Key  string `json:"key"`
}

// Control is the private stop line of a running BudMinder.
type Control struct {
	Stop    <-chan struct{} // closed when --stop asks BudMinder to shut down
	dataDir string
	srv     *http.Server
}

// StartControl opens the stop line and records it in the data folder.
// version is answered to /version, so a newer copy started later can tell
// whether this one is the same version.
func StartControl(dataDir, version string) (*Control, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	info := runInfo{PID: os.Getpid(), Port: ln.Addr().(*net.TCPAddr).Port, Key: hex.EncodeToString(key)}
	b, _ := json.Marshal(info)
	if err := os.WriteFile(filepath.Join(dataDir, runFile), b, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	stop := make(chan struct{})
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-BudMinder-Key") != info.Key {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		once.Do(func() { close(stop) })
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-BudMinder-Key") != info.Key {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(version))
	})
	c := &Control{Stop: stop, dataDir: dataDir, srv: &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}}
	go func() { _ = c.srv.Serve(ln) }()
	return c, nil
}

// Close shuts the stop line and removes its record.
func (c *Control) Close() {
	_ = c.srv.Close()
	_ = os.Remove(filepath.Join(c.dataDir, runFile))
}

// RunningHere reports whether a BudMinder runs from this folder (its stop
// line answers), and its version: "" for one from before 1.5.8, which can't
// say.
func RunningHere(dataDir string) (version string, ok bool) {
	b, err := os.ReadFile(filepath.Join(dataDir, runFile))
	if err != nil {
		return "", false
	}
	var info runInfo
	if json.Unmarshal(b, &info) != nil || info.Port == 0 {
		return "", false
	}
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(info.Port)+"/version", nil)
	req.Header.Set("X-BudMinder-Key", info.Key)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", true // the stop line answers, but it's older than /version
	}
	v, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	return strings.TrimSpace(string(v)), true
}

// WaitStopped waits (up to 30 s) for the BudMinder running from this folder
// to finish stopping: its stop line record goes away.
func WaitStopped(dataDir string) {
	for range 60 {
		if _, err := os.Stat(filepath.Join(dataDir, runFile)); err != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(time.Second)
}

// Stop asks the BudMinder running from this folder to shut down, the same way
// Ctrl+C does.
func Stop(dataDir string) error {
	notRunning := errors.New("BudMinder isn't running from this folder")
	b, err := os.ReadFile(filepath.Join(dataDir, runFile))
	if err != nil {
		return notRunning
	}
	var info runInfo
	if json.Unmarshal(b, &info) != nil || info.Port == 0 {
		return notRunning
	}
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(info.Port)+"/stop", nil)
	req.Header.Set("X-BudMinder-Key", info.Key)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		_ = os.Remove(filepath.Join(dataDir, runFile)) // left over from a run that didn't exit cleanly
		return notRunning
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return notRunning
	}
	return nil
}
