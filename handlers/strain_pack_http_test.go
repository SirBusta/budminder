package handlers_test

// Seed Vault packs through the real API: one row per pack added (the first
// one and each restock), the + on a card, edits to the newest pack, and a
// restock from zero retiring the old packs (see handlers/strain_pack.go).

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type packRow struct {
	Seeds   int
	Price   sql.NullInt64
	Free    bool
	AddedOn string
	UsedUp  bool
}

// packs lists the strain's pack rows, oldest first.
func (e *strainEditEnv) packs(t *testing.T) []packRow {
	t.Helper()
	rows, err := e.db.Query(`SELECT seeds, price_cents, free, added_on, used_up FROM strain_pack WHERE strain_id = $1 ORDER BY id`, e.strainID)
	require.NoError(t, err)
	defer rows.Close()
	var out []packRow
	for rows.Next() {
		var r packRow
		var free, used int
		require.NoError(t, rows.Scan(&r.Seeds, &r.Price, &free, &r.AddedOn, &used))
		r.Free, r.UsedUp = free == 1, used == 1
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestStrainPack_RestocksAddPacks(t *testing.T) {
	t.Parallel()
	e := newStrainEditEnv(t)
	today := time.Now().Format("2006-01-02")
	testutil.MustExec(t, e.db, "UPDATE strain SET seed_count = 0 WHERE id = $1", e.strainID)
	path := "/strains/" + strconv.Itoa(e.strainID) + "/seeds"
	add := func(body map[string]any) int {
		resp := e.client.SessionPostJSON(t, path, e.token, body)
		defer testutil.DrainAndClose(resp)
		return resp.StatusCode
	}

	// First pack, paid, dated as entered.
	require.Equal(t, http.StatusOK, add(map[string]any{"count": 10, "pack": map[string]any{"price_cents": 18000, "free": false, "added_on": "2026-10-02"}}))
	// The card's +: one more seed in the same pack.
	require.Equal(t, http.StatusOK, add(map[string]any{"count": 1}))
	// A freebie restock with no date: today.
	require.Equal(t, http.StatusOK, add(map[string]any{"count": 5, "pack": map[string]any{"price_cents": 15000, "free": true}}))
	p := e.packs(t)
	require.Len(t, p, 2)
	assert.Equal(t, packRow{Seeds: 11, Price: sql.NullInt64{Int64: 18000, Valid: true}, AddedOn: "2026-10-02"}, p[0])
	assert.Equal(t, packRow{Seeds: 5, Price: sql.NullInt64{Int64: 15000, Valid: true}, Free: true, AddedOn: today}, p[1])

	// Used up, then restocked from zero: the old packs are retired.
	testutil.MustExec(t, e.db, "UPDATE strain SET seed_count = 0 WHERE id = $1", e.strainID)
	require.Equal(t, http.StatusOK, add(map[string]any{"count": 3, "pack": map[string]any{"price_cents": nil, "free": false}}))
	p = e.packs(t)
	require.Len(t, p, 3)
	assert.True(t, p[0].UsedUp)
	assert.True(t, p[1].UsedUp)
	assert.Equal(t, packRow{Seeds: 3, AddedOn: today}, p[2], "a pack with no price still counts its seeds")

	for _, pack := range []map[string]any{{"price_cents": -1}, {"price_cents": 10_000_001}, {"added_on": "02/10/2026"}} {
		assert.Equal(t, http.StatusBadRequest, add(map[string]any{"count": 1, "pack": pack}), pack)
	}
}

func TestStrainPack_EditChangesTheNewestPack(t *testing.T) {
	t.Parallel()
	e := newStrainEditEnv(t)
	testutil.MustExec(t, e.db, "UPDATE strain SET seed_count = 7, seeds_added_on = '2024-08-24' WHERE id = $1", e.strainID)

	// Seeds from before packs were recorded: a price starts a pack for them.
	require.Equal(t, http.StatusOK, e.put(t, map[string]any{"seed_count": 7, "pack": map[string]any{"price_cents": nil, "free": false}}))
	assert.Empty(t, e.packs(t), "nothing entered: no pack")
	require.Equal(t, http.StatusOK, e.put(t, map[string]any{"seed_count": 7, "pack": map[string]any{"price_cents": 18000, "free": false}}))
	p := e.packs(t)
	require.Len(t, p, 1)
	assert.Equal(t, packRow{Seeds: 7, Price: sql.NullInt64{Int64: 18000, Valid: true}, AddedOn: "2024-08-24"}, p[0])

	testutil.MustExec(t, e.db, `INSERT INTO strain_pack (strain_id, seeds, price_cents, free, added_on) VALUES ($1, 5, 15000, 0, '2026-10-02')`, e.strainID)
	page := e.getBody(t, "/strain/"+strconv.Itoa(e.strainID)+"/edit")
	assert.Contains(t, page, `inputmode="decimal" value="150"`, "the newest pack's price")
	assert.Contains(t, page, `id="editPackEarlier"`, "the earlier pack is listed")

	// A correction changes the newest pack only.
	require.Equal(t, http.StatusOK, e.put(t, map[string]any{"seed_count": 12, "pack": map[string]any{"price_cents": 12550, "free": true}}))
	p = e.packs(t)
	require.Len(t, p, 2)
	assert.Equal(t, int64(18000), p[0].Price.Int64)
	assert.Equal(t, int64(12550), p[1].Price.Int64)
	assert.True(t, p[1].Free)
	assert.Contains(t, e.getBody(t, "/strain/"+strconv.Itoa(e.strainID)+"/edit"), `value="125.50"`)

	// Not sent: kept.
	require.Equal(t, http.StatusOK, e.put(t, map[string]any{"seed_count": 11}))
	assert.Equal(t, int64(12550), e.packs(t)[1].Price.Int64)

	// Back in stock from zero on the form: a new pack, the old ones retired.
	require.Equal(t, http.StatusOK, e.put(t, map[string]any{"seed_count": 0}))
	require.Equal(t, http.StatusOK, e.put(t, map[string]any{"seed_count": 4, "seeds_added_on": "2026-11-01", "pack": map[string]any{"price_cents": 9000, "free": false}}))
	p = e.packs(t)
	require.Len(t, p, 3)
	assert.True(t, p[0].UsedUp && p[1].UsedUp)
	assert.Equal(t, packRow{Seeds: 4, Price: sql.NullInt64{Int64: 9000, Valid: true}, AddedOn: "2026-11-01"}, p[2])

	assert.Equal(t, http.StatusBadRequest, e.put(t, map[string]any{"seed_count": 4, "pack": map[string]any{"price_cents": -5}}))
}

func TestStrainPack_AddStrainListAndDelete(t *testing.T) {
	t.Parallel()
	e := newStrainEditEnv(t)
	resp := e.client.SessionPostJSON(t, "/strains", e.token, map[string]any{
		"name": "Priced", "breeder_id": e.breeder, "indica": 50, "sativa": 50, "seed_count": 5, "seeds_added_on": "2026-09-01",
		"pack": map[string]any{"price_cents": 15000, "free": true},
	})
	var created struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	list := e.client.Get("/strains/all")
	defer testutil.DrainAndClose(list)
	var strains []struct {
		ID    int `json:"id"`
		Packs []struct {
			Seeds   int    `json:"seeds"`
			Price   *int   `json:"price_cents"`
			Free    bool   `json:"free"`
			AddedOn string `json:"added_on"`
		} `json:"packs"`
	}
	require.NoError(t, json.NewDecoder(list.Body).Decode(&strains))
	found := false
	for _, s := range strains {
		if s.ID != created.ID {
			assert.Empty(t, s.Packs)
			continue
		}
		found = true
		require.Len(t, s.Packs, 1)
		require.NotNil(t, s.Packs[0].Price)
		assert.Equal(t, 15000, *s.Packs[0].Price)
		assert.True(t, s.Packs[0].Free)
		assert.Equal(t, 5, s.Packs[0].Seeds)
		assert.Equal(t, "2026-09-01", s.Packs[0].AddedOn)
	}
	assert.True(t, found)

	del := e.client.SessionDeleteJSON(t, "/strains/"+strconv.Itoa(created.ID), e.token, nil)
	testutil.DrainAndClose(del)
	require.Equal(t, http.StatusOK, del.StatusCode)
	var n int
	require.NoError(t, e.db.QueryRow("SELECT COUNT(*) FROM strain_pack WHERE strain_id = $1", created.ID).Scan(&n))
	assert.Zero(t, n, "packs go with their strain")
}

func TestImportReview_DestinationPackPrice(t *testing.T) {
	t.Parallel()
	s := newImportServer(t)
	q := url.Values{"source": {"straincompass"}, "slug": {"blue-dream"}, "name": {"Blue Dream"}, "listing_breeder": {"Seed Supreme"},
		"dest": {"vault"}, "seed_count": {"4"}, "pack_price_cents": {"14999"}, "pack_free": {"1"}}
	_, page := s.reviewPage(t, q)
	assert.Contains(t, page, `value="149.99"`)
	assert.Regexp(t, `id="editPackFree" checked`, page)

	q.Set("pack_price_cents", "-3")
	q.Del("pack_free")
	_, page = s.reviewPage(t, q)
	assert.Contains(t, page, `inputmode="decimal" value=""`, "a bad price is left out")
	assert.NotRegexp(t, `id="editPackFree" checked`, page)
}
