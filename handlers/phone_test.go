package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhonePage_AddressOrder(t *testing.T) {
	t.Parallel()
	lan := func() string { return "192.168.1.23" }
	noLan := func() string { return "" }

	cases := []struct {
		name, host, saved string
		docker            bool
		lan               func() string
		addr, source      string
		noNet             bool
	}{
		{"saved first", "192.168.1.50:8091", "http://grow.lan:8080", false, lan, "http://grow.lan:8080", PhoneFromSaved, false},
		{"saved localhost is skipped", "192.168.1.50:8091", "http://localhost:8080", false, lan, "http://192.168.1.50:8091", PhoneFromBrowser, false},
		{"browser address", "192.168.1.50:8091", "", true, lan, "http://192.168.1.50:8091", PhoneFromBrowser, false},
		{"localhost: this computer's address and port", "localhost:8080", "", false, lan, "http://192.168.1.23:8080", PhoneFromComputer, false},
		{"127.0.0.1 too", "127.0.0.1:8085", "", false, lan, "http://192.168.1.23:8085", PhoneFromComputer, false},
		{"port 80 left out", "localhost", "", false, lan, "http://192.168.1.23", PhoneFromComputer, false},
		{"no network", "localhost:8080", "", false, noLan, "", "", true},
		{"docker as localhost asks", "localhost:8091", "", true, lan, "", "", false},
		{"ipv6 loopback", "[::1]:8080", "", false, lan, "http://192.168.1.23:8080", PhoneFromComputer, false},
	}
	for _, tc := range cases {
		p := phonePage(newReq(tc.host), tc.saved, tc.docker, tc.lan)
		assert.Equal(t, tc.addr, p.Address, tc.name)
		assert.Equal(t, tc.source, p.Source, tc.name)
		assert.Equal(t, tc.noNet, p.NoNetwork, tc.name)
		assert.Equal(t, tc.addr != "", p.QR != "", tc.name)
		assert.Equal(t, tc.docker, p.Docker, tc.name)
	}

	// Behind an https proxy, and over TLS.
	r := newReq("budminder.example.com")
	r.Header.Set("X-Forwarded-Proto", "https")
	assert.Equal(t, "https://budminder.example.com", phonePage(r, "", true, lan).Address)
	r = newReq("localhost:8443")
	r.TLS = &tls.ConnectionState{}
	assert.Equal(t, "https://192.168.1.23:8443", phonePage(r, "", false, lan).Address)
	assert.False(t, phonePage(newReq("localhost:8080"), "", false, lan).Remote)
	assert.True(t, phonePage(newReq("192.168.1.50:8080"), "", false, lan).Remote)
}

func newReq(host string) *http.Request {
	r := httptest.NewRequest("GET", "/settings", nil)
	r.Host = host
	return r
}

func TestIsLocalAddress(t *testing.T) {
	t.Parallel()
	for a, want := range map[string]bool{
		"http://localhost:8080":        true,
		"http://LOCALHOST":             true,
		"http://127.0.0.1:8080/x":      true,
		"http://[::1]:8080":            true,
		"http://0.0.0.0:8080":          true,
		"http://grow.localhost":        true,
		"http://192.168.1.10:8080":     false,
		"https://budminder.example":    false,
		"http://user@localhost:8080":   true,
		"http://localhost.example.com": false,
	} {
		assert.Equal(t, want, IsLocalAddress(a), a)
	}
}

func TestQRSVG(t *testing.T) {
	t.Parallel()
	s := QRSVG("http://192.168.1.23:8080")
	assert.True(t, strings.HasPrefix(s, "<svg "))
	assert.Contains(t, s, `fill="#000" d="M`)
}
