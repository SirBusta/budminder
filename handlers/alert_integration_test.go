package handlers_test

// Integration alerts (alertRun.integrations): an integration that refuses
// BudMinder's sign-in or doesn't answer for 15 minutes.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/alerts"
	"isley/tests/testutil/fakes"
)

// integrationAlert returns the integration alert among events, or nil.
func integrationAlert(ev []alerts.Event) *alerts.Event {
	for i := range ev {
		if ev[i].Kind == alerts.KindIntegration {
			return &ev[i]
		}
	}
	return nil
}

// withACI turns AC Infinity on with a saved token (the alert env's sensors
// are AC Infinity sensors).
func (a *alertEnv) withACI() {
	a.engine.Store.SetACIEnabled(1)
	a.engine.Store.SetACIToken("token")
}

func TestAlertEngine_IntegrationRefusedSignIn(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.withACI()
	store := a.engine.Store
	now := time.Now()

	// Refused for 10 minutes: not yet.
	store.ReportIntegration("acinfinity", "auth", "401", now.Add(-10*time.Minute))
	store.ReportIntegration("acinfinity", "auth", "401", now)
	a.check()
	assert.Nil(t, integrationAlert(a.events(t)))

	// Refused for 16 minutes: one email, timed from the first refusal.
	a2 := newAlertEnv(t)
	a2.mail(t, f)
	a2.withACI()
	a2.engine.Store.ReportIntegration("acinfinity", "auth", "401", now.Add(-16*time.Minute))
	a2.engine.Store.ReportIntegration("acinfinity", "auth", "401", now)
	a2.check()
	a2.send()
	ev := integrationAlert(a2.events(t))
	require.NotNil(t, ev)
	assert.Equal(t, "acinfinity", ev.Measure)
	assert.Equal(t, alerts.IntegrationAuth, ev.Detail)
	assert.Nil(t, ev.Value)
	assert.WithinDuration(t, now.Add(-16*time.Minute), ev.Started, 2*time.Second)
	assert.Equal(t, alerts.Critical, alerts.Severity(ev.Kind))
	require.Len(t, f.Mail(), 1)
	assert.Contains(t, f.Mail()[0].Data, "AC Infinity sign-in refused")

	// Not answering now: the same alert, no second email.
	a2.engine.Store.ReportIntegration("acinfinity", "unreachable", "timeout", now)
	a2.check()
	a2.send()
	ev = integrationAlert(a2.events(t))
	assert.Equal(t, alerts.IntegrationFailing, ev.Detail)
	assert.Nil(t, ev.Resolved)
	assert.Len(t, f.Mail(), 1)

	// One poll works: still open until 30 minutes without a failure.
	a2.engine.Store.ReportIntegration("acinfinity", "", "", now.Add(time.Second))
	a2.check()
	assert.Nil(t, integrationAlert(a2.events(t)).Resolved)

	a2.engine.Now = func() time.Time { return now.Add(31 * time.Minute) }
	a2.check()
	a2.send()
	assert.NotNil(t, integrationAlert(a2.events(t)).Resolved)
	require.Len(t, f.Mail(), 2)
	assert.Contains(t, f.Mail()[1].Data, "AC Infinity working again")
}

func TestAlertEngine_IntegrationSwitchedOffClosesQuietly(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	f := fakes.NewFakeSMTP(t, fakes.SMTPPlain)
	a.mail(t, f)
	a.withACI()
	now := time.Now()
	a.engine.Store.ReportIntegration("acinfinity", "unreachable", "", now.Add(-20*time.Minute))
	a.check()
	a.send()
	require.NotNil(t, integrationAlert(a.events(t)))
	require.Len(t, f.Mail(), 1)
	assert.Contains(t, f.Mail()[0].Data, "AC Infinity not responding")

	a.engine.Store.SetACIEnabled(0)
	a.check()
	a.send()
	assert.NotNil(t, integrationAlert(a.events(t)).Resolved)
	assert.Len(t, f.Mail(), 1, "switched off: no all-clear")
}

func TestAlertEngine_IntegrationWithoutSensorsIsNotChecked(t *testing.T) {
	t.Parallel()
	a := newAlertEnv(t)
	a.engine.Store.SetVSEnabled(1)
	a.engine.Store.SetVSTokens("access", "login")
	a.engine.Store.ReportIntegration("vivosun", "auth", "", time.Now().Add(-time.Hour))
	a.check()
	assert.Nil(t, integrationAlert(a.events(t)), "no Vivosun sensors added")

	// Off, or on without a token: not checked either.
	a.engine.Store.ReportIntegration("acinfinity", "auth", "", time.Now().Add(-time.Hour))
	a.check()
	assert.Nil(t, integrationAlert(a.events(t)), "AC Infinity is off")
}

func TestAlertEngine_IntegrationRowHasNoChart(t *testing.T) {
	t.Parallel()
	assert.False(t, alerts.HasChart(alerts.KindIntegration))
	assert.False(t, alerts.HasChart(alerts.KindClock))
	assert.True(t, alerts.HasChart(alerts.KindOffline))
}
