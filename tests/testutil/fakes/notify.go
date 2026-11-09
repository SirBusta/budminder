package fakes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// NotifyRequest is one request FakeNotify received.
type NotifyRequest struct {
	Path   string
	Query  string
	Header http.Header
	Body   string
}

// FakeNotify stands in for Pushover, ntfy, Discord, a webhook receiver or Home
// Assistant's notify services: it records every POST and answers with Status
// (200 by default) and Body. Set Status before the code under test runs, or
// with SetStatus.
type FakeNotify struct {
	*httptest.Server
	mu     sync.Mutex
	reqs   []NotifyRequest
	status atomic.Int64
	body   atomic.Value // string
}

// NewFakeNotify starts a FakeNotify, closed when the test ends.
func NewFakeNotify(t *testing.T) *FakeNotify {
	t.Helper()
	f := &FakeNotify{}
	f.status.Store(http.StatusOK)
	f.body.Store(`{"status":1}`)
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, NotifyRequest{Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: string(b)})
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(f.status.Load()))
		_, _ = w.Write([]byte(f.body.Load().(string)))
	}))
	t.Cleanup(f.Server.Close)
	return f
}

// SetStatus sets the answer to the next requests.
func (f *FakeNotify) SetStatus(code int, body string) {
	f.status.Store(int64(code))
	f.body.Store(body)
}

// Requests returns what was received so far.
func (f *FakeNotify) Requests() []NotifyRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]NotifyRequest(nil), f.reqs...)
}
