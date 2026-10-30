package handlers

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"isley/alerts"
	"isley/logger"
	"isley/model"
)

// BuildArchiveOptions tunes BuildBackupArchive. Production code sets
// these from runtime config; tests construct them by hand.
type BuildArchiveOptions struct {
	// IncludeImages walks UploadsDir and embeds every file under uploads/.
	IncludeImages bool

	// SensorDays controls sensor_data filtering:
	//   0  → include all rows (default)
	//   N  → include only the last N days
	//   -1 → skip the table entirely
	SensorDays int

	// Version is baked into the manifest. Defaults to "unknown" if empty.
	Version string

	// UploadsDir is the directory walked when IncludeImages is true.
	// Defaults to "uploads" relative to the working directory.
	UploadsDir string

	// Now feeds the manifest CreatedAt timestamp. Zero values default
	// to time.Now() so production callers don't have to pass it.
	Now time.Time
}

// BuildBackupArchive dumps the database into a zip archive and returns
// the raw bytes plus the manifest written into it. No files are
// created or modified on disk — callers persist the archive themselves.
//
// This is the unit-testable core of runBackup: it has no goroutines, no
// status mutex, no backupsDir side effect, and it does not depend on
// the model.GetDB() global (db is passed in).
func BuildBackupArchive(db *sql.DB, opts BuildArchiveOptions) ([]byte, BackupManifest, error) {
	fieldLogger := logger.Log.WithField("func", "BuildBackupArchive")

	if db == nil {
		return nil, BackupManifest{}, fmt.Errorf("BuildBackupArchive: db is required")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	uploadsDir := opts.UploadsDir
	if uploadsDir == "" {
		uploadsDir = "uploads"
	}
	version := opts.Version
	if version == "" {
		version = "unknown"
	}

	payload := BackupPayload{}

	tableQueries := []struct {
		name string
		dest *[]map[string]interface{}
	}{
		{"settings", &payload.Settings},
		{"zones", &payload.Zones},
		{"breeder", &payload.Breeders},
		{"sensors", &payload.Sensors},
		{"alert_rule", &payload.AlertRules},
		{"rolling_averages", &payload.RollingAvgs},
		{"plant_status", &payload.PlantStatuses},
		{"climate_target", &payload.ClimateTargets},
		{"strain", &payload.Strains},
		{"strain_lineage", &payload.StrainLineage},
		{"strain_effect", &payload.StrainEffects},
		{"strain_flavor", &payload.StrainFlavors},
		{"strain_terpene", &payload.StrainTerpenes},
		{"strain_medical_use", &payload.StrainMedicalUses},
		{"metric", &payload.Metrics},
		{"activity", &payload.Activities},
		{"activity_metric", &payload.ActivityMetric},
		{"plant", &payload.Plants},
		{"plant_status_log", &payload.PlantStatusLog},
		{"plant_measurements", &payload.PlantMeasure},
		{"plant_activity", &payload.PlantActivity},
		{"plant_day_entry", &payload.PlantDayEntry},
		{"plant_images", &payload.PlantImages},
		{"streams", &payload.Streams},
		{"smoke_report", &payload.SmokeReports},
		{"smoke_report_tag", &payload.SmokeReportTags},
		{"smoke_report_photo", &payload.SmokeReportPhoto},
	}

	tableCount := 0
	for _, tq := range tableQueries {
		rows, err := dumpTable(db, tq.name)
		if err != nil {
			return nil, BackupManifest{}, fmt.Errorf("dump %s: %w", tq.name, err)
		}
		if tq.name == "settings" {
			rows = withoutSecretSettings(rows)
		}
		*tq.dest = rows
		if len(rows) > 0 {
			tableCount++
		}
	}

	if opts.SensorDays != -1 {
		var sensorRows []map[string]interface{}
		var err error
		if opts.SensorDays == 0 {
			sensorRows, err = dumpTable(db, "sensor_data")
		} else {
			sensorRows, err = dumpTableFiltered(db, "sensor_data", opts.SensorDays)
		}
		if err != nil {
			return nil, BackupManifest{}, fmt.Errorf("dump sensor_data: %w", err)
		}
		payload.SensorData = sensorRows
		if len(sensorRows) > 0 {
			tableCount++
		}
	}

	fileCount := 0
	if opts.IncludeImages {
		_ = filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				fileCount++
			}
			return nil
		})
	}

	payload.Manifest = BackupManifest{
		Version:       version,
		Driver:        model.GetDriver(),
		CreatedAt:     now.UTC().Format(time.RFC3339),
		Tables:        tableCount,
		Files:         fileCount,
		IncludeImages: opts.IncludeImages,
		SensorDays:    opts.SensorDays,
	}

	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, BackupManifest{}, fmt.Errorf("marshal JSON: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	jw, err := zw.Create("backup.json")
	if err != nil {
		return nil, BackupManifest{}, fmt.Errorf("create backup.json: %w", err)
	}
	if _, err := jw.Write(jsonData); err != nil {
		return nil, BackupManifest{}, fmt.Errorf("write backup.json: %w", err)
	}

	if opts.IncludeImages {
		err := filepath.Walk(uploadsDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return walkErr
			}
			fw, err := zw.Create(path)
			if err != nil {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(fw, f)
			return err
		})
		if err != nil {
			return nil, BackupManifest{}, fmt.Errorf("add uploads: %w", err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, BackupManifest{}, fmt.Errorf("finalize zip: %w", err)
	}

	fieldLogger.Infof("Built backup archive: %d bytes, %d tables, %d files", buf.Len(), tableCount, fileCount)
	return buf.Bytes(), payload.Manifest, nil
}

// ParseBackupArchive reads a zip archive produced by BuildBackupArchive
// and returns the parsed BackupPayload. Returns a wrapped error if the
// bytes are not a valid zip, lack a backup.json entry, or contain
// malformed JSON.
func ParseBackupArchive(archive []byte) (BackupPayload, error) {
	if len(archive) == 0 {
		return BackupPayload{}, fmt.Errorf("ParseBackupArchive: archive is empty")
	}

	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return BackupPayload{}, fmt.Errorf("not a valid zip: %w", err)
	}

	for _, zf := range zr.File {
		if zf.Name != "backup.json" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return BackupPayload{}, fmt.Errorf("open backup.json: %w", err)
		}
		defer rc.Close()

		var payload BackupPayload
		dec := json.NewDecoder(rc)
		dec.UseNumber() // preserve int vs float fidelity
		if err := dec.Decode(&payload); err != nil {
			return BackupPayload{}, fmt.Errorf("decode backup.json: %w", err)
		}
		return payload, nil
	}
	return BackupPayload{}, fmt.Errorf("backup.json not found in archive")
}

// secretSettings are settings rows a backup leaves out. session_secret signs
// login cookies: anyone holding a backup could otherwise make a valid
// sign-in cookie for the live install. A restore without it keeps the
// current key until the next start, which then makes a new one. The mail
// password (Settings > Alerts) is kept through a restore (keepMailPassword).
var secretSettings = map[string]bool{"session_secret": true, alerts.SettingEmailPassword: true}

func withoutSecretSettings(rows []map[string]interface{}) []map[string]interface{} {
	kept := rows[:0]
	for _, r := range rows {
		name, _ := r["name"].(string)
		if b, ok := r["name"].([]byte); ok {
			name = string(b)
		}
		if !secretSettings[name] {
			kept = append(kept, r)
		}
	}
	return kept
}
