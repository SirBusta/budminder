package handlers

import (
	"database/sql"
	"strings"

	"isley/logger"
)

// RepairLineageBrackets rejoins parent names that an older import saved
// split inside a bracketed cross: "(Creme de la Chem" and "Mango Smile)"
// become the one parent "Creme de la Chem x Mango Smile". BudMinder's own
// imports have saved them joined since 1.0.0 (repairCannadbParentNames); this
// fixes rows brought over from before, such as Isley's CannaDB lookup, which
// otherwise show stray brackets and break the StrainCompass search link.
// Safe to run at every start: it only rewrites a strain whose parent names
// have unbalanced brackets and can be paired up. Returns the strains fixed.
func RepairLineageBrackets(db *sql.DB) (int, error) {
	type row struct {
		name string
		link sql.NullInt64
	}
	rows, err := db.Query("SELECT strain_id, parent_name, parent_strain_id FROM strain_lineage ORDER BY strain_id, id")
	if err != nil {
		return 0, err
	}
	byStrain := map[int][]row{}
	var order []int
	for rows.Next() {
		var id int
		var r row
		if err := rows.Scan(&id, &r.name, &r.link); err != nil {
			rows.Close()
			return 0, err
		}
		if _, ok := byStrain[id]; !ok {
			order = append(order, id)
		}
		byStrain[id] = append(byStrain[id], r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	fixed := 0
	for _, id := range order {
		list := byStrain[id]
		names := make([]string, len(list))
		broken := false
		for i, r := range list {
			names[i] = r.name
			if strings.Count(r.name, "(") != strings.Count(r.name, ")") {
				broken = true
			}
		}
		if !broken {
			continue
		}
		joined := repairCannadbParentNames(names)
		if len(joined) == len(names) {
			continue // the brackets couldn't be paired up; leave it as entered
		}
		if err := replaceLineageNames(db, id, joined, func(name string) (sql.NullInt64, bool) {
			for _, r := range list {
				if r.name == name {
					return r.link, true
				}
			}
			return sql.NullInt64{}, false
		}); err != nil {
			return fixed, err
		}
		fixed++
	}
	return fixed, nil
}

// replaceLineageNames swaps a strain's parent rows for names, keeping the
// link of a name that was already there and linking a new one the usual way
// (the only other local strain with that name).
func replaceLineageNames(db *sql.DB, strainID int, names []string, kept func(string) (sql.NullInt64, bool)) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec("DELETE FROM strain_lineage WHERE strain_id = $1", strainID); err != nil {
		return err
	}
	for _, name := range names {
		var link any
		if l, ok := kept(name); ok {
			if l.Valid {
				link = l.Int64
			}
		} else if link, err = uniqueLocalStrainID(tx, name, strainID); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, $2, $3)",
			strainID, name, link); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// repairRestoredLineage runs RepairLineageBrackets after a restore, since a
// backup can bring the old split names back.
func repairRestoredLineage(db *sql.DB) {
	if n, err := RepairLineageBrackets(db); err != nil {
		logger.Log.WithError(err).Error("Failed to repair split parent strain names after restore")
	} else if n > 0 {
		logger.Log.WithField("strains", n).Info("Repaired parent strain names split inside brackets")
	}
}
