package vivosun

import "isley/utils"

// Hide returns a copy of v (decoded JSON) with account details replaced, as
// redaction.py does (rules in utils.HideJSON, shared with the AC Infinity
// file). A clientId's model token is kept as modelToken: it isn't personal
// and tells which model sent the file.
func Hide(v any, email string) any {
	return utils.HideJSON(v, email, func(in, out map[string]any) {
		if _, taken := in["modelToken"]; taken {
			return
		}
		for k, val := range in {
			if utils.JSONFieldName(k) != "clientid" {
				continue
			}
			if m := modelOf(str(val)); m != "" {
				out["modelToken"] = m
			}
			return
		}
	})
}
