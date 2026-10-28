package fakes

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// SMTP security modes for FakeSMTP.
const (
	SMTPPlain    = "none"     // no TLS at all
	SMTPStartTLS = "starttls" // plain connection that offers STARTTLS
	SMTPTLS      = "tls"      // TLS from the first byte (port 465 style)
)

// SMTPMail is one message FakeSMTP accepted.
type SMTPMail struct {
	From string
	To   []string
	Data string
	TLS  bool // the message came over an encrypted connection
}

// FakeSMTP is a tiny mail server for tests: EHLO, STARTTLS, AUTH PLAIN and
// LOGIN, MAIL, RCPT, DATA, QUIT. Host is 127.0.0.1; Certs trusts its
// self-signed certificate.
type FakeSMTP struct {
	Host  string
	Port  int
	Certs *x509.CertPool
	// User and Pass, when set, are the only sign-in accepted.
	User, Pass string
	// LoginOnly offers only AUTH LOGIN (like some Microsoft servers).
	LoginOnly bool

	mode string
	tls  *tls.Config
	ln   net.Listener
	mu   sync.Mutex
	mail []SMTPMail
}

// NewFakeSMTP starts a FakeSMTP in the given mode, closed when t ends.
func NewFakeSMTP(t *testing.T, mode string) *FakeSMTP {
	t.Helper()
	cert, pool := selfSigned(t)
	f := &FakeSMTP{Host: "127.0.0.1", Certs: pool, mode: mode,
		tls: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}}
	var err error
	if mode == SMTPTLS {
		f.ln, err = tls.Listen("tcp", "127.0.0.1:0", f.tls)
	} else {
		f.ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("FakeSMTP listen: %v", err)
	}
	f.Port = f.ln.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { f.ln.Close() })
	go f.serve()
	return f
}

// Mail returns the messages accepted so far.
func (f *FakeSMTP) Mail() []SMTPMail {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SMTPMail(nil), f.mail...)
}

func (f *FakeSMTP) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.session(c)
	}
}

func (f *FakeSMTP) session(c net.Conn) {
	defer func() { c.Close() }()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	_, encrypted := c.(*tls.Conn)
	r, w := bufio.NewReader(c), bufio.NewWriter(c)
	say := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }
	read := func() (string, bool) {
		line, err := r.ReadString('\n')
		return strings.TrimRight(line, "\r\n"), err == nil
	}
	say("220 fake.smtp ESMTP")
	authed := f.User == ""
	var m SMTPMail
	for {
		line, ok := read()
		if !ok {
			return
		}
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			lines := []string{"fake.smtp"}
			if f.mode == SMTPStartTLS && !encrypted {
				lines = append(lines, "STARTTLS")
			}
			if f.LoginOnly {
				lines = append(lines, "AUTH LOGIN")
			} else {
				lines = append(lines, "AUTH PLAIN LOGIN")
			}
			for i, l := range lines {
				sep := "-"
				if i == len(lines)-1 {
					sep = " "
				}
				w.WriteString("250" + sep + l + "\r\n")
			}
			w.Flush()
		case cmd == "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(c, f.tls)
			if tc.Handshake() != nil {
				return
			}
			c, encrypted = tc, true
			r, w = bufio.NewReader(c), bufio.NewWriter(c)
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			arg := strings.TrimSpace(line[len("AUTH PLAIN"):])
			if f.LoginOnly {
				say("504 unrecognized authentication type")
				continue
			}
			raw, _ := base64.StdEncoding.DecodeString(arg)
			parts := strings.Split(string(raw), "\x00")
			if len(parts) == 3 && parts[1] == f.User && parts[2] == f.Pass {
				authed = true
				say("235 ok")
			} else {
				say("535 5.7.8 Username and Password not accepted")
			}
		case cmd == "AUTH LOGIN":
			say("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
			u, _ := read()
			say("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
			p, _ := read()
			uu, _ := base64.StdEncoding.DecodeString(u)
			pp, _ := base64.StdEncoding.DecodeString(p)
			if string(uu) == f.User && string(pp) == f.Pass {
				authed = true
				say("235 ok")
			} else {
				say("535 5.7.8 Username and Password not accepted")
			}
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			if !authed {
				say("530 authentication required")
				continue
			}
			m = SMTPMail{From: strings.Trim(line[len("MAIL FROM:"):], "<> "), TLS: encrypted}
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			m.To = append(m.To, strings.Trim(line[len("RCPT TO:"):], "<> "))
			say("250 ok")
		case cmd == "DATA":
			say("354 end with .")
			var b strings.Builder
			for {
				l, ok := read()
				if !ok {
					return
				}
				if l == "." {
					break
				}
				b.WriteString(strings.TrimPrefix(l, ".") + "\r\n")
			}
			m.Data = b.String()
			f.mu.Lock()
			f.mail = append(f.mail, m)
			f.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}
