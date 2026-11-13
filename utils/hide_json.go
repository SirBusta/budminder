package utils

import (
	"fmt"
	"strings"
)

// HideJSON returns a copy of v (decoded JSON) with account details replaced,
// for the raw-data files owners download and send for support (AC Infinity
// and Vivosun): secrets and personal fields become "<hidden>", device and
// client ids keep only their first 6 characters, and any text that looks
// like an email address is hidden. email is the account's own address (any
// case), hidden wherever it appears. Field names match without case,
// underscores or hyphens. more, when set, is called for every object with
// the original and its hidden copy, so a caller can add a field it derives
// from a hidden one (Vivosun's model token).
func HideJSON(v any, email string, more func(in, out map[string]any)) any {
	return hideJSON(v, strings.ToLower(strings.TrimSpace(email)), more)
}

// HiddenValue replaces whatever HideJSON hides.
const HiddenValue = "<hidden>"

// A field whose name contains one of these is a secret.
var secretMarks = []string{"password", "devpass", "token", "secret", "authorization", "credential",
	"accesskey", "secretkey", "signedurl", "signature", "apikey", "privatekey", "cookie"}

// Personal fields. The dev* names, appemail and zoneid are AC Infinity's
// (zoneId is a time zone there, like devTimeZone).
var privateFields = map[string]bool{"account": true, "accountid": true, "owner": true, "ownerid": true,
	"tenant": true, "tenantid": true, "user": true, "userid": true, "username": true, "email": true,
	"appemail": true, "identity": true, "identityid": true, "devuser": true, "mac": true, "macaddress": true,
	"devmacaddr": true, "ssid": true, "bssid": true, "serial": true, "serialnumber": true, "sn": true,
	"ip": true, "ips": true, "ipaddress": true, "ipaddr": true, "location": true, "gps": true,
	"coordinates": true, "latitude": true, "longitude": true, "lat": true, "lon": true, "lng": true,
	"address": true, "host": true, "url": true, "phone": true, "wifiname": true, "hwid": true,
	"hardwareid": true, "profileurl": true, "fingerprint": true, "devcode": true, "devtimezone": true,
	"timezone": true, "zoneid": true}

// Ids that keep their first 6 characters, so a file can still be matched
// to a device.
var partialIDs = map[string]bool{"deviceid": true, "devid": true, "clientid": true, "topicprefix": true}

// JSONFieldName is how HideJSON compares field names: lower case, without
// underscores or hyphens.
func JSONFieldName(k string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(k))
}

func hideJSON(v any, email string, more func(in, out map[string]any)) any {
	switch t := v.(type) {
	case map[string]any:
		o := make(map[string]any, len(t))
		for k, val := range t {
			n := JSONFieldName(k)
			switch {
			case privateFields[n] || anySecretMark(n):
				o[k] = HiddenValue
			case partialIDs[n]:
				o[k] = partialID(val)
			default:
				o[k] = hideJSON(val, email, more)
			}
		}
		if more != nil {
			more(t, o)
		}
		return o
	case []any:
		o := make([]any, len(t))
		for i := range t {
			o[i] = hideJSON(t[i], email, more)
		}
		return o
	case string:
		l := strings.ToLower(t)
		if (email != "" && strings.Contains(l, email)) || (strings.Contains(t, "@") && strings.Contains(t, ".")) {
			return HiddenValue
		}
		return t
	}
	return v
}

func anySecretMark(n string) bool {
	for _, m := range secretMarks {
		if strings.Contains(n, m) {
			return true
		}
	}
	return false
}

// partialID keeps the first 6 characters of an id given as text or as a
// number (AC Infinity sends devId as a number).
func partialID(v any) string {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case nil:
		return HiddenValue
	default:
		s = strings.TrimSpace(fmt.Sprint(t))
	}
	if len(s) <= 6 {
		return HiddenValue
	}
	return s[:6] + "..."
}
