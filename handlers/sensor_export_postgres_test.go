//go:build integration_postgres

// Export readings on Postgres: the counts use EXTRACT(EPOCH ...) there and
// strftime on SQLite, and the hourly table's bucket is a real timestamp.
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./handlers/...

package handlers_test

import (
	"testing"

	"isley/tests/testutil"
)

func TestSensorExport_Postgres(t *testing.T) {
	t.Parallel()
	runExportChecks(t, newSensorEnvOn(t, testutil.NewTestPostgresDB(t)))
}
