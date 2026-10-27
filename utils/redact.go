package utils

import (
	"errors"
	"net/url"
)

// HideURLSecrets returns err with the query string and any user:password
// removed from the address a failed HTTP request reports. Go's client
// errors read `Post "https://host/path?userId=<token>": ...`, and the AC
// Infinity API takes its token in the query string, so logging the error
// as-is writes the token to the log on every failed poll. Other errors
// come back unchanged.
func HideURLSecrets(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	u, perr := url.Parse(ue.URL)
	if perr != nil {
		return &url.Error{Op: ue.Op, URL: "(address hidden)", Err: ue.Err}
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return &url.Error{Op: ue.Op, URL: u.String(), Err: ue.Err}
}
