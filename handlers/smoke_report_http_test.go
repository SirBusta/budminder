package handlers_test

// Smoke reports: saving, tags, prefill from a plant, photos, settings.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func intp(n int) *int { return &n }

// newSmokeEnv is newSensorEnv with photos written to a temporary folder.
func newSmokeEnv(t *testing.T) *sensorEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(t.TempDir()))
	testutil.SeedAdmin(t, db, sensorManagePassword)
	c, token := server.LoginAndFetchCSRF(t, sensorManagePassword, "/settings")
	return &sensorEnv{db: db, server: server, client: c, token: token}
}

func TestSmokeOverall(t *testing.T) {
	t.Parallel()
	r := handlers.SmokeReport{ScoreBud: intp(8), ScoreTaste: intp(7), ScoreSmell: intp(8), ScoreEffects: intp(9)}
	assert.Equal(t, 8.0, *handlers.SmokeOverall(r, handlers.DefaultSmokeWeights()))
	assert.Equal(t, 8.3, *handlers.SmokeOverall(r, handlers.ParseSmokeWeights("bud:1,taste:1,smell:1,effects:3")))
	// Blank scores are skipped; weight 0 leaves a score out.
	r2 := handlers.SmokeReport{ScoreBud: intp(4), ScoreEffects: intp(10)}
	assert.Equal(t, 7.0, *handlers.SmokeOverall(r2, handlers.DefaultSmokeWeights()))
	assert.Equal(t, 10.0, *handlers.SmokeOverall(r2, handlers.ParseSmokeWeights("bud:0")))
	assert.Nil(t, handlers.SmokeOverall(handlers.SmokeReport{}, handlers.DefaultSmokeWeights()))
	// Broken or out-of-range weights fall back / clamp.
	assert.Equal(t, handlers.SmokeWeights{Bud: 1, Taste: 5, Smell: 0, Effects: 1}, handlers.ParseSmokeWeights("bud:x,taste:9,smell:-2,nope:3"))
}

type smokeList struct {
	Reports []handlers.SmokeReport `json:"reports"`
}

func (e *sensorEnv) smokeAll(t *testing.T) []handlers.SmokeReport {
	t.Helper()
	resp := e.client.Get("/smoke/all")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out smokeList
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out.Reports
}

func (e *sensorEnv) postSmoke(t *testing.T, method, path string, body map[string]any) (int, map[string]any) {
	t.Helper()
	var resp *http.Response
	if method == http.MethodPut {
		resp = e.client.SessionPutJSON(t, path, e.token, body)
	} else {
		resp = e.client.SessionPostJSON(t, path, e.token, body)
	}
	defer testutil.DrainAndClose(resp)
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSmokeReport_CreateEditDelete(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	strain := testutil.SeedStrain(t, e.db, testutil.SeedBreeder(t, e.db, "Cali"), "Grape OG")
	plant := testutil.SeedPlant(t, e.db, "Grape OG #1", strain, testutil.SeedZone(t, e.db, "Tent"))

	code, out := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{
		"plant_id": plant, "strain_id": strain, "strain_name": "Grape OG", "breeder_name": "Cali", "smoked_on": "2025-04-12",
		"method": "joint", "again": "yes", "score_bud": 9, "score_taste": 9, "score_smell": 8, "score_effects": 9,
		"tags": []map[string]any{{"kind": "effect", "name": "relaxed"}, {"kind": "smell", "name": "grape"},
			{"kind": "smell", "name": "Grape soda", "custom": true}, {"kind": "effect", "name": "relaxed"}},
	})
	require.Equal(t, http.StatusOK, code, out)
	id := int(out["id"].(float64))

	list := e.smokeAll(t)
	require.Len(t, list, 1)
	r := list[0]
	assert.Equal(t, "Grape OG #1", r.PlantName)
	assert.Equal(t, 8.8, *r.Overall)
	assert.Len(t, r.Tags, 3, "the repeated tag is stored once")

	code, out = e.postSmoke(t, http.MethodPut, fmt.Sprintf("/smoke/%d", id), map[string]any{
		"plant_id": plant, "strain_name": "Grape OG", "smoked_on": "2025-04-20", "score_effects": 7,
		"tags": []map[string]any{{"kind": "side", "name": "dry_mouth"}},
	})
	require.Equal(t, http.StatusOK, code, out)
	r = e.smokeAll(t)[0]
	assert.Equal(t, "2025-04-20", r.SmokedOn)
	assert.Nil(t, r.ScoreBud, "fields left out are cleared")
	assert.Equal(t, 7.0, *r.Overall)
	require.Len(t, r.Tags, 1)
	assert.Equal(t, "dry_mouth", r.Tags[0].Name)

	resp := e.client.SessionDeleteJSON(t, fmt.Sprintf("/smoke/%d", id), e.token, nil)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, e.smokeAll(t))
	var tags int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM smoke_report_tag`).Scan(&tags))
	assert.Zero(t, tags)
}

func TestSmokeReport_RejectsBadInput(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	for name, body := range map[string]map[string]any{
		"no strain":         {"smoked_on": "2025-04-12"},
		"score out of 1-10": {"strain_name": "X", "score_bud": 11},
		"bad date":          {"strain_name": "X", "smoked_on": "12/04/2025"},
		"unknown choice":    {"strain_name": "X", "tags": []map[string]any{{"kind": "effect", "name": "flying"}}},
		"unknown kind":      {"strain_name": "X", "tags": []map[string]any{{"kind": "mood", "name": "x", "custom": true}}},
		"bad again":         {"strain_name": "X", "again": "never"},
		"missing plant":     {"strain_name": "X", "plant_id": 9999},
	} {
		code, _ := e.postSmoke(t, http.MethodPost, "/smoke", body)
		assert.Equal(t, http.StatusBadRequest, code, name)
	}
	assert.Empty(t, e.smokeAll(t))
}

func TestSmokePrefill_FromPlant(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	strain := testutil.SeedStrain(t, e.db, testutil.SeedBreeder(t, e.db, "The Cali Connection"), "Grape OG")
	e.exec(t, `UPDATE strain SET thc_min = 19, thc_max = 24 WHERE id = $1`, strain)
	zone := testutil.SeedZone(t, e.db, "3x3 Tent")
	plant := testutil.SeedPlant(t, e.db, "Grape OG #1", strain, zone)
	e.exec(t, `UPDATE plant SET start_dt = '2024-09-20 09:00:00' WHERE id = $1`, plant)
	for _, s := range [][2]string{{"Veg", "2024-10-11"}, {"Drying", "2025-02-01"}, {"Curing", "2025-02-12"}} {
		e.exec(t, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)`, plant, statusIDByName(t, e.db, s[0]), s[1])
	}
	e.exec(t, `INSERT INTO sensors (name, zone_id, source, device, type) VALUES ('Inline fan', $1, 'acinfinity', 'd', 'ACIP.2'), ('Mars Hydro FC3000', $1, 'acinfinity', 'd', 'ACIP.1')`, zone)
	e.exec(t, `INSERT INTO smoke_report (strain_name, grow_space, smoked_on, container, medium, nutrients) VALUES ('Old', '3x3 Tent', '2024-06-01', '5 gal fabric', 'Coco', 'Jacks 321')`)

	resp := e.client.Get(fmt.Sprintf("/smoke/prefill?plant_id=%d", plant))
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var p handlers.SmokePrefill
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	assert.Equal(t, "Grape OG", p.StrainName)
	assert.Equal(t, "The Cali Connection", p.BreederName)
	assert.Equal(t, "3x3 Tent", p.GrowSpace)
	assert.Equal(t, "2025-02-01", p.HarvestedOn)
	assert.Equal(t, "2025-02-12", p.CuringOn)
	require.NotNil(t, p.DaysToHarvest)
	assert.Equal(t, 134, *p.DaysToHarvest)
	require.NotNil(t, p.DryDays)
	assert.Equal(t, 11, *p.DryDays)
	assert.Equal(t, "19–24", p.THC)
	assert.Equal(t, "Mars Hydro FC3000", p.LightPort, "the port named like a light")
	assert.Equal(t, "5 gal fabric", p.Container, "remembered from the last report in that tent")
	assert.Equal(t, "Jacks 321", p.Nutrients)
}

func TestSmokeReport_SurvivesPlantDelete(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	strain := testutil.SeedStrain(t, e.db, testutil.SeedBreeder(t, e.db, "B"), "S")
	plant := testutil.SeedPlant(t, e.db, "P", strain, testutil.SeedZone(t, e.db, "Z"))
	code, _ := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{"plant_id": plant, "strain_id": strain, "strain_name": "S", "score_taste": 6})
	require.Equal(t, http.StatusOK, code)
	require.NoError(t, handlers.DeletePlantById(e.db, fmt.Sprint(plant)))
	list := e.smokeAll(t)
	require.Len(t, list, 1)
	assert.Nil(t, list[0].PlantID)
	assert.Equal(t, "S", list[0].StrainName)
}

func TestSmokeReport_AddToStrains(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	body := map[string]any{"source": "bought", "strain_name": "Gary Payton", "breeder_name": "Cookies", "add_to_strains": true,
		"dispensary": "Green Leaf", "price": 45, "amount_g": 3.5, "score_effects": 7}
	code, out := e.postSmoke(t, http.MethodPost, "/smoke", body)
	require.Equal(t, http.StatusOK, code, out)
	strainID := int(out["strain_id"].(float64))
	var name, breeder string
	var seeds int
	require.NoError(t, e.db.QueryRow(`SELECT s.name, b.name, s.seed_count FROM strain s JOIN breeder b ON b.id = s.breeder_id WHERE s.id = $1`, strainID).Scan(&name, &breeder, &seeds))
	assert.Equal(t, "Gary Payton", name)
	assert.Equal(t, "Cookies", breeder)
	assert.Zero(t, seeds, "library only")

	// The same strain again is reused, not added twice.
	code, out = e.postSmoke(t, http.MethodPost, "/smoke", body)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(strainID), out["strain_id"])

	r := e.smokeAll(t)[0]
	require.NotNil(t, r.PricePerGram)
	assert.Equal(t, 12.86, *r.PricePerGram)
}

func smokePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.RGBA{0, 128, 0, 255})
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, img))
	return b.Bytes()
}

func (e *sensorEnv) uploadSmoke(t *testing.T, id int, name string, data []byte) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("photos", name)
	require.NoError(t, err)
	_, _ = fw.Write(data)
	require.NoError(t, w.Close())
	req, err := http.NewRequest(http.MethodPost, e.client.BaseURL+fmt.Sprintf("/smoke/%d/photos", id), &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-CSRF-Token", e.token)
	resp, err := e.client.Do(req)
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	out := map[string]any{}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func TestSmokePhotos_UploadOrderDelete(t *testing.T) {
	t.Parallel()
	e := newSmokeEnv(t)
	code, out := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{"strain_name": "S"})
	require.Equal(t, http.StatusOK, code)
	id := int(out["id"].(float64))

	code, _ = e.uploadSmoke(t, id, "notes.txt", []byte("not an image at all"))
	assert.Equal(t, http.StatusBadRequest, code, "only real images")

	code, out = e.uploadSmoke(t, id, "a.png", smokePNG(t))
	require.Equal(t, http.StatusOK, code, out)
	code, _ = e.uploadSmoke(t, id, "b.png", smokePNG(t))
	require.Equal(t, http.StatusOK, code)

	r := e.smokeAll(t)[0]
	require.Len(t, r.Photos, 2)
	first, second := r.Photos[0], r.Photos[1]

	code, _ = e.postSmoke(t, http.MethodPut, fmt.Sprintf("/smoke/%d/photos/order", id), map[string]any{"ids": []int{second.ID, first.ID}})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, second.ID, e.smokeAll(t)[0].Photos[0].ID, "the reordered photo is now the main one")

	var path string
	require.NoError(t, e.db.QueryRow(`SELECT path FROM smoke_report_photo WHERE id = $1`, first.ID).Scan(&path))
	_, err := os.Stat(path)
	require.NoError(t, err, "stored under uploads/smoke")
	assert.Equal(t, "smoke", filepath.Base(filepath.Dir(path)))

	resp := e.client.SessionDeleteJSON(t, fmt.Sprintf("/smoke/%d/photos/%d", id, first.ID), e.token, nil)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "the file is removed too")
	assert.Len(t, e.smokeAll(t)[0].Photos, 1)
}

func TestSmokePhoto_FromJournalIsCopied(t *testing.T) {
	t.Parallel()
	e := newSmokeEnv(t)
	strain := testutil.SeedStrain(t, e.db, testutil.SeedBreeder(t, e.db, "B"), "S")
	plant := testutil.SeedPlant(t, e.db, "P", strain, testutil.SeedZone(t, e.db, "Z"))
	src := filepath.Join(e.server.UploadDir, "plants", "journal.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, smokePNG(t), 0o644))
	var imageID int
	require.NoError(t, e.db.QueryRow(`INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date) VALUES ($1, $2, '', 100, '2025-01-01') RETURNING id`, plant, src).Scan(&imageID))

	code, out := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{"plant_id": plant, "strain_name": "S"})
	require.Equal(t, http.StatusOK, code)
	id := int(out["id"].(float64))
	code, out = e.postSmoke(t, http.MethodPost, fmt.Sprintf("/smoke/%d/photos/journal", id), map[string]any{"image_id": imageID})
	require.Equal(t, http.StatusOK, code, out)

	require.NoError(t, os.Remove(src)) // deleting the journal photo…
	r := e.smokeAll(t)[0]
	require.Len(t, r.Photos, 1)
	var path string
	require.NoError(t, e.db.QueryRow(`SELECT path FROM smoke_report_photo WHERE report_id = $1`, id).Scan(&path))
	_, err := os.Stat(path)
	assert.NoError(t, err, "…leaves the report's copy")
}

func TestSmokeSettings_WeightsAndHidden(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	code, _ := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{"strain_name": "S", "score_bud": 6, "score_effects": 10})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 8.0, *e.smokeAll(t)[0].Overall)

	code, out := e.postSmoke(t, http.MethodPost, "/smoke/settings", map[string]any{
		"weights": map[string]int{"bud": 1, "taste": 1, "smell": 1, "effects": 3},
		"hidden":  []string{"nutrients", "made_up_field"},
	})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, 9.0, *e.smokeAll(t)[0].Overall, "the new weights apply to every report")
	set := handlers.LoadSmokeSettings(e.db)
	assert.True(t, set.Hidden["nutrients"])
	assert.False(t, set.Hidden["made_up_field"], "unknown fields are ignored")
}

func TestSmokeReport_NeedsLogin(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	c := server.NewClient(t)
	resp := c.SessionPostJSON(t, "/smoke", "", map[string]any{"strain_name": "S"})
	testutil.DrainAndClose(resp)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	resp = c.Get("/smoke/new")
	testutil.DrainAndClose(resp)
	assert.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusUnauthorized, resp.StatusCode)
	assert.False(t, strings.Contains(resp.Header.Get("Location"), "/smoke/new"))
}

func TestSmokePages_Render(t *testing.T) {
	t.Parallel()
	e := newSensorEnv(t)
	strain := testutil.SeedStrain(t, e.db, testutil.SeedBreeder(t, e.db, "Cali"), "Grape OG")
	plant := testutil.SeedPlant(t, e.db, "Grape OG #1", strain, testutil.SeedZone(t, e.db, "Tent"))
	code, out := e.postSmoke(t, http.MethodPost, "/smoke", map[string]any{
		"plant_id": plant, "strain_id": strain, "strain_name": "Grape OG", "breeder_name": "Cali", "smoked_on": "2025-04-12",
		"method": "joint", "again": "yes", "score_bud": 9, "score_effects": 8, "cure_days": 59, "lights": "LED 480",
		"tags": []map[string]any{{"kind": "effect", "name": "relaxed"}, {"kind": "smell", "name": "Grape soda", "custom": true}},
	})
	require.Equal(t, http.StatusOK, code, out)
	id := int(out["id"].(float64))

	list := e.page(t, "/smoke")
	assert.Contains(t, list, `id="smokePage"`)
	assert.Contains(t, list, "smoke-page.js")
	assert.Contains(t, list, `href="/smoke"`, "the top-menu item")

	rep := e.page(t, fmt.Sprintf("/smoke/%d", id))
	assert.Contains(t, rep, "8.5<small>/10</small>", "overall score")
	assert.Contains(t, rep, "Relaxed", "choice-list words are translated")
	assert.Contains(t, rep, "Grape soda", "the user's own words as typed")
	assert.Contains(t, rep, "LED 480")
	assert.Contains(t, rep, "59 days")
	assert.Contains(t, rep, fmt.Sprintf(`href="/plant/%d"`, plant))

	// Hiding a field hides it on the report too (the value is kept).
	code, _ = e.postSmoke(t, http.MethodPost, "/smoke/settings", map[string]any{"weights": map[string]int{"bud": 1, "taste": 1, "smell": 1, "effects": 1}, "hidden": []string{"lights"}})
	require.Equal(t, http.StatusOK, code)
	assert.NotContains(t, e.page(t, fmt.Sprintf("/smoke/%d", id)), "LED 480")

	form := e.page(t, fmt.Sprintf("/smoke/%d/edit", id))
	assert.Contains(t, form, `id="smokeForm"`)
	assert.Contains(t, form, "smoke-form.js")
	newForm := e.page(t, fmt.Sprintf("/smoke/new?plant_id=%d", plant))
	assert.Contains(t, newForm, fmt.Sprintf(`&#34;plant_id&#34;:%d`, plant))
}
