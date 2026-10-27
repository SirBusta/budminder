package launcher

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The port BudMinder uses when BUDMINDER_PORT isn't set: 8080, unless the
// "port taken" window picked another one, kept in the data folder.

const portFile = "port" // in the data folder

// SavedPort is the port chosen in the "port taken" window, or "".
func SavedPort(dataDir string) string {
	b, err := os.ReadFile(filepath.Join(dataDir, portFile))
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(b))
	if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
		return ""
	}
	return p
}

// SavePort keeps port for the next starts.
func SavePort(dataDir, port string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, portFile), []byte(port+"\n"), 0o644)
}

// PortFree reports whether BudMinder could listen on port.
func PortFree(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	return ln.Close()
}

// FreePortAfter is the first free port after port (up to 20 tries), or "".
func FreePortAfter(port string) string {
	n, err := strconv.Atoi(port)
	if err != nil {
		return ""
	}
	for p := n + 1; p <= n+20 && p <= 65535; p++ {
		if PortFree(strconv.Itoa(p)) == nil {
			return strconv.Itoa(p)
		}
	}
	return ""
}
