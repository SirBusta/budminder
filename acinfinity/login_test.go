package acinfinity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/acinfinity"
)

// fakeACI answers like AC Infinity's sign-in for one email/password.
func fakeACI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/appUserLogin" || r.ParseForm() != nil {
			http.NotFound(w, r)
			return
		}
		if r.PostForm.Get("appEmail") == "grower@example.com" && r.PostForm.Get("appPasswordl") == "1234567890123456789012345" {
			_, _ = w.Write([]byte(`{"code":200,"msg":"success","data":{"appId":"NEWTOKEN"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":10001,"msg":"Incorrect account or password","data":{}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLogin(t *testing.T) {
	t.Parallel()
	srv := fakeACI(t)

	// The password is cut to AC Infinity's 25 characters.
	tok, err := acinfinity.Login(context.Background(), nil, srv.URL, "grower@example.com", "1234567890123456789012345-and-more")
	require.NoError(t, err)
	assert.Equal(t, "NEWTOKEN", tok)

	_, err = acinfinity.Login(context.Background(), nil, srv.URL, "grower@example.com", "wrong")
	var refused *acinfinity.RefusedError
	require.True(t, errors.As(err, &refused), "got %v", err)
	assert.Equal(t, "Incorrect account or password", refused.Msg)

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	_, err = acinfinity.Login(context.Background(), nil, gone.URL, "grower@example.com", "x")
	assert.ErrorIs(t, err, acinfinity.ErrUnreachable)
}
