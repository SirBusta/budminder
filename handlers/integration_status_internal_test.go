package handlers

import (
	"testing"
	"time"

	"isley/config"
)

func TestIntegrationState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	stale := 5 * time.Minute
	recent, old := now.Add(-time.Minute), now.Add(-32*24*time.Hour)
	okH := config.IntegrationHealth{LastOK: now.Add(-30 * time.Second)}
	authH := config.IntegrationHealth{LastOK: old, LastErr: now.Add(-20 * time.Second), ErrKind: "auth", ErrDetail: "100001 token invalid"}
	downH := config.IntegrationHealth{LastErr: now.Add(-20 * time.Second), ErrKind: "unreachable"}
	recoveredH := config.IntegrationHealth{LastErr: now.Add(-time.Hour), ErrKind: "auth", LastOK: now.Add(-20 * time.Second)}

	cases := []struct {
		name                string
		enabled, configured bool
		h                   config.IntegrationHealth
		newest              time.Time
		hasReadings         bool
		want                string
	}{
		{"switched off", false, true, okH, recent, true, "off"},
		{"no token or address", true, false, okH, recent, true, "setup"},
		{"token refused", true, true, authH, old, true, "auth"},
		{"can't connect", true, true, downH, recent, true, "failing"},
		// The bug this fixes: on + token saved, but nothing recorded for a month.
		{"answers but no new readings", true, true, okH, old, true, "stale"},
		{"just set up", true, true, config.IntegrationHealth{}, time.Time{}, false, "waiting"},
		{"working", true, true, okH, recent, true, "ok"},
		{"failed earlier, works again", true, true, recoveredH, recent, true, "ok"},
	}
	for _, c := range cases {
		got := integrationState(c.enabled, c.configured, c.h, c.newest, c.hasReadings, stale, now)
		if got.State != c.want {
			t.Errorf("%s: state %q, want %q", c.name, got.State, c.want)
		}
	}
	got := integrationState(true, true, authH, old, true, stale, now)
	if got.Detail != "100001 token invalid" || got.Since != old.UnixMilli() {
		t.Errorf("auth detail/since = %q/%d, want the service's message and the last reading", got.Detail, got.Since)
	}
}
