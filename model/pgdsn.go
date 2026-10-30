package model

import "strings"

// postgresDSN builds the lib/pq connection string from the BUDMINDER_DB_*
// settings (get is env.Get). Every value is quoted, so passwords with
// spaces, quotes or backslashes, and an empty password, reach PostgreSQL
// as typed. Sessions run in UTC whatever the server's own time zone: readings
// take their time from CURRENT_TIMESTAMP, and everything else reads them as UTC.
func postgresDSN(get func(string) string) string {
	sslMode := get("DB_SSLMODE")
	if sslMode == "" {
		sslMode = "disable"
	}
	parts := []string{
		"host=" + quoteDSNValue(get("DB_HOST")),
		"port=" + quoteDSNValue(get("DB_PORT")),
		"user=" + quoteDSNValue(get("DB_USER")),
		"password=" + quoteDSNValue(get("DB_PASSWORD")),
		"dbname=" + quoteDSNValue(get("DB_NAME")),
		"sslmode=" + quoteDSNValue(sslMode),
		"timezone='UTC'",
	}
	return strings.Join(parts, " ")
}

// quoteDSNValue quotes one key=value value the way libpq reads it: in
// single quotes, with backslashes and single quotes escaped.
func quoteDSNValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `'`, `\'`)
	return "'" + v + "'"
}

// quoteIdent quotes a PostgreSQL identifier (e.g. a database name with
// capitals or hyphens) for statements that can't take it as a parameter.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
