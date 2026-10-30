package model

import (
	"testing"

	"github.com/lib/pq"
)

func TestPostgresDSN_QuotesEveryValue(t *testing.T) {
	t.Parallel()
	vals := map[string]string{
		"DB_HOST": "db", "DB_PORT": "5432", "DB_USER": "isley",
		"DB_PASSWORD": `it's a \pass`, "DB_NAME": "Bud-Minder",
	}
	got := postgresDSN(func(k string) string { return vals[k] })
	want := `host='db' port='5432' user='isley' password='it\'s a \\pass' dbname='Bud-Minder' sslmode='disable' timezone='UTC'`
	if got != want {
		t.Fatalf("dsn:\n got %s\nwant %s", got, want)
	}
	if _, err := pq.NewConnector(got); err != nil {
		t.Fatalf("lib/pq can't read the dsn: %v", err)
	}
}

func TestPostgresDSN_EmptyPassword(t *testing.T) {
	t.Parallel()
	vals := map[string]string{"DB_HOST": "db", "DB_PORT": "5432", "DB_USER": "u", "DB_NAME": "n", "DB_SSLMODE": "require"}
	got := postgresDSN(func(k string) string { return vals[k] })
	want := `host='db' port='5432' user='u' password='' dbname='n' sslmode='require' timezone='UTC'`
	if got != want {
		t.Fatalf("dsn:\n got %s\nwant %s", got, want)
	}
	if _, err := pq.NewConnector(got); err != nil {
		t.Fatalf("lib/pq can't read the dsn: %v", err)
	}
}

func TestQuoteIdent(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"isleydb": `"isleydb"`, "Bud-Minder": `"Bud-Minder"`, `a"b`: `"a""b"`} {
		if got := quoteIdent(in); got != want {
			t.Errorf("quoteIdent(%q) = %s, want %s", in, got, want)
		}
	}
}
