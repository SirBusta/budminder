package launcher

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// TestStop_KeepsRunFileWhenBusy: a BudMinder that accepts the stop
// request but doesn't answer in time is still running, so its run file
// stays for the next try.
func TestStop_KeepsRunFileWhenBusy(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() { // accept and never answer
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()

	dir := t.TempDir()
	b, _ := json.Marshal(runInfo{PID: 1, Port: ln.Addr().(*net.TCPAddr).Port, Key: "k"})
	if err := os.WriteFile(filepath.Join(dir, runFile), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if Stop(dir) == nil {
		t.Fatal("Stop succeeded against a server that never answered")
	}
	if _, err := os.Stat(filepath.Join(dir, runFile)); err != nil {
		t.Fatalf("run file removed: %v", err)
	}
}
