package vivosun

import "strings"

// Hide returns a copy of v (decoded JSON) with account details replaced, as
// redaction.py does: secrets and personal fields become "<hidden>", device
// and client ids keep only their first 6 characters, and any text that
// looks like an email address is hidden. email is the account's own address
// (any case), hidden wherever it appears.
func Hide(v any, email string) any {
	return hide(v, strings.ToLower(strings.TrimSpace(email)))
}

var secretMarks = []string{"password", "devpass", "token", "secret", "authorization", "credential",
	"accesskey", "secretkey", "signedurl", "signature", "apikey", "privatekey", "cookie"}

var privateFields = map[string]bool{"account": true, "accountid": true, "owner": true, "ownerid": true,
	"tenant": true, "tenantid": true, "user": true, "userid": true, "username": true, "email": true,
	"identity": true, "identityid": true, "devuser": true, "mac": true, "macaddress": true, "ssid": true,
	"bssid": true, "serial": true, "serialnumber": true, "sn": true, "ip": true, "ips": true,
	"ipaddress": true, "ipaddr": true, "location": true, "gps": true, "coordinates": true,
	"latitude": true, "longitude": true, "lat": true, "lon": true, "lng": true, "address": true,
	"host": true, "url": true, "phone": true, "wifiname": true, "hwid": true, "hardwareid": true,
	"profileurl": true, "fingerprint": true}

var partialIDs = map[string]bool{"deviceid": true, "clientid": true, "topicprefix": true}

const hidden = "<hidden>"

func hide(v any, email string) any {
	switch t := v.(type) {
	case map[string]any:
		o := make(map[string]any, len(t))
		for k, val := range t {
			n := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(k))
			switch {
			case privateFields[n] || anyMark(n):
				o[k] = hidden
			case partialIDs[n]:
				o[k] = partial(str(val))
			default:
				o[k] = hide(val, email)
			}
		}
		return o
	case []any:
		o := make([]any, len(t))
		for i := range t {
			o[i] = hide(t[i], email)
		}
		return o
	case string:
		l := strings.ToLower(t)
		if (email != "" && strings.Contains(l, email)) || (strings.Contains(t, "@") && strings.Contains(t, ".")) {
			return hidden
		}
		return t
	}
	return v
}

func anyMark(n string) bool {
	for _, m := range secretMarks {
		if strings.Contains(n, m) {
			return true
		}
	}
	return false
}

func partial(s string) string {
	if len(s) <= 6 {
		return hidden
	}
	return s[:6] + "..."
}
