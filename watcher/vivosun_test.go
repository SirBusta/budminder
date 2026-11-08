package watcher

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
	"isley/vivosun"
)

// vsWatcher polls the fake Vivosun, signed in, with a clock the test moves.
func vsWatcher(t *testing.T, db *sql.DB, vs *fakes.FakeVS, now *time.Time) *Watcher {
	t.Helper()
	w := newTestWatcher(t, db)
	w.VSBaseURL = vs.URL
	w.VSEnabled = func() bool { return true }
	w.VSTokens = func() vivosun.Tokens { return vivosun.Tokens{Access: fakes.VivosunAccess, Login: fakes.VivosunLogin} }
	w.Now = func() time.Time { return *now }
	return w
}

func TestPollVivosun_SavesTheNewestRowOnce(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	vs := fakes.FakeVivosun(t)
	now := time.Now()
	w := vsWatcher(t, db, vs, &now)
	var names map[string]string
	w.VSDeviceNames = func(n map[string]string) { names = n }

	inside := seedSensor(t, db, vivosun.Source, fakes.VivosunDeviceID, "VS.pTemp")
	room := seedSensor(t, db, vivosun.Source, fakes.VivosunDeviceID, "VS.bTemp.out")
	other := seedSensor(t, db, vivosun.Source, "some-other-device", "VS.pTemp")

	older := fakes.VCureRow(now.Add(-2 * time.Minute).Unix())
	older["pTemp"] = 2000
	vs.SetRows(older, fakes.VCureRow(now.Add(-time.Minute).Unix()))

	contacted, err := w.PollVivosun(context.Background())
	require.NoError(t, err)
	assert.True(t, contacted)
	assert.Equal(t, 1, countSensorData(t, db, inside), "only the newest row")
	assert.InDelta(t, 22.5, latestValue(t, db, inside), 0.001)
	assert.InDelta(t, 21.0, latestValue(t, db, room), 0.001, "room air is the b* reading")
	assert.Equal(t, 0, countSensorData(t, db, other), "a device not on the account")
	assert.Equal(t, map[string]string{fakes.VivosunDeviceID: "VCure C80"}, names)

	now = now.Add(time.Minute)
	_, err = w.PollVivosun(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, countSensorData(t, db, inside), "the same row isn't saved twice")

	vs.SetRows(fakes.VCureRow(now.Add(-20 * time.Second).Unix()))
	_, err = w.PollVivosun(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, countSensorData(t, db, inside), "a newer row is saved")
	assert.EqualValues(t, 1, vs.DeviceCalls.Load(), "the device list is kept for 10 minutes, even with a device missing from it")

	seedSensor(t, db, vivosun.Source, "just-added-device", "VS.pTemp")
	_, err = w.PollVivosun(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 2, vs.DeviceCalls.Load(), "a sensor added for a device not in the list fetches it again")
	_, err = w.PollVivosun(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 2, vs.DeviceCalls.Load(), "once")

	now = now.Add(11 * time.Minute)
	_, err = w.PollVivosun(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 3, vs.DeviceCalls.Load(), "refreshed after 10 minutes")
}

func TestPollVivosun_SkipsOldRows(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	vs := fakes.FakeVivosun(t)
	now := time.Now()
	w := vsWatcher(t, db, vs, &now)
	id := seedSensor(t, db, vivosun.Source, fakes.VivosunDeviceID, "VS.pTemp")
	// Vivosun keeps answering with a device's last rows after it goes offline.
	vs.SetRows(fakes.VCureRow(now.Add(-11 * time.Minute).Unix()))

	contacted, err := w.PollVivosun(context.Background())
	require.NoError(t, err)
	assert.True(t, contacted)
	assert.Equal(t, 0, countSensorData(t, db, id), "a row over 10 minutes old means the device stopped sending")
}

func TestPollVivosun_NothingToAsk(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	vs := fakes.FakeVivosun(t)
	now := time.Now()
	w := vsWatcher(t, db, vs, &now)

	contacted, err := w.PollVivosun(context.Background())
	assert.NoError(t, err)
	assert.False(t, contacted, "no Vivosun sensors added")

	seedSensor(t, db, vivosun.Source, fakes.VivosunDeviceID, "VS.pTemp")
	w.VSTokens = func() vivosun.Tokens { return vivosun.Tokens{} }
	contacted, err = w.PollVivosun(context.Background())
	assert.NoError(t, err)
	assert.False(t, contacted, "not signed in")
	assert.Zero(t, vs.DeviceCalls.Load()+vs.ReadingCalls.Load())
}

func TestPollVivosun_SignsInAgainAtMostHourly(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	vs := fakes.FakeVivosun(t)
	now := time.Now()
	w := vsWatcher(t, db, vs, &now)
	id := seedSensor(t, db, vivosun.Source, fakes.VivosunDeviceID, "VS.pTemp")
	relogins := 0
	w.VSRelogin = func(ctx context.Context) (bool, error) {
		relogins++
		_, err := (&vivosun.Client{BaseURL: vs.URL}).Login(ctx, fakes.VivosunEmail, fakes.VivosunPassword)
		return true, err
	}
	rows := func() { vs.SetRows(fakes.VCureRow(now.Add(-30 * time.Second).Unix())) }

	rows()
	vs.Expire()
	contacted, err := w.pollVivosunStayingSignedIn(context.Background())
	require.NoError(t, err, "signed in again and polled")
	assert.True(t, contacted)
	assert.Equal(t, 1, relogins)
	assert.Equal(t, 1, countSensorData(t, db, id))

	now = now.Add(30 * time.Minute)
	rows()
	vs.Expire()
	_, err = w.pollVivosunStayingSignedIn(context.Background())
	var pe *PollError
	require.True(t, errors.As(err, &pe), "%v", err)
	assert.Equal(t, "auth", pe.Kind)
	assert.Equal(t, 1, relogins, "not again within the hour")
	assert.Equal(t, 1, countSensorData(t, db, id))

	now = now.Add(31 * time.Minute)
	rows()
	_, err = w.pollVivosunStayingSignedIn(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, relogins)
	assert.Equal(t, 2, countSensorData(t, db, id))
}

func TestPollVivosun_NoSavedPassword(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	vs := fakes.FakeVivosun(t)
	now := time.Now()
	w := vsWatcher(t, db, vs, &now)
	seedSensor(t, db, vivosun.Source, fakes.VivosunDeviceID, "VS.pTemp")
	w.VSRelogin = func(context.Context) (bool, error) { return false, nil }
	vs.Expire()

	_, err := w.pollVivosunStayingSignedIn(context.Background())
	var pe *PollError
	require.True(t, errors.As(err, &pe), "%v", err)
	assert.Equal(t, "auth", pe.Kind)
	assert.Equal(t, "Token expired, please login again", pe.Detail, "Vivosun's own words reach the card")
	assert.Zero(t, vs.Logins.Load())
}
