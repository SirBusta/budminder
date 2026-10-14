// Package env reads BudMinder's startup settings (environment variables).
//
// Each setting is read as BUDMINDER_<NAME>, falling back to ISLEY_<NAME>, the
// name used by Isley (the project BudMinder is based on), so containers set
// up with the older names keep working. A BUDMINDER_ variable that is set but
// empty also falls back, since compose files often pass "${VAR:-}" through.
package env

import "os"

const (
	Prefix       = "BUDMINDER_"
	LegacyPrefix = "ISLEY_"
)

// Lookup returns the setting's value and whether either name is set.
func Lookup(name string) (string, bool) {
	if v, ok := os.LookupEnv(Prefix + name); ok && v != "" {
		return v, true
	}
	return os.LookupEnv(LegacyPrefix + name)
}

// Get returns the setting's value, or "" when neither name is set.
func Get(name string) string {
	v, _ := Lookup(name)
	return v
}
