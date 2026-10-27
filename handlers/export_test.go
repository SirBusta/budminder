package handlers

// Test-only hooks for handlers_test: scan previews need a live AC Infinity
// account or Ecowitt gateway over HTTP, so the tests feed a parsed
// response straight into the same code the scan handlers use.

import (
	"database/sql"
	"isley/model/types"
)

func previewKeys(db *sql.DB, found []foundSensor) []string {
	var out []string
	for _, f := range newSensorsOnly(db, found) {
		out = append(out, f.Source+"|"+f.Device+"|"+f.Type)
	}
	return out
}

// PreviewACIScan is what "Scan AC Infinity" (preview) would list for resp,
// as "source|device|type" keys.
func PreviewACIScan(db *sql.DB, resp types.ACIResponse) []string {
	return previewKeys(db, aciFoundSensors(resp))
}

// PreviewEcowittScan is what "Scan Ecowitt" (preview) would list for a
// gateway at address.
func PreviewEcowittScan(db *sql.DB, address string, resp types.ECWAPIResponse) []string {
	return previewKeys(db, ecowittFoundSensors(address, resp))
}

// InsertBareStrain is the strain "Add to my Strains" on a smoke report
// creates when there's no StrainCompass match.
func InsertBareStrain(db *sql.DB, name string, breederID int) (int, error) {
	return insertBareStrain(db, name, breederID)
}
