package handlers

import (
	"database/sql"
	"strings"
	"time"

	"isley/logger"
	"isley/model/types"
	"isley/utils"
)

// Seed Vault packs (table strain_pack). Every pack added to a strain's seeds
// is a row: the first one and each restock, with what it cost. The Seed Vault
// counts the packs the seeds on hand come from, so seeds put back in stock
// from zero mark the strain's earlier packs used up. The Strains page scales
// each strain's Cost and Value by the seeds left (strains-page.js).

// strainPack is a pack as sent by a form or the Restock window.
type strainPack struct {
	PriceCents *int   `json:"price_cents"`
	Free       bool   `json:"free"`
	AddedOn    string `json:"added_on"` // the Restock window's date; today when empty
}

// maxPackCents caps a pack price at $100,000.
const maxPackCents = 10_000_000

func (p *strainPack) valid() bool {
	if p == nil {
		return true
	}
	if p.PriceCents != nil && (*p.PriceCents < 0 || *p.PriceCents > maxPackCents) {
		return false
	}
	if d := strings.TrimSpace(p.AddedOn); d != "" {
		if _, err := time.Parse(utils.LayoutDate, d); err != nil {
			return false
		}
	}
	return true
}

func (p strainPack) empty() bool { return p.PriceCents == nil && !p.Free }

func insertPack(tx *sql.Tx, strainID, seeds int, p strainPack, date string) error {
	var price any
	if p.PriceCents != nil {
		price = *p.PriceCents
	}
	free := 0
	if p.Free {
		free = 1
	}
	_, err := tx.Exec("INSERT INTO strain_pack (strain_id, seeds, price_cents, free, added_on) VALUES ($1, $2, $3, $4, $5)",
		strainID, seeds, price, free, date)
	return err
}

// retirePacks marks a strain's packs used up: its seeds ran out before this
// new pack.
func retirePacks(tx *sql.Tx, strainID int) error {
	_, err := tx.Exec("UPDATE strain_pack SET used_up = 1 WHERE strain_id = $1 AND used_up = 0", strainID)
	return err
}

// newestPack is the id of the strain's newest pack still in use.
func newestPack(tx *sql.Tx, strainID int) (int, bool, error) {
	var id sql.NullInt64
	err := tx.QueryRow("SELECT MAX(id) FROM strain_pack WHERE strain_id = $1 AND used_up = 0", strainID).Scan(&id)
	return int(id.Int64), id.Valid, err
}

// addPackSeeds records seeds added on the Strains page. With a pack (the
// Restock and Add seeds windows) they're a new pack; without one (the + on a
// card) they join the newest pack.
func addPackSeeds(tx *sql.Tx, strainID, prevCount, added int, pack *strainPack, today string) error {
	if pack == nil && prevCount > 0 {
		_, err := tx.Exec(`UPDATE strain_pack SET seeds = seeds + $1
			WHERE id = (SELECT MAX(id) FROM strain_pack WHERE strain_id = $2 AND used_up = 0)`, added, strainID)
		return err
	}
	if prevCount <= 0 {
		if err := retirePacks(tx, strainID); err != nil {
			return err
		}
	}
	p := strainPack{}
	if pack != nil {
		p = *pack
	}
	date := strings.TrimSpace(p.AddedOn)
	if date == "" {
		date = today
	}
	return insertPack(tx, strainID, added, p, date)
}

// saveFormPack stores the pack fields of a strain form (Add/Edit strain, the
// import review). Seeds put in stock from zero there are a new pack, dated
// like the seeds (addedOn: the resolved seeds_added_on). Otherwise the form
// edits the newest pack, or starts one for seeds that have none yet (stocked
// before packs were recorded). pack nil = not sent: nothing changes.
func saveFormPack(tx *sql.Tx, strainID int, pack *strainPack, prevCount, seedCount int, addedOn any, today string) error {
	if seedCount <= 0 {
		return nil
	}
	date := today
	if d, ok := addedOn.(string); ok && d != "" {
		date = d
	}
	p := strainPack{}
	if pack != nil {
		p = *pack
	}
	if prevCount <= 0 {
		if err := retirePacks(tx, strainID); err != nil {
			return err
		}
		return insertPack(tx, strainID, seedCount, p, date)
	}
	if pack == nil {
		return nil
	}
	id, ok, err := newestPack(tx, strainID)
	if err != nil {
		return err
	}
	if !ok {
		if p.empty() {
			return nil
		}
		return insertPack(tx, strainID, seedCount, p, date)
	}
	var price any
	if p.PriceCents != nil {
		price = *p.PriceCents
	}
	free := 0
	if p.Free {
		free = 1
	}
	_, err = tx.Exec("UPDATE strain_pack SET price_cents = $1, free = $2 WHERE id = $3", price, free, id)
	return err
}

// currentPacks reads the packs in use, by strain id, oldest first; for one
// strain when strainID > 0, else for all.
func currentPacks(db *sql.DB, strainID int) map[int][]types.StrainPack {
	query := "SELECT id, strain_id, seeds, price_cents, free, added_on FROM strain_pack WHERE used_up = 0"
	var args []any
	if strainID > 0 {
		query += " AND strain_id = $1"
		args = append(args, strainID)
	}
	rows, err := db.Query(query+" ORDER BY id", args...)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read seed packs")
		return nil
	}
	defer rows.Close()
	out := map[int][]types.StrainPack{}
	for rows.Next() {
		var p types.StrainPack
		var sid, free int
		var price sql.NullInt64
		if err := rows.Scan(&p.ID, &sid, &p.Seeds, &price, &free, &p.AddedOn); err != nil {
			logger.Log.WithError(err).Error("Failed to read a seed pack")
			return nil
		}
		if price.Valid {
			c := int(price.Int64)
			p.PriceCents = &c
		}
		p.Free = free == 1
		out[sid] = append(out[sid], p)
	}
	if err := rows.Err(); err != nil {
		logger.Log.WithError(err).Error("Failed to read seed packs")
	}
	return out
}
