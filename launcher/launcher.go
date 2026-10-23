// Package launcher is what makes the downloadable program friendly to start:
// the welcome box printed once BudMinder is ready, and (on Linux) a desktop
// and app-menu icon that starts BudMinder in the background and opens the
// browser. None of it applies in Docker, where the address and the desktop
// belong to another machine.
package launcher

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

// InDocker reports whether BudMinder runs in a container.
func InDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// Welcome is what the welcome box shows.
type Welcome struct {
	Port        string
	LANAddress  string // "" when none was found
	Username    string // the first sign-in, shown only while it still works
	Password    string
	IconsMade   bool // the app-menu icon was just added
	IconDesktop bool // ...and one on the desktop
	FromConsole bool // started in a terminal (the stop hint only makes sense there)
}

// Box draws the welcome box. color adds green to the border on terminals.
func Box(w Welcome, color bool) string {
	lines := []string{
		"BudMinder is running",
		"",
		"Open it in your browser:  http://localhost:" + w.Port,
	}
	if w.LANAddress != "" {
		lines = append(lines, "From your phone:          http://"+w.LANAddress+":"+w.Port)
	}
	if w.Password != "" {
		lines = append(lines, "",
			"First sign-in",
			"  user:      "+w.Username,
			"  password:  "+w.Password,
			"You'll then be asked to choose a new password.")
	}
	lines = append(lines, "", "Tip: bookmark the page to open BudMinder quickly.")
	if w.IconsMade {
		where := "app menu"
		if w.IconDesktop {
			where = "desktop and app menu"
		}
		lines = append(lines, "A BudMinder icon was added to your "+where+".",
			"Double-click it to start BudMinder and open it next time.")
	}
	if w.FromConsole {
		lines = append(lines, "To stop BudMinder, press Ctrl+C or close this window.")
	}

	width := 0
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > width {
			width = n
		}
	}
	width += 6 // three spaces each side
	on, off := "", ""
	if color {
		on, off = "\033[32m", "\033[0m"
	}
	var b strings.Builder
	edge := func(left, right string) {
		b.WriteString(on + left + strings.Repeat("═", width) + right + off + "\n")
	}
	row := func(text string) {
		pad := width - 3 - utf8.RuneCountInString(text)
		b.WriteString(on + "║" + off + "   " + text + strings.Repeat(" ", pad) + on + "║" + off + "\n")
	}
	b.WriteString("\n")
	edge("╔", "╗")
	row("")
	for _, l := range lines {
		row(l)
	}
	row("")
	edge("╚", "╝")
	return b.String()
}

// IsTerminal reports whether f is a terminal (not a file or nothing at all).
func IsTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// LANAddress is this computer's address on the home network (the first
// private IPv4 address of a network card that's up), or "".
func LANAddress() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ip := ipn.IP.To4(); ip != nil && ip.IsPrivate() {
					return ip.String()
				}
			}
		}
	}
	return ""
}

// Running reports whether a BudMinder already answers on port.
func Running(port string) bool {
	c := http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := c.Get("http://localhost:" + port + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// OpenBrowser opens url in the default browser.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("don't know how to open a browser on %s", runtime.GOOS)
	}
	return cmd.Start()
}
