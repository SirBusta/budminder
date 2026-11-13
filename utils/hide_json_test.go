package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHideJSON_SecretsAndPersonalFields(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name": "Tent fan", "temperature": 2450,
		"user_id": "u-42", "Access-Token": "tok", "apiKey": "k", "SESSION_COOKIE": "c",
		"wifiName": "home-net", "ipAddress": "192.168.1.9", "timeZone": "America/Chicago",
		"note": "Owner@Example.com", "list": []any{map[string]any{"password": "pw", "label": "kept"}},
		"devId": json.Number("1234567890123"), "deviceId": "abc", "clientId": nil,
	}
	out := HideJSON(in, "OWNER@example.com ", nil)
	b, err := json.Marshal(out)
	require.NoError(t, err)
	s := string(b)
	for _, bad := range []string{"u-42", `"tok"`, `"k"`, `"c"`, "home-net", "192.168", "Chicago", "xample.com", `"pw"`, "1234567890123", `"abc"`} {
		assert.NotContains(t, s, bad)
	}
	for _, good := range []string{"Tent fan", "2450", "kept", `"devId":"123456..."`} {
		assert.Contains(t, s, good)
	}
	m := out.(map[string]any)
	assert.Equal(t, HiddenValue, m["deviceId"], "an id of 6 characters or fewer is hidden whole")
	assert.Equal(t, HiddenValue, m["clientId"], "a missing id is hidden")
}

func TestHideJSON_MoreSeesOriginal(t *testing.T) {
	t.Parallel()
	out := HideJSON(map[string]any{"clientId": "secret-id-1"}, "", func(in, out map[string]any) {
		out["seen"] = in["clientId"]
	})
	m := out.(map[string]any)
	assert.Equal(t, "secret-id-1", m["seen"], "more gets the unhidden original")
	assert.True(t, strings.HasSuffix(m["clientId"].(string), "..."))
}
