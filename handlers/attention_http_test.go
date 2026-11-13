package handlers_test

// Today page, Needs attention (handlers/attention.go): the settings window,
// repeating tasks, hidden lines, Done / Undo and burping, through the real
// session + CSRF round-trip; and the demo's rules for them.

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type attentionEnv struct {
	*sensorEnv
}

func newAttentionEnv(t *testing.T) *attentionEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, homePassword)
	c, token := server.LoginAndFetchCSRF(t, homePassword, "/settings")
	return &attentionEnv{&sensorEnv{db: db, server: server, client: c, token: token}}
}

func (e *attentionEnv) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	resp := e.client.SessionPostJSON(t, path, e.token, body)
	defer testutil.DrainAndClose(resp)
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

type attentionData struct {
	Today    string `json:"today"`
	CareDays struct {
		Water []int `json:"water"`
		Feed  []int `json:"feed"`
	} `json:"care_days"`
	OldSeeds  []struct{ ID int } `json:"old_seeds"`
	Attention *struct {
		Settings struct {
			Water struct {
				On   bool `json:"on"`
				Due  int  `json:"due"`
				Over int  `json:"over"`
			} `json:"water"`
			Burp struct {
				On bool `json:"on"`
			} `json:"burp"`
		} `json:"settings"`
		Tasks []struct {
			ID    int               `json:"id"`
			Name  string            `json:"name"`
			Icon  string            `json:"icon"`
			Every int               `json:"every"`
			Zones map[string]string `json:"zones"`
		} `json:"tasks"`
		Zones  []struct{ ID int } `json:"zones"`
		Hidden map[string]struct {
			How   string `json:"how"`
			Until string `json:"until"`
			Basis string `json:"basis"`
		} `json:"hidden"`
		Burped map[string]string `json:"burped"`
		Curing []struct {
			ID    int    `json:"id"`
			Since string `json:"since"`
		} `json:"curing"`
		Presets []struct{ Key string } `json:"presets"`
	} `json:"attention"`
}

func getAttention(t *testing.T, c *testutil.Client) attentionData {
	t.Helper()
	resp := c.Get("/home/data")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var d attentionData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&d))
	return d
}

func attentionSettings(waterDue, waterOver int) map[string]any {
	return map[string]any{
		"water": map[string]any{"on": true, "due": waterDue, "over": waterOver},
		"feed":  map[string]any{"on": false, "due": 10, "over": 18},
		"photo": map[string]any{"on": true, "days": 14},
		"seeds": map[string]any{"on": true}, "sensors": map[string]any{"on": true},
		"burp": map[string]any{"on": true, "daily": 14, "then": 7},
	}
}

func TestAttention_DefaultsAndGuests(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	d := getAttention(t, e.client)
	require.NotNil(t, d.Attention)
	assert.Equal(t, 3, d.Attention.Settings.Water.Due)
	assert.Equal(t, 6, d.Attention.Settings.Water.Over)
	assert.False(t, d.Attention.Settings.Burp.On, "burping starts off")
	assert.Equal(t, []int{3, 6}, d.CareDays.Water)
	assert.Equal(t, []int{10, 18}, d.CareDays.Feed)
	assert.Empty(t, d.Attention.Tasks)
	assert.Len(t, d.Attention.Presets, 11)

	guest := getAttention(t, testutil.NewTestServer(t, testutil.NewTestDB(t), testutil.WithGuestMode()).NewClient(t))
	assert.Nil(t, guest.Attention, "guests get no Needs attention data")
	assert.Equal(t, []int{3, 6}, guest.CareDays.Water, "tag colors are for everyone")
}

func TestAttention_SaveSettingsAndTasks(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	a := testutil.SeedZone(t, e.db, "Flower 1")
	b := testutil.SeedZone(t, e.db, "Flower 2")

	code, out := e.post(t, "/home/attention/settings", map[string]any{
		"settings": attentionSettings(2, 5),
		"tasks": []map[string]any{
			{"id": 0, "name": "  Empty   dehumidifier ", "icon": "fa-bucket", "every": 2, "zones": []int{a, b}},
			{"id": 0, "name": "My own task", "icon": "fa-<script>", "every": 30, "zones": []int{a}},
		},
	})
	require.Equal(t, http.StatusOK, code, out)
	d := getAttention(t, e.client)
	assert.Equal(t, 2, d.Attention.Settings.Water.Due)
	assert.Equal(t, []int{2, 5}, d.CareDays.Water, "the Plants page and tags follow the saved days")
	require.Len(t, d.Attention.Tasks, 2)
	first := d.Attention.Tasks[0]
	assert.Equal(t, "Empty dehumidifier", first.Name, "spaces are tidied")
	assert.Equal(t, "fa-bucket", first.Icon)
	assert.Len(t, first.Zones, 2)
	for _, day := range first.Zones {
		assert.Equal(t, d.Today, day, "a new task counts from today")
	}
	assert.Equal(t, "fa-repeat", d.Attention.Tasks[1].Icon, "an unknown icon becomes the plain one")
	assert.NotEqual(t, first.ID, d.Attention.Tasks[1].ID)

	// Mark it done in Flower 2 some days ago, then edit the task: kept tents
	// keep their day, a dropped tent goes, an added tent starts today.
	past := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	code, out = e.post(t, "/home/attention/done", map[string]any{"task_id": first.ID, "zone_id": b, "day": past})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, d.Today, out["previous"])
	c := testutil.SeedZone(t, e.db, "Veg")
	code, out = e.post(t, "/home/attention/settings", map[string]any{
		"settings": attentionSettings(2, 5),
		"tasks":    []map[string]any{{"id": first.ID, "name": "Empty dehumidifier", "icon": "fa-bucket", "every": 3, "zones": []int{b, c}}},
	})
	require.Equal(t, http.StatusOK, code, out)
	d = getAttention(t, e.client)
	require.Len(t, d.Attention.Tasks, 1, "a task left out is deleted")
	got := d.Attention.Tasks[0]
	assert.Equal(t, first.ID, got.ID)
	assert.Equal(t, 3, got.Every)
	assert.Equal(t, map[string]string{itoa(b): past, itoa(c): d.Today}, got.Zones)

	// A deleted tent drops out of the page data.
	e.exec(t, `DELETE FROM zones WHERE id = $1`, c)
	d = getAttention(t, e.client)
	assert.Equal(t, map[string]string{itoa(b): past}, d.Attention.Tasks[0].Zones)
}

func TestAttention_SaveRefusesBadValues(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	z := testutil.SeedZone(t, e.db, "Tent")
	task := func(mod func(m map[string]any)) []map[string]any {
		m := map[string]any{"id": 0, "name": "Check for pests", "icon": "fa-bug", "every": 7, "zones": []int{z}}
		if mod != nil {
			mod(m)
		}
		return []map[string]any{m}
	}
	for what, body := range map[string]map[string]any{
		"no settings":         {"tasks": task(nil)},
		"overdue before due":  {"settings": attentionSettings(5, 5), "tasks": task(nil)},
		"days out of range":   {"settings": attentionSettings(0, 5), "tasks": task(nil)},
		"empty name":          {"settings": attentionSettings(3, 6), "tasks": task(func(m map[string]any) { m["name"] = "   " })},
		"name too long":       {"settings": attentionSettings(3, 6), "tasks": task(func(m map[string]any) { m["name"] = string(make([]byte, 61)) + "x" })},
		"every out of range":  {"settings": attentionSettings(3, 6), "tasks": task(func(m map[string]any) { m["every"] = 366 })},
		"no tents":            {"settings": attentionSettings(3, 6), "tasks": task(func(m map[string]any) { m["zones"] = []int{} })},
		"tent that isn't one": {"settings": attentionSettings(3, 6), "tasks": task(func(m map[string]any) { m["zones"] = []int{z + 99} })},
	} {
		code, _ := e.post(t, "/home/attention/settings", body)
		assert.Equal(t, http.StatusBadRequest, code, what)
	}
	assert.Empty(t, getAttention(t, e.client).Attention.Tasks, "nothing was saved")
}

func TestAttention_HideAndShow(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	code, out := e.post(t, "/home/attention/hide", map[string]any{"key": "water:12", "how": "until", "basis": "2026-11-03"})
	require.Equal(t, http.StatusOK, code, out)
	code, out = e.post(t, "/home/attention/hide", map[string]any{"key": "task:1:2", "how": "tomorrow", "basis": "ignored"})
	require.Equal(t, http.StatusOK, code, out)
	d := getAttention(t, e.client)
	assert.Equal(t, "2026-11-03", d.Attention.Hidden["water:12"].Basis)
	tomorrow := d.Attention.Hidden["task:1:2"]
	assert.Equal(t, "tomorrow", tomorrow.How)
	assert.Greater(t, tomorrow.Until, d.Today)
	assert.Empty(t, tomorrow.Basis)

	code, _ = e.post(t, "/home/attention/show", map[string]any{"key": "water:12"})
	require.Equal(t, http.StatusOK, code)
	d = getAttention(t, e.client)
	assert.NotContains(t, d.Attention.Hidden, "water:12")
	assert.Contains(t, d.Attention.Hidden, "task:1:2")

	// An expired "tomorrow" hide is no longer sent.
	e.exec(t, `UPDATE settings SET value = $1 WHERE name = 'attention.hidden'`, `{"task:1:2":{"how":"tomorrow","until":"2020-01-02","at":"2020-01-01"}}`)
	assert.Empty(t, getAttention(t, e.client).Attention.Hidden)

	for what, body := range map[string]map[string]any{
		"odd key":   {"key": "<b>", "how": "until", "basis": ""},
		"odd how":   {"key": "seeds", "how": "forever", "basis": ""},
		"odd basis": {"key": "seeds", "how": "until", "basis": "1@x\"y"},
	} {
		code, _ := e.post(t, "/home/attention/hide", body)
		assert.Equal(t, http.StatusBadRequest, code, what)
	}
}

func TestAttention_DoneAndUndo(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	z := testutil.SeedZone(t, e.db, "Tent")
	code, out := e.post(t, "/home/attention/settings", map[string]any{
		"settings": attentionSettings(3, 6),
		"tasks":    []map[string]any{{"id": 0, "name": "Refill humidifier", "icon": "fa-cloud", "every": 2, "zones": []int{z}}},
	})
	require.Equal(t, http.StatusOK, code, out)
	d := getAttention(t, e.client)
	id := d.Attention.Tasks[0].ID
	old := time.Now().AddDate(0, 0, -4).Format("2006-01-02")

	code, out = e.post(t, "/home/attention/done", map[string]any{"task_id": id, "zone_id": z, "day": old})
	require.Equal(t, http.StatusOK, code, out)
	code, out = e.post(t, "/home/attention/done", map[string]any{"task_id": id, "zone_id": z, "day": d.Today})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, old, out["previous"], "Undo gets the day before")

	tomorrow := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	for what, body := range map[string]map[string]any{
		"a day ahead":      {"task_id": id, "zone_id": z, "day": tomorrow},
		"not a day":        {"task_id": id, "zone_id": z, "day": "soon"},
		"tent not on task": {"task_id": id, "zone_id": z + 5, "day": d.Today},
		"no such task":     {"task_id": id + 5, "zone_id": z, "day": d.Today},
	} {
		code, _ := e.post(t, "/home/attention/done", body)
		assert.NotEqual(t, http.StatusOK, code, what)
	}
}

func TestAttention_Burp(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	z := testutil.SeedZone(t, e.db, "Jars")
	curing := livingPlant(t, e.sensorEnv, "Cured", z, "Curing")
	livingPlant(t, e.sensorEnv, "Growing", z, "Veg")

	d := getAttention(t, e.client)
	require.Len(t, d.Attention.Curing, 1, "only curing plants are listed")
	assert.Equal(t, curing, d.Attention.Curing[0].ID)
	assert.Equal(t, "2026-09-01", d.Attention.Curing[0].Since)

	code, out := e.post(t, "/home/attention/done", map[string]any{"burp": []map[string]any{{"plant_id": curing, "day": d.Today}}})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, []any{map[string]any{"plant_id": float64(curing), "day": ""}}, out["previous"])
	assert.Equal(t, d.Today, getAttention(t, e.client).Attention.Burped[itoa(curing)])

	// Undo of the first burp forgets it.
	code, _ = e.post(t, "/home/attention/done", map[string]any{"burp": []map[string]any{{"plant_id": curing, "day": ""}}})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, getAttention(t, e.client).Attention.Burped)

	code, _ = e.post(t, "/home/attention/done", map[string]any{"burp": []map[string]any{{"plant_id": curing + 99, "day": d.Today}}})
	assert.Equal(t, http.StatusNotFound, code, "a plant that doesn't exist")
}

func TestAttention_PlantsPageUsesTheDays(t *testing.T) {
	t.Parallel()
	e := newAttentionEnv(t)
	code, out := e.post(t, "/home/attention/settings", map[string]any{"settings": attentionSettings(4, 9), "tasks": []any{}})
	require.Equal(t, http.StatusOK, code, out)
	resp := e.client.Get("/plants")
	defer testutil.DrainAndClose(resp)
	raw, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(raw), `&#34;water&#34;:[4,9]`, "the Plants page gets the saved days")
}

func TestDemo_NeedsAttention(t *testing.T) {
	t.Parallel()
	e := newDemo(t)
	task := func(mod func(m map[string]any)) map[string]any {
		m := map[string]any{"id": 0, "name": "Check for pests", "icon": "fa-bug", "every": 7, "zones": []int{e.zone}}
		if mod != nil {
			mod(m)
		}
		return map[string]any{"settings": attentionSettings(3, 6), "tasks": []map[string]any{m}}
	}
	code, out := e.json(t, "POST", "/home/attention/settings", task(nil))
	require.Equal(t, http.StatusOK, code, out)
	code, out = e.json(t, "POST", "/home/attention/settings", task(func(m map[string]any) { m["name"] = "Revisar si hay plagas" }))
	assert.Equal(t, http.StatusOK, code, "a suggestion in another language: %v", out)
	for what, body := range map[string]map[string]any{
		"typed name":    task(func(m map[string]any) { m["name"] = "Visit my site" }),
		"odd icon":      task(func(m map[string]any) { m["icon"] = "fa-skull" }),
		"unknown field": task(func(m map[string]any) { m["note"] = "hi" }),
		"too often":     task(func(m map[string]any) { m["every"] = 0 }),
	} {
		code, out := e.json(t, "POST", "/home/attention/settings", body)
		assertRefused(t, code, out, what)
	}

	code, out = e.json(t, "POST", "/home/attention/hide", map[string]any{"key": "water:1", "how": "until", "basis": "2026-11-01"})
	assert.Equal(t, http.StatusOK, code, out)
	code, out = e.json(t, "POST", "/home/attention/hide", map[string]any{"key": "water:1", "how": "until", "basis": "<script>"})
	assertRefused(t, code, out, "odd basis")
	code, out = e.json(t, "POST", "/home/attention/show", map[string]any{"key": "water:1"})
	assert.Equal(t, http.StatusOK, code, out)
	code, out = e.json(t, "POST", "/home/attention/done", map[string]any{"burp": []map[string]any{{"plant_id": e.plant, "day": time.Now().Format("2006-01-02")}}})
	assert.Equal(t, http.StatusOK, code, out)
	code, out = e.json(t, "POST", "/home/attention/done", map[string]any{"burp": []map[string]any{{"plant_id": e.plant, "day": "x"}}})
	assertRefused(t, code, out, "a burp day that isn't one")
}
