package alerts_test

// Text for integration alerts (message.go).

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"isley/alerts"
)

func TestIntegrationAlertText(t *testing.T) {
	t.Parallel()
	w := alerts.Words{T: func(key string) string { return key + ":{name}" }}
	ev := func(source, detail string) alerts.Event {
		return alerts.Event{Kind: alerts.KindIntegration, Measure: source, Detail: detail}
	}
	cases := []struct {
		e        alerts.Event
		resolved bool
		title    string
		line     string
	}{
		{ev("acinfinity", alerts.IntegrationAuth), false, "al_ev_int_auth:AC Infinity", "al_line_int_auth:AC Infinity"},
		{ev("vivosun", alerts.IntegrationAuth), false, "al_ev_int_auth:Vivosun", "al_line_int_auth:Vivosun"},
		{ev("homeassistant", alerts.IntegrationAuth), false, "al_ev_int_token:Home Assistant", "al_line_int_token:Home Assistant"},
		{ev("ecowitt", alerts.IntegrationFailing), false, "al_ev_int_failing:Ecowitt", "al_line_int_failing:Ecowitt"},
		{ev("vivosun", alerts.IntegrationFailing), true, "al_ev_int_ok:Vivosun", "al_line_int_ok:Vivosun"},
	}
	for _, c := range cases {
		assert.Equal(t, c.title, w.Title(c.e, c.resolved))
		assert.Equal(t, c.line, w.Line(c.e, c.resolved))
	}
}
