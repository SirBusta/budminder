// Package alerts sends alert messages to the grower. Email is the first
// channel (Settings > Alerts, handlers/alert_settings.go).
//
// It must not import handlers or watcher: the watcher calls into it.
package alerts

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// Settings rows for email alerts.
const (
	SettingEmailOn       = "alert_email_enabled" // "1": send alert emails
	SettingEmailHost     = "alert_email_host"
	SettingEmailPort     = "alert_email_port"
	SettingEmailSecurity = "alert_email_security" // SecuritySTARTTLS, SecurityTLS or SecurityNone
	SettingEmailUser     = "alert_email_username"
	SettingEmailPassword = "alert_email_password" // left out of backups (handlers.secretSettings)
	SettingEmailFrom     = "alert_email_from"
	SettingEmailTo       = "alert_email_to"  // comma-separated
	SettingLinkAddress   = "alert_link_base" // e.g. http://192.168.1.10:8080, for links in messages
)

// Email security choices.
const (
	SecuritySTARTTLS = "starttls" // usually port 587
	SecurityTLS      = "tls"      // usually port 465
	SecurityNone     = "none"     // usually port 25; local relays only
)

// MaxRecipients caps the To list.
const MaxRecipients = 10

// Email is the mail server and addresses alert emails use.
type Email struct {
	On       bool
	Host     string
	Port     int
	Security string
	Username string
	Password string
	From     string
	To       []string
	Link     string // BudMinder's address, for links in messages

	rootCAs *x509.CertPool // tests: trust a self-signed server
}

// Message is one email: a subject and the same text as plain text and HTML.
type Message struct {
	Subject string
	Text    string
	HTML    string
}

// LoadEmail reads the email settings from the settings table.
func LoadEmail(db *sql.DB) (Email, error) {
	get := func(name string) (string, error) {
		var v string
		err := db.QueryRow("SELECT value FROM settings WHERE name = $1", name).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return v, err
	}
	var e Email
	vals := map[string]string{}
	for _, n := range []string{SettingEmailOn, SettingEmailHost, SettingEmailPort, SettingEmailSecurity,
		SettingEmailUser, SettingEmailPassword, SettingEmailFrom, SettingEmailTo, SettingLinkAddress} {
		v, err := get(n)
		if err != nil {
			return e, err
		}
		vals[n] = v
	}
	e.On = vals[SettingEmailOn] == "1"
	e.Host = vals[SettingEmailHost]
	e.Port, _ = strconv.Atoi(vals[SettingEmailPort])
	e.Security = vals[SettingEmailSecurity]
	if e.Security == "" {
		e.Security = SecuritySTARTTLS
	}
	e.Username = vals[SettingEmailUser]
	e.Password = vals[SettingEmailPassword]
	e.From = vals[SettingEmailFrom]
	e.To = SplitAddresses(vals[SettingEmailTo])
	e.Link = vals[SettingLinkAddress]
	return e, nil
}

// SplitAddresses splits a comma, semicolon or space separated list.
func SplitAddresses(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' }) {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// Problem names what's wrong with the settings, as a translation key, or ""
// when they're complete enough to try sending.
func (e Email) Problem() string {
	switch {
	case e.Host == "" || strings.ContainsAny(e.Host, " /:@"):
		return "al_err_host"
	case e.Port < 1 || e.Port > 65535:
		return "al_err_port"
	case e.Security != SecuritySTARTTLS && e.Security != SecurityTLS && e.Security != SecurityNone:
		return "al_err_security"
	case !validAddress(e.From):
		return "al_err_from"
	case len(e.To) == 0 || len(e.To) > MaxRecipients:
		return "al_err_to"
	case e.Password != "" && e.Username == "":
		return "al_err_username"
	}
	for _, a := range e.To {
		if !validAddress(a) {
			return "al_err_to"
		}
	}
	return ""
}

// validAddress accepts a bare address (name@host), the only form the
// settings take, so no display name or CR/LF reaches a header.
func validAddress(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && a.Name == "" && !strings.ContainsAny(s, "\r\n")
}

// Send delivers m to every recipient. Errors carry the mail server's own
// words (they never include the password); Hint turns them into advice.
func (e Email) Send(ctx context.Context, m Message) error {
	if p := e.Problem(); p != "" {
		return fmt.Errorf("email settings incomplete (%s)", p)
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	addr := net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
	tlsConf := &tls.Config{ServerName: e.Host, RootCAs: e.rootCAs, MinVersion: tls.VersionTLS12}

	d := &net.Dialer{}
	var conn net.Conn
	var err error
	if e.Security == SecurityTLS {
		conn, err = (&tls.Dialer{NetDialer: d, Config: tlsConf}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return failed(stepConnect, fmt.Errorf("connect to %s: %w", addr, err))
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, e.Host)
	if err != nil {
		conn.Close()
		return failed(stepGreeting, fmt.Errorf("mail server greeting: %w", err))
	}
	defer c.Close()

	if e.Security == SecuritySTARTTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return failed(stepStartTLS, errors.New("the mail server doesn't offer STARTTLS; choose SSL/TLS or None"))
		}
		if err := c.StartTLS(tlsConf); err != nil {
			return failed(stepStartTLS, fmt.Errorf("STARTTLS: %w", err))
		}
	}
	if e.Username != "" {
		if err := c.Auth(e.auth(c)); err != nil {
			return failed(stepAuth, fmt.Errorf("sign in: %w", err))
		}
	}
	if err := c.Mail(e.From); err != nil {
		return failed(stepSender, fmt.Errorf("sender %s: %w", e.From, err))
	}
	for _, to := range e.To {
		if err := c.Rcpt(to); err != nil {
			return failed(stepRecipient, fmt.Errorf("recipient %s: %w", to, err))
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	if _, err := w.Write(e.build(m, time.Now())); err != nil {
		w.Close()
		return fmt.Errorf("send: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return c.Quit()
}

// The steps of Send, so Hint knows where it failed.
const (
	stepConnect   = "connect"
	stepGreeting  = "greeting"
	stepStartTLS  = "starttls"
	stepAuth      = "auth"
	stepSender    = "sender"
	stepRecipient = "recipient"
)

type sendError struct {
	step string
	err  error
}

func (s *sendError) Error() string { return s.err.Error() }
func (s *sendError) Unwrap() error { return s.err }

func failed(step string, err error) error { return &sendError{step, err} }

// microsoftHosts send for Outlook.com, Hotmail, Live and Microsoft 365.
var microsoftHosts = []string{"outlook.com", "office365.com", "hotmail.com", "live.com"}

func isMicrosoft(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range microsoftHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// Hint returns the text key (al_hint_*) of plain advice for an error from
// Send to host, or "" when there's nothing better than the server's words.
func Hint(err error, host string) string {
	var se *sendError
	if !errors.As(err, &se) {
		return ""
	}
	msg := strings.ToLower(err.Error())
	code := 0
	var tp *textproto.Error
	if errors.As(err, &tp) {
		code = tp.Code
	}
	var dns *net.DNSError
	var rec tls.RecordHeaderError
	var verify *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var name x509.HostnameError
	var invalid x509.CertificateInvalidError
	cert := errors.As(err, &verify) || errors.As(err, &unknown) || errors.As(err, &name) || errors.As(err, &invalid)

	switch se.step {
	case stepConnect, stepGreeting, stepStartTLS:
		switch {
		case errors.As(err, &dns):
			return "al_hint_host"
		case cert:
			return "al_hint_cert"
		case se.step == stepConnect && !errors.As(err, &rec):
			return "al_hint_reach"
		}
		// SSL/TLS on a plain port, a plain start on an SSL port (the server
		// waits for TLS and never greets), or no STARTTLS offered.
		return "al_hint_security"
	case stepAuth:
		switch {
		case isMicrosoft(host) || strings.Contains(msg, "5.7.139") || strings.Contains(msg, "basic authentication is disabled"):
			return "al_hint_microsoft"
		case strings.Contains(msg, "unencrypted connection"):
			return "al_hint_unencrypted"
		case code == 534 || code == 535 || strings.Contains(msg, "password"):
			return "al_hint_password"
		}
	case stepSender:
		if code == 530 {
			return "al_hint_login_needed"
		}
		return "al_hint_from"
	case stepRecipient:
		return "al_hint_to"
	}
	return ""
}

// auth picks PLAIN, or LOGIN when the server only offers that (some
// Microsoft servers). net/smtp's PlainAuth refuses to send the password over
// an unencrypted connection to anything but localhost.
func (e Email) auth(c *smtp.Client) smtp.Auth {
	_, mechs := c.Extension("AUTH")
	if !strings.Contains(" "+strings.ToUpper(mechs)+" ", " PLAIN ") && strings.Contains(strings.ToUpper(mechs), "LOGIN") {
		return &loginAuth{user: e.Username, pass: e.Password, host: e.Host}
	}
	return smtp.PlainAuth("", e.Username, e.Password, e.Host)
}

type loginAuth struct{ user, pass, host string }

func (a *loginAuth) Start(s *smtp.ServerInfo) (string, []byte, error) {
	if !s.TLS && s.Name != "localhost" && s.Name != "127.0.0.1" && s.Name != "::1" {
		return "", nil, errors.New("unencrypted connection")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(from []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(from))) {
	case "username:", "user name", "username":
		return []byte(a.user), nil
	case "password:", "password":
		return []byte(a.pass), nil
	}
	return nil, fmt.Errorf("unexpected server challenge %q", from)
}

// build makes the multipart/alternative message (plain text, then HTML).
func (e Email) build(m Message, now time.Time) []byte {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	id := hex.EncodeToString(raw[:])
	boundary := "bm-" + id
	domain := "budminder.local"
	if i := strings.LastIndex(e.From, "@"); i >= 0 {
		domain = e.From[i+1:]
	}

	var b bytes.Buffer
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", mime.QEncoding.Encode("utf-8", "BudMinder")+" <"+e.From+">")
	h("To", strings.Join(e.To, ", "))
	h("Subject", mime.QEncoding.Encode("utf-8", oneLine(m.Subject)))
	h("Date", now.Format(time.RFC1123Z))
	h("Message-ID", "<"+id+"@"+domain+">")
	h("MIME-Version", "1.0")
	h("Auto-Submitted", "auto-generated")
	h("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	part := func(ctype, body string) {
		b.WriteString("--" + boundary + "\r\n")
		h("Content-Type", ctype+"; charset=utf-8")
		h("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		qp := quotedprintable.NewWriter(&b)
		_, _ = qp.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n")))
		_ = qp.Close()
		b.WriteString("\r\n")
	}
	part("text/plain", m.Text)
	part("text/html", m.HTML)
	b.WriteString("--" + boundary + "--\r\n")
	return b.Bytes()
}

// Simple makes a Message from plain paragraphs and an optional link
// (linkText to link), escaping everything for the HTML part.
func Simple(subject string, paras []string, link, linkText string) Message {
	text := strings.Join(paras, "\n\n")
	var hb strings.Builder
	hb.WriteString(`<div style="font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;font-size:15px;line-height:1.5;color:#1f2a1f">`)
	for _, p := range paras {
		hb.WriteString("<p>" + html.EscapeString(p) + "</p>")
	}
	if link != "" {
		text += "\n\n" + linkText + ": " + link
		hb.WriteString(`<p><a href="` + html.EscapeString(link) + `" style="color:#2f7d32">` + html.EscapeString(linkText) + `</a></p>`)
	}
	hb.WriteString(`<p style="color:#6b766b;font-size:12px">BudMinder</p></div>`)
	return Message{Subject: subject, Text: text + "\n\n-- \nBudMinder\n", HTML: hb.String()}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
