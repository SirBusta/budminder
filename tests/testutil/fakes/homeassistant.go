package fakes

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"isley/tests/testutil"
)

// HomeAssistantToken is the access token FakeHomeAssistant accepts.
const HomeAssistantToken = "test-ha-token"

// FakeHA is a stand-in Home Assistant server. It answers GET /api/ and
// GET /api/states the way Home Assistant does, but only with
// "Authorization: Bearer <HomeAssistantToken>"; any other token gets 401,
// any other path 404.
type FakeHA struct {
	*httptest.Server
	// States is the /api/states body; tests may replace it before a call.
	States atomic.Value // []byte
	// Hits counts /api/states requests.
	Hits atomic.Int64
}

// FakeHomeAssistant starts a FakeHA serving
// tests/fixtures/homeassistant/<fixture>.json from /api/states.
func FakeHomeAssistant(t *testing.T, fixture string) *FakeHA {
	t.Helper()
	f := &FakeHA{}
	f.States.Store(testutil.MustReadFixture(t, "homeassistant/"+fixture+".json"))
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+HomeAssistantToken {
			http.Error(w, "401: Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/":
			_, _ = w.Write([]byte(`{"message":"API running."}`))
		case "/api/states":
			f.Hits.Add(1)
			_, _ = w.Write(f.States.Load().([]byte))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Server.Close)
	return f
}
