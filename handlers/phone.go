package handlers

// Settings > Phone & tablet (mockup data/mockups/settings/phone/v1.html):
// the address a phone on the home network uses to open BudMinder, and a QR
// code for it made here (rsc.io/qr, as the tray's "Open on your phone…"
// window), so nothing goes to an outside service.

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"rsc.io/qr"

	"isley/alerts"
	"isley/launcher"
)

// Where the phone address came from.
const (
	PhoneFromSaved    = "saved"    // Change on this page, or BudMinder address in Alerts (one setting)
	PhoneFromBrowser  = "browser"  // the address this browser opened BudMinder at
	PhoneFromComputer = "computer" // this computer's home-network address
)

// PhonePage is the page data of Settings > Phone & tablet.
type PhonePage struct {
	Address string        // "" = BudMinder can't tell (Docker opened as localhost): it asks
	Source  string        // PhoneFrom*, or ""
	QR      template.HTML // SVG, or "" without an address
	// NoNetwork: this computer has no home-network address and the page was
	// opened on it, so no phone can reach BudMinder.
	NoNetwork bool
	Docker    bool
	Windows   bool // the Windows firewall steps (not in Docker)
	// Remote: the page was opened from another device ("the computer
	// running BudMinder" rather than "this computer").
	Remote bool
}

// PhonePageData works out the address in this order: a saved address that
// isn't localhost, the browser's address when it isn't localhost, outside
// Docker this computer's home-network address (with the port the browser
// used), else none.
func PhonePageData(c *gin.Context) PhonePage {
	return phonePage(c.Request, savedLink(c), launcher.InDocker(), launcher.LANAddress)
}

func savedLink(c *gin.Context) string {
	var v string
	_ = DBFromContext(c).QueryRow(`SELECT value FROM settings WHERE name = $1`, alerts.SettingLinkAddress).Scan(&v)
	return v
}

func phonePage(r *http.Request, saved string, docker bool, lan func() string) PhonePage {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host, port := splitHost(r.Host)
	p := PhonePage{Docker: docker, Windows: runtime.GOOS == "windows" && !docker, Remote: !isLocalHost(host)}
	switch {
	case saved != "" && !IsLocalAddress(saved):
		p.Address, p.Source = saved, PhoneFromSaved
	case host != "" && !isLocalHost(host):
		p.Address, p.Source = scheme+"://"+r.Host, PhoneFromBrowser
	case !docker:
		if ip := lan(); ip != "" {
			a := ip
			if port != "" && !(scheme == "http" && port == "80") && !(scheme == "https" && port == "443") {
				a = net.JoinHostPort(ip, port)
			}
			p.Address, p.Source = scheme+"://"+a, PhoneFromComputer
		} else {
			p.NoNetwork = true
		}
	}
	if p.Address != "" {
		p.QR = template.HTML(QRSVG(p.Address))
	}
	return p
}

// splitHost splits a Host header into host and port ("" when none).
func splitHost(h string) (string, string) {
	if host, port, err := net.SplitHostPort(h); err == nil {
		return host, port
	}
	return strings.Trim(h, "[]"), ""
}

// isLocalHost reports whether host only works on the computer itself.
func isLocalHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// IsLocalAddress reports whether address (http://host:port...) points at
// localhost, which a phone can't open.
func IsLocalAddress(address string) bool {
	rest := address
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}
	host, _ := splitHost(rest)
	return isLocalHost(host)
}

// QRSVG draws text as a QR code (medium error correction, 4-module quiet
// zone) in SVG, black on white whatever the theme so every camera reads it.
func QRSVG(text string) string {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return ""
	}
	const quiet = 4
	n := code.Size + 2*quiet
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" aria-hidden="true"><rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, n, n, n, n)
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}

// PhoneAddressHandler: GET /settings/phone gives the page data again after
// the address changed (here or in Alerts).
func PhoneAddressHandler(c *gin.Context) {
	p := PhonePageData(c)
	c.JSON(http.StatusOK, gin.H{"address": p.Address, "source": p.Source, "qr": string(p.QR), "noNetwork": p.NoNetwork})
}
