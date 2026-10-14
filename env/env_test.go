package env

// +parallel:serial — t.Setenv changes the process environment and cannot run
// alongside parallel tests.

import "testing"

func TestGet_PrefersBudMinderName(t *testing.T) {
	t.Setenv("BUDMINDER_ZZ_TEST", "new")
	t.Setenv("ISLEY_ZZ_TEST", "old")
	if got := Get("ZZ_TEST"); got != "new" {
		t.Fatalf("Get = %q, want new", got)
	}
}

func TestGet_FallsBackToIsleyName(t *testing.T) {
	t.Setenv("ISLEY_ZZ_TEST", "old")
	if got := Get("ZZ_TEST"); got != "old" {
		t.Fatalf("Get = %q, want old", got)
	}
}

func TestGet_EmptyBudMinderNameFallsBack(t *testing.T) {
	t.Setenv("BUDMINDER_ZZ_TEST", "")
	t.Setenv("ISLEY_ZZ_TEST", "old")
	if got := Get("ZZ_TEST"); got != "old" {
		t.Fatalf("Get = %q, want old", got)
	}
}

func TestLookup_NeitherSet(t *testing.T) {
	if v, ok := Lookup("ZZ_TEST_NEVER_SET"); ok || v != "" {
		t.Fatalf("Lookup = %q, %v; want empty, false", v, ok)
	}
}
