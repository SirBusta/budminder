package handlers_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

type parentLink struct {
	Name string
	Link sql.NullInt64
}

func parentLinks(t *testing.T, db *sql.DB, strainID int) []parentLink {
	t.Helper()
	rows, err := db.Query("SELECT parent_name, parent_strain_id FROM strain_lineage WHERE strain_id = $1 ORDER BY id", strainID)
	require.NoError(t, err)
	defer rows.Close()
	var out []parentLink
	for rows.Next() {
		var r parentLink
		require.NoError(t, rows.Scan(&r.Name, &r.Link))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// Data from before 1.0.0 (Isley's CannaDB lookup) can hold a bracketed cross
// split into separate parents; the repair rejoins it and keeps links.
func TestRepairLineageBrackets_RejoinsASplitCross(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	breeder := testutil.SeedBreeder(t, db, "Exotic Genetix")
	mango := testutil.SeedStrain(t, db, breeder, "Mango Runtz")
	white := testutil.SeedStrain(t, db, breeder, "White Runtz")
	chem := testutil.SeedStrain(t, db, breeder, "Creme de la Chem x Mango Smile")
	plain := testutil.SeedStrain(t, db, breeder, "Runtz")
	odd := testutil.SeedStrain(t, db, breeder, "Odd One")
	add := func(strainID int, name string, link any) {
		testutil.MustExec(t, db, `INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, $2, $3)`, strainID, name, link)
	}
	add(mango, "(Creme de la Chem", nil)
	add(mango, "Mango Smile)", nil)
	add(mango, "White Runtz", white)
	add(plain, "Zkittlez", nil)
	add(plain, "Gelato", nil)
	add(odd, "(Never closed", nil) // can't be paired up: left as entered
	add(odd, "Other", nil)

	n, err := handlers.RepairLineageBrackets(db)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	assert.Equal(t, []parentLink{
		{Name: "Creme de la Chem x Mango Smile", Link: sql.NullInt64{Int64: int64(chem), Valid: true}},
		{Name: "White Runtz", Link: sql.NullInt64{Int64: int64(white), Valid: true}},
	}, parentLinks(t, db, mango), "rejoined, linked to the matching strain, the old link kept")
	assert.Equal(t, []parentLink{{Name: "Zkittlez"}, {Name: "Gelato"}}, parentLinks(t, db, plain), "untouched")
	assert.Equal(t, []parentLink{{Name: "(Never closed"}, {Name: "Other"}}, parentLinks(t, db, odd), "untouched")

	n, err = handlers.RepairLineageBrackets(db)
	require.NoError(t, err)
	assert.Zero(t, n, "nothing left to repair on the next start")
}
