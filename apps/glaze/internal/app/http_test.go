package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

// client talks to a real server with a fresh database.
type client struct {
	t   *testing.T
	srv *httptest.Server
	app *App
}

func newClient(t *testing.T) *client {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{DB: db, DataDir: dir, Web: fstest.MapFS{"index.html": {Data: []byte("the app")}}}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return &client{t, srv, a}
}

// call sends body (JSON text or raw bytes), checks the status and decodes a
// JSON answer into out when out isn't nil.
func (c *client) call(method, path string, body any, want int, out any) []byte {
	c.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case string:
		r = strings.NewReader(b)
	case []byte:
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, r)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		c.t.Fatalf("%s %s: %d %s, want %d", method, path, resp.StatusCode, data, want)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: %v in %s", method, path, err, data)
		}
	}
	return data
}

func (c *client) clay(name string) int64 {
	c.t.Helper()
	var b ClayBody
	c.call("POST", "/api/clay-bodies", fmt.Sprintf(`{"name":%q}`, name), 200, &b)
	return b.ID
}

func (c *client) material(name, kind string) int64 {
	c.t.Helper()
	var m MaterialDetail
	c.call("POST", "/api/materials", fmt.Sprintf(`{"name":%q,"kind":%q}`, name, kind), 200, &m)
	return m.ID
}

func (c *client) firing(schedule string) int64 {
	c.t.Helper()
	var f Firing
	c.call("POST", "/api/firings", fmt.Sprintf(`{"fired_on":"2026-11-09","cone":"6","schedule":%q}`, schedule), 200, &f)
	return f.ID
}

// unload takes a load through firing to unloaded, leaving the kiln free.
func (c *client) unload(id int64) {
	c.t.Helper()
	path := fmt.Sprintf("/api/firings/%d/state", id)
	c.call("PUT", path, `{"state":"firing"}`, 200, nil)
	c.call("PUT", path, `{"state":"unloaded"}`, 200, nil)
}

func (c *client) tile(number int64, extra string) Piece {
	c.t.Helper()
	var p Piece
	c.call("POST", "/api/pieces", fmt.Sprintf(`{"kind":"tile","number":%d%s}`, number, extra), 200, &p)
	return p
}

func (c *client) list(query string) []Piece {
	c.t.Helper()
	var out []Piece
	c.call("GET", "/api/pieces?"+query, nil, 200, &out)
	return out
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestIdentity(t *testing.T) {
	c := newClient(t)
	c.call("POST", "/api/pieces", `{"kind":"tile"}`, 400, nil)
	c.call("POST", "/api/pieces", `{"kind":"tile","number":0}`, 400, nil)
	c.call("POST", "/api/pieces", `{"kind":"pot","number":5}`, 400, nil)
	c.call("POST", "/api/pieces", `{"kind":"vase"}`, 400, nil)
	c.call("POST", "/api/pieces", `{"kind":"tile","numbr":5}`, 400, nil) // a misspelled field fails loudly

	// Only the number is needed: a partial record is still a record.
	p := c.tile(142, "")
	if p.State != "bench" || len(p.Layers) != 0 || p.ClayBody != nil {
		t.Errorf("partial tile = %+v", p)
	}

	// A number on two tiles is refused, whether new or edited.
	c.call("POST", "/api/pieces", `{"kind":"tile","number":142}`, 409, nil)
	other := c.tile(143, "")
	c.call("PUT", fmt.Sprintf("/api/pieces/%d", other.ID), `{"kind":"tile","number":142}`, 409, nil)

	// Pots have no numbers, so any number of them can exist.
	c.call("POST", "/api/pieces", `{"kind":"pot"}`, 200, nil)
	c.call("POST", "/api/pieces", `{"kind":"pot"}`, 200, nil)

	if found := c.list("number=142"); len(found) != 1 || found[0].ID != p.ID {
		t.Errorf("find by number = %+v", found)
	}
	c.call("PUT", fmt.Sprintf("/api/pieces/%d", p.ID), `{"kind":"pot"}`, 400, nil) // can't change kind
}

func TestLayersAreKeptInOrder(t *testing.T) {
	c := newClient(t)
	slip, oxide, glaze := c.material("White slip", "slip"), c.material("Copper", "oxide"), c.material("Celadon", "commercial")
	p := c.tile(1, fmt.Sprintf(`,"layers":[
		{"material_id":%d,"stage":"greenware","method":"brush","thickness":2.04,"coats":2,"area":"rim"},
		{"material_id":%d,"stage":"bisque","method":"sponge"},
		{"material_id":%d,"stage":"bisque","method":"dip","thickness":3.5}]`, slip, oxide, glaze))
	if len(p.Layers) != 3 || p.Layers[0].Material != "White slip" || p.Layers[1].Material != "Copper" || p.Layers[2].Material != "Celadon" {
		t.Fatalf("layers = %+v", p.Layers)
	}
	if *p.Layers[0].Thickness != 2.0 || *p.Layers[0].Coats != 2 || p.Layers[0].Area != "rim" {
		t.Errorf("first layer = %+v", p.Layers[0])
	}

	for _, bad := range []string{
		fmt.Sprintf(`{"material_id":%d,"thickness":5.1}`, glaze),
		fmt.Sprintf(`{"material_id":%d,"method":"dip","coats":2}`, glaze),
		fmt.Sprintf(`{"material_id":%d,"method":"throw"}`, glaze),
		`{"material_id":9999}`,
	} {
		c.call("POST", "/api/pieces", `{"kind":"tile","number":2,"layers":[`+bad+`]}`, 400, nil)
	}

	// Each material remembers how it was last applied, most recent first.
	var ms []Material
	c.call("GET", "/api/materials", nil, 200, &ms)
	if ms[0].Name != "White slip" && ms[0].Uses != 1 {
		t.Errorf("materials = %+v", ms)
	}
	for _, m := range ms {
		if m.Name == "Celadon" && (*m.LastUse.Method != "dip" || *m.LastUse.Thickness != 3.5) {
			t.Errorf("last use of Celadon = %+v", m.LastUse)
		}
	}
}

func TestStateFollowsTheKilnCycle(t *testing.T) {
	c := newClient(t)
	p := c.tile(1, "")
	path := fmt.Sprintf("/api/pieces/%d", p.ID)
	f := c.firing("slow cool")

	var got Piece
	c.call("PUT", path, fmt.Sprintf(`{"kind":"tile","number":1,"firing_id":%d,"level":"top","x":0,"y":0}`, f), 200, &got)
	if got.State != "kiln" || *got.Level != "top" || *got.Spot != "edge" || *got.X != 0 || got.Firing.Schedule != "slow cool" {
		t.Errorf("loaded = %+v", got)
	}
	c.call("PUT", path, fmt.Sprintf(`{"kind":"tile","number":1,"firing_id":%d,"level":"roof"}`, f), 400, nil)

	c.call("PUT", path, fmt.Sprintf(`{"kind":"tile","number":1,"firing_id":%d,"level":"top","x":0,"y":0,"rating":"great","defects":["running","running"]}`, f), 200, &got)
	if got.State != "done" || len(got.Defects) != 1 {
		t.Errorf("rated = %+v", got)
	}
	c.call("PUT", path, `{"kind":"tile","number":1,"rating":"amazing"}`, 400, nil)
	c.call("PUT", path, `{"kind":"tile","number":1,"defects":["sadness"]}`, 400, nil)

	// Taken out of the firing, its position goes too.
	c.call("PUT", path, `{"kind":"tile","number":1,"level":"top","x":0,"y":0}`, 200, &got)
	if got.State != "bench" || got.Level != nil || got.Spot != nil || got.X != nil {
		t.Errorf("unloaded = %+v", got)
	}
	// The backlog has a rating but no known firing.
	old := c.tile(2, `,"rating":"fine","from_memory":true`)
	if old.State != "done" || !old.FromMemory {
		t.Errorf("backlog tile = %+v", old)
	}

	// A firing with pieces in it can't be deleted.
	c.call("PUT", path, fmt.Sprintf(`{"kind":"tile","number":1,"firing_id":%d}`, f), 200, nil)
	c.unload(f)
	if got := c.list("number=1")[0]; got.State != "waiting" {
		t.Errorf("unloaded with no result = %+v", got)
	}
	c.call("DELETE", fmt.Sprintf("/api/firings/%d", f), nil, 409, nil)
	var fr Firing
	c.call("GET", fmt.Sprintf("/api/firings/%d", f), nil, 200, &fr)
	if fr.Pieces != 1 || fr.Waiting != 1 {
		t.Errorf("firing = %+v", fr)
	}
	c.call("POST", "/api/firings", `{"fired_on":"yesterday"}`, 400, nil)
	empty := c.firing("x")
	c.call("DELETE", fmt.Sprintf("/api/firings/%d", empty), nil, 204, nil)

	var s map[string]string
	c.call("GET", "/api/summary", nil, 200, &s)
	if s["text"] != "1 waiting for a result" {
		t.Errorf("summary = %v", s)
	}
}

func TestRecipesFreezeOnceUsed(t *testing.T) {
	c := newClient(t)
	id := c.material("House celadon", "own")
	path := fmt.Sprintf("/api/materials/%d/recipe", id)
	v1 := `{"lines":[{"ingredient":"Feldspar","amount":40},{"ingredient":"Silica","amount":60},{"ingredient":"Iron","amount":1,"addition":true}]}`

	var m MaterialDetail
	c.call("PUT", path, v1, 200, &m)
	if len(m.Recipes) != 1 || m.Recipes[0].Version != 1 || len(m.Recipes[0].Lines) != 3 || m.Recipes[0].Used {
		t.Fatalf("first save = %+v", m.Recipes)
	}
	// Unused, so a correction doesn't make a new version.
	c.call("PUT", path, `{"lines":[{"ingredient":"Feldspar","amount":45},{"ingredient":"Silica","amount":55}]}`, 200, &m)
	if len(m.Recipes) != 1 || m.Recipes[0].Lines[0].Amount != 45 {
		t.Fatalf("correction = %+v", m.Recipes)
	}
	firstVersion := m.Recipes[0].ID

	// A layer that doesn't choose gets the newest version.
	p := c.tile(1, fmt.Sprintf(`,"layers":[{"material_id":%d}]`, id))
	if *p.Layers[0].RecipeID != firstVersion || *p.Layers[0].RecipeVersion != 1 {
		t.Fatalf("layer = %+v", p.Layers[0])
	}

	// Used, so the change becomes version 2 and version 1 stays as it was.
	c.call("PUT", path, `{"change_note":"more silica","lines":[{"ingredient":"Feldspar","amount":30},{"ingredient":"Silica","amount":70}]}`, 200, &m)
	if len(m.Recipes) != 2 || m.Recipes[0].Version != 2 || m.Recipes[0].ChangeNote != "more silica" {
		t.Fatalf("after change = %+v", m.Recipes)
	}
	if !m.Recipes[1].Used || m.Recipes[1].Lines[0].Amount != 45 {
		t.Errorf("version 1 changed: %+v", m.Recipes[1])
	}
	got := c.list("number=1")[0]
	if *got.Layers[0].RecipeVersion != 1 {
		t.Errorf("old layer now on version %d", *got.Layers[0].RecipeVersion)
	}

	// Editing that old piece keeps its version, and a new layer gets version 2.
	body, _ := json.Marshal(map[string]any{"kind": "tile", "number": 1,
		"layers": []map[string]any{{"material_id": id, "recipe_id": firstVersion}, {"material_id": id}}})
	var edited Piece
	c.call("PUT", fmt.Sprintf("/api/pieces/%d", got.ID), string(body), 200, &edited)
	if *edited.Layers[0].RecipeVersion != 1 || *edited.Layers[1].RecipeVersion != 2 {
		t.Errorf("edited layers = %+v", edited.Layers)
	}

	// A version from another material is refused.
	other := c.material("Other", "own")
	c.call("POST", "/api/pieces", fmt.Sprintf(`{"kind":"tile","number":2,"layers":[{"material_id":%d,"recipe_id":%d}]}`, other, firstVersion), 400, nil)

	// Water is part of what a version froze, so changing it makes a new one.
	var watered MaterialDetail
	c.call("PUT", path, `{"water":80,"consistency":"thin cream","lines":[{"ingredient":"Feldspar","amount":30},{"ingredient":"Silica","amount":70}]}`, 200, &watered)
	if len(watered.Recipes) != 3 || *watered.Recipes[0].Water != 80 || watered.Recipes[0].Consistency != "thin cream" {
		t.Fatalf("water on a used version = %+v", watered.Recipes)
	}
	if watered.Recipes[1].Water != nil {
		t.Errorf("version 2 gained water: %+v", watered.Recipes[1])
	}
	c.call("PUT", path, `{"water":0,"lines":[{"ingredient":"Silica","amount":1}]}`, 400, nil)

	// An oxide wash is the oxide, anything else in it, and its water.
	wash := c.material("Copper wash", "oxide")
	var w MaterialDetail
	c.call("PUT", fmt.Sprintf("/api/materials/%d/recipe", wash), `{"water":100,"lines":[{"ingredient":"Copper carbonate","amount":10},{"ingredient":"CMC","amount":1}]}`, 200, &w)
	if *w.Recipes[0].Water != 100 || len(w.Recipes[0].Lines) != 2 {
		t.Errorf("wash = %+v", w.Recipes[0])
	}

	// Bad recipes.
	c.call("PUT", path, `{"lines":[]}`, 400, nil)
	c.call("PUT", path, `{"lines":[{"ingredient":"Iron","amount":1,"addition":true}]}`, 400, nil)
	c.call("PUT", path, `{"lines":[{"ingredient":"Silica","amount":0}]}`, 400, nil)
	c.call("PUT", path, `{"lines":[{"ingredient":" ","amount":10}]}`, 400, nil)
	bought := c.material("Bought", "commercial")
	c.call("PUT", fmt.Sprintf("/api/materials/%d/recipe", bought), `{"lines":[{"ingredient":"Silica","amount":1}]}`, 400, nil)
	c.call("PUT", fmt.Sprintf("/api/materials/%d", id), `{"name":"House celadon","kind":"commercial"}`, 409, nil)
}

func TestInUseIsArchivedNotDeleted(t *testing.T) {
	c := newClient(t)
	clay, used := c.clay("Stoneware"), c.material("Copper", "oxide")
	c.tile(1, fmt.Sprintf(`,"clay_body_id":%d,"layers":[{"material_id":%d}]`, clay, used))

	c.call("POST", "/api/clay-bodies", `{"name":"Stoneware"}`, 409, nil)
	c.call("POST", "/api/materials", `{"name":"Copper","kind":"oxide"}`, 409, nil)
	c.call("POST", "/api/materials", `{"name":"","kind":"oxide"}`, 400, nil)
	c.call("POST", "/api/materials", `{"name":"X","kind":"paint"}`, 400, nil)

	c.call("DELETE", fmt.Sprintf("/api/clay-bodies/%d", clay), nil, 409, nil)
	c.call("DELETE", fmt.Sprintf("/api/materials/%d", used), nil, 409, nil)

	var m MaterialDetail
	c.call("PUT", fmt.Sprintf("/api/materials/%d", used), `{"name":"Copper carbonate","kind":"oxide","archived":true}`, 200, &m)
	if !m.Archived {
		t.Error("not archived")
	}
	// The old record still shows it, under its new name.
	if got := c.list("number=1")[0]; got.Layers[0].Material != "Copper carbonate" {
		t.Errorf("layer = %+v", got.Layers[0])
	}

	unused := c.material("Unused", "own")
	c.call("PUT", fmt.Sprintf("/api/materials/%d/recipe", unused), `{"lines":[{"ingredient":"Silica","amount":1}]}`, 200, nil)
	c.call("DELETE", fmt.Sprintf("/api/materials/%d", unused), nil, 204, nil)
	c.call("GET", fmt.Sprintf("/api/materials/%d", unused), nil, 404, nil)
	empty := c.clay("Porcelain")
	c.call("DELETE", fmt.Sprintf("/api/clay-bodies/%d", empty), nil, 204, nil)
}

func TestRefireKeepsTheNumber(t *testing.T) {
	c := newClient(t)
	f1 := c.firing("one")
	first := c.tile(7, fmt.Sprintf(`,"firing_id":%d,"level":"bottom","rating":"fine"`, f1))
	c.unload(f1)
	f2 := c.firing("two")

	var second Piece
	c.call("POST", "/api/pieces", fmt.Sprintf(`{"kind":"tile","refire_of":%d}`, first.ID), 200, &second)
	if *second.Number != 7 || *second.RefireOf != first.ID || second.State != "bench" {
		t.Fatalf("re-fire = %+v", second)
	}

	// The number finds the latest record first, and each links to the other.
	found := c.list("number=7")
	if len(found) != 2 || found[0].ID != second.ID || *found[1].RefiredBy != second.ID {
		t.Errorf("found = %+v", found)
	}
	// The new record gets its own firing; the old one stays as it came out.
	c.call("PUT", fmt.Sprintf("/api/pieces/%d", second.ID), fmt.Sprintf(`{"kind":"tile","number":7,"refire_of":%d,"firing_id":%d}`, first.ID, f2), 200, nil)
	if old := found[1]; old.Firing.ID != f1 || *old.Rating != "fine" {
		t.Errorf("old record = %+v", old)
	}

	c.call("DELETE", fmt.Sprintf("/api/pieces/%d", first.ID), nil, 409, nil)
	c.call("POST", "/api/pieces", `{"kind":"tile","refire_of":9999}`, 400, nil)
	c.call("DELETE", fmt.Sprintf("/api/pieces/%d", second.ID), nil, 204, nil)
	c.call("DELETE", fmt.Sprintf("/api/pieces/%d", first.ID), nil, 204, nil)
}

func TestFilters(t *testing.T) {
	c := newClient(t)
	stone, porcelain := c.clay("Stoneware"), c.clay("Porcelain")
	copper, iron := c.material("Copper", "oxide"), c.material("Iron", "oxide")
	slow := c.firing("slow cool")
	c.tile(1, fmt.Sprintf(`,"clay_body_id":%d,"firing_id":%d,"level":"top","x":0,"y":0,"rating":"great","defects":["running"],"layers":[{"material_id":%d}]`, stone, slow, copper))
	c.unload(slow)
	fast := c.firing("fast")
	c.tile(2, fmt.Sprintf(`,"clay_body_id":%d,"firing_id":%d,"level":"bottom","rating":"failed","notes":"crawled badly","layers":[{"material_id":%d}]`, porcelain, fast, iron))
	c.tile(3, "")
	c.call("POST", "/api/pieces", `{"kind":"pot"}`, 200, nil)

	numbers := func(query string) string {
		var out []string
		for _, p := range c.list(query) {
			if p.Number == nil {
				out = append(out, "pot")
			} else {
				out = append(out, fmt.Sprint(*p.Number))
			}
		}
		return strings.Join(out, ",")
	}
	for query, want := range map[string]string{
		"":                               "pot,3,2,1",
		"kind=pot":                       "pot",
		fmt.Sprintf("material=%d", iron): "2",
		fmt.Sprintf("clay=%d", stone):    "1",
		"rating=great":                   "1",
		"defect=running":                 "1",
		"level=bottom":                   "2",
		"spot=edge":                      "1",
		"schedule=fast":                  "2",
		"state=bench":                    "pot,3",
		"state=kiln":                     "",
		"state=done":                     "2,1",
		"q=crawled":                      "2",
		"q=copper":                       "1",
		"q=3":                            "3",
		fmt.Sprintf("firing=%d", fast):   "2",
		"rating=great&level=bottom":      "",
	} {
		if got := numbers(query); got != want {
			t.Errorf("?%s = %q, want %q", query, got, want)
		}
	}
	c.call("GET", "/api/pieces?rating=nope", nil, 400, nil)
	c.call("GET", "/api/pieces?state=nope", nil, 400, nil)

	var s map[string][]string
	c.call("GET", "/api/suggestions", nil, 200, &s)
	if strings.Join(s["schedules"], ",") != "fast,slow cool" {
		t.Errorf("suggestions = %v", s)
	}
}

func TestKilnPosition(t *testing.T) {
	c := newClient(t)
	f := c.firing("x")
	place := func(number int64, position string, want int) Piece {
		t.Helper()
		var p Piece
		c.call("POST", "/api/pieces", fmt.Sprintf(`{"kind":"tile","number":%d,"firing_id":%d,"level":"top",%s}`, number, f, position), want, &p)
		return p
	}
	if p := place(1, `"x":0.5,"y":0.5`, 200); *p.Spot != "middle" {
		t.Errorf("centre of the shelf = %q", *p.Spot)
	}
	for i, near := range []string{`"x":0.1,"y":0.5`, `"x":0.9,"y":0.5`, `"x":0.5,"y":0.1`, `"x":0.5,"y":0.9`} {
		if p := place(int64(i+2), near, 200); *p.Spot != "edge" {
			t.Errorf("%s = %q", near, *p.Spot)
		}
	}
	// Anywhere on the shelf is fine, including right up against something else.
	if p := place(10, `"x":0.5004,"y":0.5`, 200); *p.X != 0.5 {
		t.Errorf("x = %v, want it rounded", *p.X)
	}
	place(11, `"x":1,"y":1`, 200)
	place(12, `"x":0,"y":0`, 200)

	place(20, `"x":1.1,"y":0`, 400)
	place(21, `"x":0,"y":1.5`, 400)
	place(22, `"x":-0.1,"y":0`, 400)
	place(23, `"x":0.5`, 400)                                                            // half a position
	c.call("POST", "/api/pieces", `{"kind":"tile","number":24,"spot":"edge"}`, 400, nil) // worked out, never sent
}

func TestFiringListIsInShelfOrder(t *testing.T) {
	c := newClient(t)
	f := c.firing("x")
	c.tile(1, fmt.Sprintf(`,"firing_id":%d,"level":"top"`, f))
	c.tile(2, fmt.Sprintf(`,"firing_id":%d,"level":"bottom"`, f))
	c.tile(3, fmt.Sprintf(`,"firing_id":%d,"level":"middle"`, f))
	var order []int64
	for _, p := range c.list(fmt.Sprintf("firing=%d", f)) {
		order = append(order, *p.Number)
	}
	if fmt.Sprint(order) != "[2 3 1]" {
		t.Errorf("order = %v", order)
	}
}

func TestPhotos(t *testing.T) {
	c := newClient(t)
	p := c.tile(1, "")
	path := fmt.Sprintf("/api/pieces/%d/photos", p.ID)

	var ph Photo
	c.call("POST", path+"?stage=after", jpegBytes(t, 3000, 2000), 200, &ph)
	c.call("POST", path, jpegBytes(t, 10, 10), 400, nil) // no stage
	c.call("POST", path+"?stage=after", []byte("not a photo"), 400, nil)
	c.call("POST", "/api/pieces/999/photos?stage=after", jpegBytes(t, 10, 10), 404, nil)

	size := func(url string) image.Point {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(c.call("GET", url, nil, 200, nil)))
		if err != nil {
			t.Fatal(err)
		}
		return image.Pt(cfg.Width, cfg.Height)
	}
	if got := size(fmt.Sprintf("/api/photos/%d", ph.ID)); got.X != 1600 {
		t.Errorf("photo size = %v", got)
	}
	if got := size(fmt.Sprintf("/api/photos/%d/thumb", ph.ID)); got.X != 400 {
		t.Errorf("thumbnail size = %v", got)
	}
	if got := c.list("number=1")[0]; len(got.Photos) != 1 || got.Photos[0].Stage != "after" {
		t.Errorf("photos = %+v", got.Photos)
	}

	c.call("DELETE", fmt.Sprintf("/api/photos/%d", ph.ID), nil, 204, nil)
	c.call("GET", fmt.Sprintf("/api/photos/%d", ph.ID), nil, 404, nil)
	if _, err := os.Stat(thumbFile(c.app, ph.ID)); !os.IsNotExist(err) {
		t.Errorf("thumbnail left behind: %v", err)
	}

	// A new photo never takes the id of a deleted one, because they're cached by id.
	var next Photo
	c.call("POST", path+"?stage=before", jpegBytes(t, 10, 10), 200, &next)
	if next.ID == ph.ID {
		t.Error("photo id was reused")
	}

	// Deleting the piece takes its files with it.
	c.call("DELETE", fmt.Sprintf("/api/pieces/%d", p.ID), nil, 204, nil)
	if _, err := os.Stat(photoFile(c.app, next.ID)); !os.IsNotExist(err) {
		t.Errorf("photo left behind: %v", err)
	}
}

func TestSummaryText(t *testing.T) {
	for _, c := range []struct {
		bench, kiln, waiting int
		want                 string
	}{
		{0, 0, 0, "Nothing waiting"},
		{2, 0, 0, "2 to be glazed"},
		{0, 4, 0, "4 in the kiln"},
		{0, 0, 3, "3 waiting for a result"},
		{2, 4, 3, "2 to be glazed · 4 in the kiln · 3 waiting for a result"},
	} {
		if got := summaryText(c.bench, c.kiln, c.waiting); got != c.want {
			t.Errorf("summaryText(%d, %d, %d) = %q", c.bench, c.kiln, c.waiting, got)
		}
	}
}

func TestOneLoadAtATime(t *testing.T) {
	c := newClient(t)
	f := c.firing("slow")
	state := fmt.Sprintf("/api/firings/%d/state", f)
	c.call("POST", "/api/firings", `{}`, 409, nil) // there's one kiln

	// A date is optional at first, and an empty kiln can't start.
	var cancelled Firing
	c.call("PUT", state, `{"state":"firing"}`, 409, nil)
	c.call("PUT", state, `{"state":"unloaded"}`, 409, nil) // has to fire first
	c.call("DELETE", fmt.Sprintf("/api/firings/%d", f), nil, 204, nil)
	c.call("POST", "/api/firings", `{}`, 200, &cancelled)
	if cancelled.FiredOn != nil || cancelled.State != "loading" {
		t.Errorf("new load = %+v", cancelled)
	}
	f = cancelled.ID
	state = fmt.Sprintf("/api/firings/%d/state", f)

	p := c.tile(1, fmt.Sprintf(`,"firing_id":%d,"level":"top","x":0,"y":0`, f))
	c.call("POST", "/api/pieces", `{"kind":"tile","number":2,"firing_id":99}`, 400, nil)

	var got Firing
	c.call("PUT", state, `{"state":"firing"}`, 200, &got)
	if got.State != "firing" || got.FiredOn == nil {
		t.Errorf("started = %+v", got)
	}
	// Once it's firing nothing else can go in, but what's in it can be saved.
	c.call("POST", "/api/pieces", fmt.Sprintf(`{"kind":"tile","number":2,"firing_id":%d}`, f), 409, nil)
	c.call("PUT", fmt.Sprintf("/api/pieces/%d", p.ID), fmt.Sprintf(`{"kind":"tile","number":1,"firing_id":%d,"level":"top","x":1,"y":0,"notes":"moved"}`, f), 200, nil)

	// A mistaken tap can be stepped back.
	c.call("PUT", state, `{"state":"loading"}`, 200, nil)
	c.call("PUT", state, `{"state":"firing"}`, 200, nil)
	c.call("PUT", state, `{"state":"unloaded"}`, 200, nil)
	next := c.firing("next")                             // the kiln is free again
	c.call("PUT", state, `{"state":"firing"}`, 409, nil) // and now it's taken
	c.call("DELETE", fmt.Sprintf("/api/firings/%d", next), nil, 204, nil)
	c.call("PUT", state, `{"state":"firing"}`, 200, nil)
	c.call("PUT", state, `{"state":"nonsense"}`, 409, nil)
}

func TestHousekeeping(t *testing.T) {
	c := newClient(t)
	backup := c.call("GET", "/api/backup", nil, 200, nil)
	if !bytes.HasPrefix(backup, []byte("SQLite format 3\x00")) {
		t.Error("backup isn't a database")
	}
	c.call("GET", "/api/nope", nil, 404, nil)
	c.call("POST", "/api/summary", nil, 404, nil)
	if got := c.call("GET", "/pieces/3", nil, 200, nil); string(got) != "the app" {
		t.Errorf("GET /pieces/3 = %q", got)
	}
}
