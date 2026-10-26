//go:build !windows

package launcher

import "errors"

// AllowFirewall: only Windows for now.
func AllowFirewall(string) error { return errors.New("only on Windows") }
