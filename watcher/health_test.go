package watcher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// AC Infinity answers 200 OK with its own error code when the saved token no
// longer works; before 1.0.1 that was ignored and polling silently got nothing.
func TestPollACI_RefusedTokenIsAnAuthError(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	id := seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.tempC")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":100001,"msg":"Token invalid","data":[]}`))
	}))
	t.Cleanup(srv.Close)

	w := newTestWatcher(t, db)
	w.ACIBaseURL = srv.URL
	err := w.PollACI(context.Background(), "old-token")

	var pe *PollError
	require.True(t, errors.As(err, &pe), "got %v", err)
	assert.Equal(t, "auth", pe.Kind)
	assert.Equal(t, "100001 Token invalid", pe.Detail)
	assert.Equal(t, 0, countSensorData(t, db, id))
}

func TestPollACI_Unreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	w := newTestWatcher(t, testutil.NewTestDB(t))
	w.ACIBaseURL = srv.URL
	var pe *PollError
	require.True(t, errors.As(w.PollACI(context.Background(), "tok"), &pe))
	assert.Equal(t, "unreachable", pe.Kind)
}

// Run passes each integration's outcome to ReportHealth.
func TestRun_ReportsIntegrationHealth(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.tempC")

	for _, c := range []struct {
		body, kind string
	}{
		{`{"code":100001,"msg":"Token invalid","data":[]}`, "auth"},
		{string(testutil.MustReadFixture(t, "aci/happy.json")), ""},
	} {
		synctest.Test(t, func(t *testing.T) {
			var mu sync.Mutex
			var got []string
			w := newTestWatcher(t, db)
			w.HTTP = &stubHTTPDoer{body: []byte(c.body)}
			w.ACIBaseURL = "http://aci.example"
			w.ACIEnabled = func() bool { return true }
			w.ACIToken = func() string { return "tok" }
			w.PollingInterval = func() time.Duration { return time.Hour }
			w.ReportHealth = func(source, kind, detail string) {
				mu.Lock()
				defer mu.Unlock()
				got = append(got, source+"|"+kind)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { w.Run(ctx); close(done) }()
			synctest.Wait()
			cancel()
			<-done
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, []string{"acinfinity|" + c.kind}, got)
		})
	}
}
