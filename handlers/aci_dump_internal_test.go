package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The raw AC Infinity file always hides account details, with the Vivosun
// file's rules (GET /sensors/dumpACI has no way to turn this off).
func TestHideACIDump_HidesAccountDetails(t *testing.T) {
	t.Parallel()
	body := []byte(`{"code":200,"data":[{"devId":1424979258063355000,"devCode":"ABCD1234","devName":"Grow Tent",
		"devMacAddr":"34:94:54:AA:BB:CC","appEmail":"grower@example.com","devTimeZone":"America/Los_Angeles",
		"zoneId":"America/Los_Angeles","wifiName":"MyHomeWifi","userId":"9876543","deviceInfo":{"temperature":2450,
		"humidity":5520,"ports":[{"port":1,"portName":"Fan","speak":5}]}}]}`)
	out, err := hideACIDump(body, "Grower@Example.com")
	require.NoError(t, err)
	s := string(out)
	for _, bad := range []string{"1424979258063355000", "ABCD1234", "34:94:54", "grower@", "Los_Angeles", "MyHomeWifi", "9876543"} {
		assert.NotContains(t, s, bad)
	}
	for _, good := range []string{`"Grow Tent"`, "2450", "5520", `"Fan"`, `"142497..."`} {
		assert.Contains(t, s, good)
	}
}

// An answer that isn't JSON is refused, never passed on unhidden.
func TestHideACIDump_RefusesNonJSON(t *testing.T) {
	t.Parallel()
	_, err := hideACIDump([]byte("<html>grower@example.com</html>"), "")
	assert.Error(t, err)
}
