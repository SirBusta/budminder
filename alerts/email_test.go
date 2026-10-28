package alerts_test

// Sending alert emails to a fake mail server (tests/testutil/fakes/smtp.go).

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

// decodeMail returns the subject and each part's decoded body by type.
func decodeMail(t *testing.T, data string) (string, map[string]string) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(data))
	require.NoError(t, err)
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	require.NoError(t, err)
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	parts := map[string]string{}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		b, err := io.ReadAll(p) // multipart undoes quoted-printable
		require.NoError(t, err)
		parts[ct] = string(b)
	}
	return subject, parts
}

func emailFor(f *fakes.FakeSMTP, security string) alerts.Email {
	e := alerts.Email{Host: f.Host, Port: f.Port, Security: security, Username: f.User, Password: f.Pass,
		From: "budminder@example.com", To: []string{"grower@example.com", "partner@example.com"}}
	e.TrustForTest(f.Certs)
	return e
}

var testMsg = alerts.Simple("VPD high in Tent 1", []string{"VPD is 1.9 kPa <above> 1.5 kPa.", "Since 14:05."}, "http://192.168.1.10:8080/alerts", "Open BudMinder")

func TestEmail_SendsOverEachSecurity(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{fakes.SMTPStartTLS, fakes.SMTPTLS, fakes.SMTPPlain} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			f := fakes.NewFakeSMTP(t, mode)
			f.User, f.Pass = "grower@example.com", "app-pass-123"
			require.NoError(t, emailFor(f, mode).Send(context.Background(), testMsg))

			got := f.Mail()
			require.Len(t, got, 1)
			assert.Equal(t, "budminder@example.com", got[0].From)
			assert.Equal(t, []string{"grower@example.com", "partner@example.com"}, got[0].To)
			assert.Equal(t, mode != fakes.SMTPPlain, got[0].TLS)
			assert.NotContains(t, got[0].Data, "app-pass-123")
			subject, parts := decodeMail(t, got[0].Data)
			assert.Equal(t, "VPD high in Tent 1", subject)
			require.Len(t, parts, 2)
			assert.Contains(t, parts["text/plain"], "VPD is 1.9 kPa <above> 1.5 kPa.")
			assert.Contains(t, parts["text/plain"], "Open BudMinder: http://192.168.1.10:8080/alerts")
			assert.Contains(t, parts["text/html"], "&lt;above&gt;", "HTML part is escaped")
			assert.Contains(t, parts["text/html"], `href="http://192.168.1.10:8080/alerts"`)
		})
	}
}

func TestEmail_LoginAuthOnly(t *testing.T) {
	t.Parallel()
	f := fakes.NewFakeSMTP(t, fakes.SMTPStartTLS)
	f.User, f.Pass, f.LoginOnly = "grower@example.com", "app-pass-123", true
	require.NoError(t, emailFor(f, alerts.SecuritySTARTTLS).Send(context.Background(), testMsg))
	assert.Len(t, f.Mail(), 1)
}

func TestEmail_ErrorsSayWhatWentWrong(t *testing.T) {
	t.Parallel()
	f := fakes.NewFakeSMTP(t, fakes.SMTPStartTLS)
	f.User, f.Pass = "grower@example.com", "right-pass"

	e := emailFor(f, alerts.SecuritySTARTTLS)
	e.Password = "wrong-pass"
	err := e.Send(context.Background(), testMsg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Username and Password not accepted")
	assert.NotContains(t, err.Error(), "wrong-pass")

	// SSL/TLS chosen for a server that starts plain.
	e = emailFor(f, alerts.SecurityTLS)
	assert.Error(t, e.Send(context.Background(), testMsg))

	// STARTTLS chosen for a server that doesn't offer it.
	plain := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	err = emailFor(plain, alerts.SecuritySTARTTLS).Send(context.Background(), testMsg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "STARTTLS")

	// An untrusted certificate is refused.
	e = emailFor(f, alerts.SecuritySTARTTLS)
	e.TrustForTest(nil)
	assert.Error(t, e.Send(context.Background(), testMsg))
	assert.Empty(t, f.Mail())
}

func TestEmail_Problem(t *testing.T) {
	t.Parallel()
	ok := alerts.Email{Host: "smtp.example.com", Port: 587, Security: alerts.SecuritySTARTTLS,
		Username: "me@example.com", Password: "x", From: "me@example.com", To: []string{"me@example.com"}}
	assert.Empty(t, ok.Problem())

	cases := map[string]func(e *alerts.Email){
		"al_err_host":     func(e *alerts.Email) { e.Host = "" },
		"al_err_port":     func(e *alerts.Email) { e.Port = 70000 },
		"al_err_security": func(e *alerts.Email) { e.Security = "ssl3" },
		"al_err_from":     func(e *alerts.Email) { e.From = "Me <me@example.com>" },
		"al_err_to":       func(e *alerts.Email) { e.To = []string{"me@example.com", "not an address"} },
		"al_err_username": func(e *alerts.Email) { e.Username = "" },
	}
	for want, change := range cases {
		e := ok
		e.To = append([]string(nil), ok.To...)
		change(&e)
		assert.Equal(t, want, e.Problem())
	}
	e := ok
	e.From = "me@example.com\r\nBcc: x@example.com"
	assert.Equal(t, "al_err_from", e.Problem())
}

func TestEmail_LoadAndSplit(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	for k, v := range map[string]string{
		alerts.SettingEmailOn: "1", alerts.SettingEmailHost: "smtp.example.com", alerts.SettingEmailPort: "465",
		alerts.SettingEmailSecurity: "tls", alerts.SettingEmailTo: "a@example.com; b@example.com,c@example.com",
	} {
		_, err := db.Exec("INSERT INTO settings (name, value) VALUES ($1, $2)", k, v)
		require.NoError(t, err)
	}
	e, err := alerts.LoadEmail(db)
	require.NoError(t, err)
	assert.True(t, e.On)
	assert.Equal(t, 465, e.Port)
	assert.Equal(t, alerts.SecurityTLS, e.Security)
	assert.Equal(t, []string{"a@example.com", "b@example.com", "c@example.com"}, e.To)

	empty, err := alerts.LoadEmail(testutil.NewTestDB(t))
	require.NoError(t, err)
	assert.False(t, empty.On)
	assert.Equal(t, alerts.SecuritySTARTTLS, empty.Security)
	assert.True(t, strings.TrimSpace(empty.Host) == "")
}
