package model

import (
	"database/sql"
	"fmt"
	"strings"

	"isley/logger"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// dbExecutor is the common subset of *sql.DB and *sql.Tx that copyTableData needs.
type dbExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

var conflictKeys = map[string]string{
	"settings":           "id",
	"zones":              "id",
	"sensors":            "id",
	"strain":             "id",
	"plant_status":       "id",
	"plant":              "id",
	"plant_status_log":   "id",
	"metric":             "id",
	"plant_measurements": "id",
	"activity":           "id",
	"activity_metric":    "id",
	"plant_activity":     "id",
	"plant_images":       "id",
	"breeder":            "id",
	"sensor_data":        "id",
	"streams":            "id",
	"strain_lineage":     "id",
	"strain_effect":      "id",
	"strain_flavor":      "id",
	"strain_terpene":     "id",
	"strain_medical_use": "id",
	"sensor_data_hourly": "sensor_id, bucket",
	"plant_day_entry":    "id",
	"smoke_report":       "id",
	"smoke_report_tag":   "id",
	"smoke_report_photo": "id",
}

var boolToIntFields = map[string][]string{
	"strain": {"autoflower"},
}

var orderedTables = []string{
	"settings",
	"zones",
	"breeder", // Must come before strain
	"strain",
	"strain_lineage", // After strain — references strain(id)
	"strain_effect",
	"strain_flavor",
	"strain_terpene",
	"strain_medical_use",
	"sensors",
	"sensor_data",
	"sensor_data_hourly", // hourly averages outlive the raw readings they came from
	// rolling_averages is excluded — it's a trigger-maintained cache (one row per sensor)
	// that rebuilds automatically on the first sensor_data insert after migration.
	"plant_status",
	"plant",
	"plant_status_log",
	"metric",
	"plant_measurements",
	"activity",
	"activity_metric",
	"plant_activity",
	"plant_day_entry",
	"plant_images",
	"streams",
	"smoke_report", // after plant and strain, which it references
	"smoke_report_tag",
	"smoke_report_photo",
}

// seededTables are tables the Postgres migrations fill with starter rows
// (default settings, activities, metrics, stages). The SQLite database had
// the same starter rows, plus the user's changes and additions, so where it
// has rows they replace Postgres's: with ON CONFLICT DO NOTHING the starter
// rows used to win wherever the ids matched, resetting settings to defaults
// and labeling a custom activity as a built-in one. Children first.
var seededTables = []string{"activity_metric", "activity", "metric", "plant_status", "settings"}

// MigrateSqliteToPostgres copies all data from the SQLite database at
// sqlitePath into the PostgreSQL database pg.  The entire operation runs
// inside a single transaction so a failure on any table rolls back all
// previously copied tables, leaving the destination untouched.
func MigrateSqliteToPostgres(sqlitePath string, pg *sql.DB) error {
	sqliteDB, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer sqliteDB.Close()

	tx, err := pg.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() // no-op after Commit

	// Only a fresh Postgres gets its starter rows replaced. Once plants are
	// there (a second run), their activities, measurements and status log
	// point at those rows, and ON CONFLICT keeps what the first run copied.
	var plants int
	if err := tx.QueryRow("SELECT COUNT(*) FROM plant").Scan(&plants); err != nil {
		return fmt.Errorf("count plant: %w", err)
	}
	for _, table := range seededTables {
		if plants > 0 {
			break
		}
		var n int
		if err := sqliteDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&n); err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}
		if n == 0 {
			continue
		}
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("clear starter rows in %s: %w", table, err)
		}
	}

	for _, table := range orderedTables {
		logger.Log.Infof("Migrating table: %s", table)
		if err := copyTableData(sqliteDB, tx, table); err != nil {
			return fmt.Errorf("copy table %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}

	logger.Log.Info("SQLite-to-PostgreSQL migration committed successfully")
	return nil
}

func listSQLiteTables(db *sql.DB) ([]string, error) {
	tables := []string{}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, nil
}

func copyTableData(src *sql.DB, dest dbExecutor, table string) error {
	const batchSize = 5000

	// Disable triggers (PostgreSQL only) to avoid firing during bulk insert
	if IsPostgres() {
		_, err := dest.Exec(fmt.Sprintf("ALTER TABLE %s DISABLE TRIGGER ALL", table))
		if err != nil {
			return fmt.Errorf("disabling triggers on %s: %w", table, err)
		}
	}

	// Count total rows (for logging)
	var totalRows int
	err := src.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&totalRows)
	if err != nil {
		return fmt.Errorf("counting rows in %s: %w", table, err)
	}

	selectCols := "*"
	if IsPostgres() {
		shared, err := sharedColumns(src, dest, table)
		if err != nil {
			return err
		}
		selectCols = strings.Join(shared, ",")
	}

	offset := 0
	for offset < totalRows {
		query := fmt.Sprintf("SELECT %s FROM %s LIMIT %d OFFSET %d", selectCols, table, batchSize, offset)
		rows, err := src.Query(query)
		if err != nil {
			return err
		}

		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return err
		}

		colCount := len(cols)
		valuesList := [][]interface{}{}

		for rows.Next() {
			values := make([]interface{}, colCount)
			ptrs := make([]interface{}, colCount)
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}

			// Normalize booleans that are strings ("true"/"false") to 1/0
			for i, col := range cols {
				for _, bfield := range boolToIntFields[table] {
					if col == bfield {
						if s, ok := values[i].(string); ok {
							if strings.EqualFold(s, "true") {
								values[i] = 1
							} else if strings.EqualFold(s, "false") {
								values[i] = 0
							}
						}
					}
				}
			}

			valuesList = append(valuesList, values)
		}
		rows.Close()

		if len(valuesList) == 0 {
			break
		}

		// Build batch insert
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES ", table, strings.Join(cols, ","))
		args := []interface{}{}
		placeholders := []string{}

		argCounter := 1
		for _, vals := range valuesList {
			ph := []string{}
			for range vals {
				ph = append(ph, fmt.Sprintf("$%d", argCounter))
				argCounter++
			}
			placeholders = append(placeholders, fmt.Sprintf("(%s)", strings.Join(ph, ",")))
			args = append(args, vals...)
		}

		insertSQL += strings.Join(placeholders, ",")

		if conflictKey, ok := conflictKeys[table]; ok {
			insertSQL += fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", conflictKey)
		}

		_, err = dest.Exec(insertSQL, args...)
		if err != nil {
			return fmt.Errorf("executing batch insert into %s: %w", table, err)
		}

		offset += len(valuesList)
		logger.Log.Infof("Migrated %d rows from %s (%d/%d)", len(valuesList), table, offset, totalRows)
	}

	if IsPostgres() && hasSerialID(table) {
		_, err := dest.Exec(fmt.Sprintf(`
        SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE(MAX(id), 1), true) FROM %s
    `, table, table))
		if err != nil {
			return fmt.Errorf("reset sequence for table %s: %w", table, err)
		}
	}

	// Re-enable triggers
	if IsPostgres() {
		_, err := dest.Exec(fmt.Sprintf("ALTER TABLE %s ENABLE TRIGGER ALL", table))
		if err != nil {
			return fmt.Errorf("re-enabling triggers on %s: %w", table, err)
		}
	}

	return nil
}

// sharedColumns lists the table's columns that both databases have. The
// two schemas have drifted (Postgres migration 013 dropped sensors.show,
// for one), and one column only SQLite has used to fail the whole switch.
func sharedColumns(src *sql.DB, dest dbExecutor, table string) ([]string, error) {
	rows, err := dest.Query(`SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1`, table)
	if err != nil {
		return nil, fmt.Errorf("list postgres columns of %s: %w", table, err)
	}
	inDest := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		inDest[strings.ToLower(name)] = true
	}
	rows.Close()

	srcRows, err := src.Query(fmt.Sprintf("SELECT name FROM pragma_table_info('%s')", table))
	if err != nil {
		return nil, fmt.Errorf("list sqlite columns of %s: %w", table, err)
	}
	defer srcRows.Close()
	var shared []string
	for srcRows.Next() {
		var name string
		if err := srcRows.Scan(&name); err != nil {
			return nil, err
		}
		if inDest[strings.ToLower(name)] {
			shared = append(shared, name)
		} else {
			logger.Log.Warnf("Switch to PostgreSQL: %s.%s has no place in PostgreSQL, left out", table, name)
		}
	}
	if len(shared) == 0 {
		return nil, fmt.Errorf("%s: no columns in common", table)
	}
	return shared, nil
}

func hasSerialID(table string) bool {
	// List of tables where 'id' is a SERIAL/identity column and needs sequence reset
	serialTables := map[string]bool{
		"settings":           true,
		"zones":              true,
		"sensors":            true,
		"sensor_data":        true,
		"strain":             true,
		"plant_status":       true,
		"plant":              true,
		"plant_status_log":   true,
		"metric":             true,
		"plant_measurements": true,
		"activity":           true,
		"activity_metric":    true,
		"plant_activity":     true,
		"plant_images":       true,
		"breeder":            true,
		"streams":            true,
		"strain_lineage":     true,
		"strain_effect":      true,
		"strain_flavor":      true,
		"strain_terpene":     true,
		"strain_medical_use": true,
		"plant_day_entry":    true,
		"smoke_report":       true,
		"smoke_report_tag":   true,
		"smoke_report_photo": true,
	}

	return serialTables[table]
}

func IsPostgresEmpty(db *sql.DB) (bool, error) {
	var tablesToCheck = []string{"sensors", "sensor_data", "plant"}

	for _, table := range tablesToCheck {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		err := db.QueryRow(query).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("error checking table %s: %w", table, err)
		}
		if count > 0 {
			return false, nil // Data exists
		}
	}
	return true, nil // No rows found in any key table
}
