package handlers

// Honest status for the sensor integrations (AC Infinity, Ecowitt, Home
// Assistant): based on how the last polls went and when readings last
// arrived, not just on whether the integration is switched on and has a
// token saved. Before 1.0.1 a dead AC Infinity token still showed
// "Connected" while nothing was recorded for weeks.

import (
	"database/sql"
	"time"

	"isley/config"
	"isley/homeassistant"
)

// IntegrationStatus is one integration's state for the pages:
//
//	off      switched off
//	setup    switched on but not set up (no token / address / gateway)
//	waiting  set up, no answer and no readings yet
//	ok       the last poll worked and readings are arriving
//	auth     the service refused the sign-in or token (sign in again)
//	failing  the last poll failed for another reason (can't connect, …)
//	stale    polls answer, but no new readings for a while
type IntegrationStatus struct {
	State  string `json:"state"`
	Since  int64  `json:"since,omitempty"`  // ms: when readings stopped, or the failure
	Detail string `json:"detail,omitempty"` // the service's own message
}

// integrationSources lists the integrations in display order.
var integrationSources = []string{"acinfinity", "ecowitt", homeassistant.Source}

// IntegrationStatuses works out every integration's status.
func IntegrationStatuses(db *sql.DB, store *config.Store) map[string]IntegrationStatus {
	out := map[string]IntegrationStatus{}
	for _, src := range integrationSources {
		var enabled, configured bool
		switch src {
		case "acinfinity":
			enabled, configured = store.ACIEnabled() == 1, store.ACIToken() != ""
		case "ecowitt":
			enabled, configured = store.ECEnabled() == 1, len(store.ECDevices()) > 0
		case homeassistant.Source:
			enabled, configured = store.HAEnabled() == 1, store.HAURL() != "" && store.HAToken() != ""
		}
		newest, hasReadings := newestReading(db, src)
		out[src] = integrationState(enabled, configured, store.IntegrationHealth(src), newest, hasReadings,
			staleAfter(store.PollingInterval()), time.Now())
	}
	return out
}

// integrationState is the decision itself, separate so it can be tested
// without a database.
func integrationState(enabled, configured bool, h config.IntegrationHealth, newest time.Time, hasReadings bool,
	staleAfter time.Duration, now time.Time) IntegrationStatus {
	switch {
	case !enabled:
		return IntegrationStatus{State: "off"}
	case !configured:
		return IntegrationStatus{State: "setup"}
	case h.Failing() && h.ErrKind == "auth":
		return IntegrationStatus{State: "auth", Since: lastGood(h, newest, hasReadings), Detail: h.ErrDetail}
	case h.Failing():
		return IntegrationStatus{State: "failing", Since: lastGood(h, newest, hasReadings), Detail: h.ErrDetail}
	case hasReadings && now.Sub(newest) > staleAfter:
		return IntegrationStatus{State: "stale", Since: newest.UnixMilli()}
	case h.LastOK.IsZero() && !hasReadings:
		return IntegrationStatus{State: "waiting"}
	}
	return IntegrationStatus{State: "ok"}
}

// lastGood is when things were last fine: the newest reading, or else the
// last successful poll (0 if neither is known).
func lastGood(h config.IntegrationHealth, newest time.Time, hasReadings bool) int64 {
	if hasReadings {
		return newest.UnixMilli()
	}
	if !h.LastOK.IsZero() {
		return h.LastOK.UnixMilli()
	}
	return 0
}

// newestReading is the time of the newest reading from any of a source's
// sensors (each sensor's newest via its index, then the latest of those).
func newestReading(db *sql.DB, source string) (time.Time, bool) {
	var at sql.NullTime
	err := db.QueryRow(`
		SELECT sd.create_dt FROM sensor_data sd
		WHERE sd.id = (SELECT MAX((SELECT MAX(id) FROM sensor_data WHERE sensor_id = s.id)) FROM sensors s WHERE s.source = $1)`,
		source).Scan(&at)
	if err != nil || !at.Valid {
		return time.Time{}, false
	}
	return at.Time, true
}
