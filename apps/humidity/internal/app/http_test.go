package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"atelier/apps/humidity/internal/hw"

	"atelier/shared/kit"
)

// TestHTTP walks through the API with the sim sensor and pump and a fresh
// database, the way the web app uses it.
func TestHTTP(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	live, err := NewLive(db, []string{"0x44"})
	if err != nil {
		t.Fatal(err)
	}
	simPump := &hw.SimPump{}
	a := &App{DB: db, DataDir: dir, MaxRH: 95, Live: live,
		Pump: NewPump(simPump, live, 10*time.Second, 3*time.Minute, 95),
		Web:  fstest.MapFS{"index.html": {Data: []byte("the app")}}}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { a.Pump.Loop(ctx) })
	wg.Go(func() { a.SensorLoop(ctx, []hw.Sensor{hw.NewSimSensor(simPump)}) })
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		srv.Close()
		cancel()
		wg.Wait()
		db.Close()
	})

	// call sends body (JSON text, or raw bytes) and checks the status code;
	// it decodes a JSON answer into out when out isn't nil.
	call := func(method, path string, body any, want int, out any) *http.Response {
		t.Helper()
		var r io.Reader
		switch b := body.(type) {
		case string:
			r = strings.NewReader(b)
		case []byte:
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, srv.URL+path, r)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("%s %s: %d %s, want %d", method, path, resp.StatusCode, data, want)
		}
		if resp.StatusCode >= 400 {
			var e struct{ Error string }
			if json.Unmarshal(data, &e) != nil || e.Error == "" {
				t.Fatalf("%s %s: error body %q is not {\"error\": …}", method, path, data)
			}
		}
		if out != nil {
			if err := json.Unmarshal(data, out); err != nil {
				t.Fatalf("%s %s: %v in %s", method, path, err, data)
			}
		}
		resp.Body = io.NopCloser(bytes.NewReader(data))
		return resp
	}

	// Now, with its nulls.
	var raw map[string]any
	call("GET", "/api/now", nil, 200, &raw)
	for _, key := range []string{"room", "sensors", "target", "max_rh", "pump", "learning"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("now has no %q: %v", key, raw)
		}
	}
	if raw["target"] != nil || raw["max_rh"] != 95.0 || raw["learning"] != true {
		t.Errorf("now = %v", raw)
	}

	// Target.
	var now Now
	call("PUT", "/api/target", `{"pct": 82}`, 200, &now)
	if now.Target == nil || *now.Target != 82 {
		t.Errorf("target = %v, want 82", now.Target)
	}
	// The sim shelf sits near 60 %, so the pump starts in the same reply.
	if now.Pump.Status != "running" || !simPump.On() {
		t.Errorf("pump after target 82 = %+v, want running", now.Pump)
	}
	call("PUT", "/api/target", `{"pct": 95}`, 400, nil)
	call("PUT", "/api/target", `{"pct": 82.5}`, 400, nil)
	call("PUT", "/api/target", `{"pct": null}`, 200, &now)
	if now.Target != nil {
		t.Errorf("target = %v, want null", *now.Target)
	}

	// Pump.
	call("POST", "/api/pump/run", `{"seconds": 0}`, 400, nil)
	call("POST", "/api/pump/run", `{"seconds": 30}`, 200, &now)
	if !now.Pump.Enabled || !now.Pump.On || now.Pump.Status != "running" || now.Pump.LastRunAt == nil || !simPump.On() {
		t.Errorf("after run: %+v", now.Pump)
	}
	call("POST", "/api/pump/stop", nil, 200, &now)
	if now.Pump.Status != "stopped" || *now.Pump.Reason != "user" || simPump.On() {
		t.Errorf("after stop: %+v", now.Pump)
	}
	call("POST", "/api/pump/resume", nil, 200, &now)
	if now.Pump.Status != "idle" || *now.Pump.Reason == "user" {
		t.Errorf("after resume: %+v", now.Pump)
	}

	// Sensor offset.
	call("PUT", "/api/sensors/0x44/offset", `{"offset": 1.5}`, 200, &now)
	if len(now.Sensors) != 1 || now.Sensors[0].Offset != 1.5 {
		t.Errorf("sensors = %+v", now.Sensors)
	}
	call("PUT", "/api/sensors/0x45/offset", `{"offset": 1.5}`, 404, nil)
	call("PUT", "/api/sensors/0x44/offset", `{"offset": 11}`, 400, nil)

	// Add pieces.
	call("POST", "/api/pieces", `{"form": "bowl", "size": "medium", "thickness": "medium"}`, 400, nil)
	call("POST", "/api/pieces", `{"form": "cup", "size": "medium", "thickness": "medium", "dryness": 1}`, 400, nil)
	call("POST", "/api/pieces", `{"form": "bowl", "size": "medium", "thickness": "medium", "dryness": 1, "quantity": 21}`, 400, nil)
	var created []map[string]any
	call("POST", "/api/pieces", `{"form": "bowl", "size": "medium", "thickness": "medium", "dryness": 1.04, "label": "blue", "quantity": 3}`, 200, &created)
	if len(created) != 3 {
		t.Fatalf("created %d pieces, want 3", len(created))
	}
	for i, p := range created {
		if p["number"] != float64(i+1) || p["dryness"] != 1.0 || p["state"] != "in" || p["label"] != "blue" {
			t.Errorf("created[%d] = %v", i, p)
		}
		if _, ok := p["checks"]; ok {
			t.Errorf("created[%d] has checks", i)
		}
		if e, _ := p["estimate"].(map[string]any); e["status"] != "drying" || e["ready_at"] == nil {
			t.Errorf("created[%d] estimate = %v", i, p["estimate"])
		}
	}
	id := int64(created[0]["id"].(float64))
	piecePath := fmt.Sprintf("/api/pieces/%d", id)

	// A higher target means damper air and slower drying: the estimate moves
	// later as soon as the target changes, with no new check.
	readyAt := func(pct string) time.Time {
		call("PUT", "/api/target", `{"pct": `+pct+`}`, 200, nil)
		var p Piece
		call("GET", piecePath, nil, 200, &p)
		if p.Estimate == nil || p.Estimate.ReadyAt == nil {
			t.Fatalf("estimate at target %s = %+v", pct, p.Estimate)
		}
		return *p.Estimate.ReadyAt
	}
	at70, at90 := readyAt("70"), readyAt("90")
	if !at90.After(at70.Add(24 * time.Hour)) {
		t.Errorf("ready at 70 %% = %v, at 90 %% = %v; want 90 %% much later", at70, at90)
	}
	call("PUT", "/api/target", `{"pct": null}`, 200, nil)

	// Photo.
	// A camera-sized photo is stored shrunk to photoEdge on its long edge.
	var camera bytes.Buffer
	if err := jpeg.Encode(&camera, image.NewRGBA(image.Rect(0, 0, 4000, 3000)), nil); err != nil {
		t.Fatal(err)
	}
	call("PUT", piecePath+"/photo", []byte("GIF89a"), 400, nil)
	call("PUT", piecePath+"/photo", append(camera.Bytes(), make([]byte, kit.MaxPhoto)...), 413, nil)
	call("PUT", piecePath+"/photo", camera.Bytes(), 204, nil)
	resp := call("GET", piecePath+"/photo", nil, 200, nil)
	stored, err := jpeg.DecodeConfig(resp.Body)
	if err != nil || stored.Width != photoEdge || stored.Height != 1200 || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("photo: %s, %dx%d, %v", resp.Header.Get("Content-Type"), stored.Width, stored.Height, err)
	}
	call("GET", fmt.Sprintf("/api/pieces/%d/photo", id+1), nil, 404, nil)

	// Checks.
	var p Piece
	call("POST", piecePath+"/checks", `{"dryness": 2.44}`, 200, &p)
	if !p.Photo || len(p.Checks) != 2 || p.LastCheck.Dryness != 2.4 || p.Checks[1].Dryness != 2.4 {
		t.Errorf("after check: %+v", p)
	}
	call("POST", piecePath+"/checks", `{"dryness": 5.1}`, 400, nil)
	call("POST", "/api/pieces/999/checks", `{"dryness": 2}`, 404, nil)

	// Flip and undo.
	call("PATCH", piecePath, `{"state": "flipped"}`, 200, &p)
	if p.State != "flipped" || p.FlippedAt == nil {
		t.Errorf("after flip: %+v", p)
	}
	call("PATCH", piecePath, `{"state": "flipped"}`, 409, nil)
	call("PATCH", piecePath, `{"state": "in"}`, 200, &p)
	if p.State != "in" || p.FlippedAt != nil {
		t.Errorf("after undo flip: %+v", p)
	}
	call("PATCH", piecePath, `{"state": "flipped"}`, 200, &p)

	// Edits.
	call("PATCH", piecePath, `{"label": null}`, 400, nil)
	call("PATCH", piecePath, `{"dryness": 3}`, 400, nil)
	call("PATCH", piecePath, `{"size": "huge"}`, 400, nil)
	call("PATCH", piecePath, `{"label": "", "notes": "slip on rim", "size": "large"}`, 200, &p)
	if p.Label != "" || p.Notes != "slip on rim" || p.Size != "large" {
		t.Errorf("after edit: %+v", p)
	}

	// Trim, then put back after its number was taken.
	call("PATCH", piecePath, `{"state": "trimmed"}`, 200, &p)
	if p.State != "trimmed" || p.EndedAt == nil || p.Estimate != nil || p.Dryness != 3.0 || len(p.Checks) != 3 {
		t.Errorf("after trim: %+v", p)
	}
	call("POST", piecePath+"/checks", `{"dryness": 3}`, 409, nil)
	call("PATCH", piecePath, `{"state": "discarded"}`, 409, nil)
	var list []map[string]any
	call("GET", "/api/pieces?ended=1", nil, 200, &list)
	if len(list) != 1 || list[0]["id"] != float64(id) {
		t.Errorf("ended pieces = %v", list)
	}
	call("POST", "/api/pieces", `{"form": "mug", "size": "small", "thickness": "thin", "dryness": 3.0}`, 200, &created)
	if created[0]["number"] != 1.0 {
		t.Errorf("new piece number = %v, want 1 (the trimmed piece's)", created[0]["number"])
	}
	call("PATCH", piecePath, `{"state": "in"}`, 200, &p)
	if p.State != "flipped" || p.EndedAt != nil || p.Number != 4 || len(p.Checks) != 2 || p.Estimate == nil {
		t.Errorf("after put back: %+v", p)
	}

	// Discard with a dryness, then put back: the check goes again.
	call("PATCH", piecePath, `{"state": "discarded", "dryness": 4.2}`, 200, &p)
	if p.State != "discarded" || p.LastCheck.Dryness != 4.2 {
		t.Errorf("after discard: %+v", p)
	}
	call("PATCH", piecePath, `{"state": "in"}`, 200, &p)
	if len(p.Checks) != 2 {
		t.Errorf("after put back: %d checks, want 2", len(p.Checks))
	}

	// The shelf's order: past, then ready, then drying.
	call("POST", "/api/pieces", `{"form": "vase", "size": "xlarge", "thickness": "thick", "dryness": 4.5}`, 200, nil)
	call("GET", "/api/pieces", nil, 200, &list)
	var order []string
	for _, p := range list {
		order = append(order, p["estimate"].(map[string]any)["status"].(string))
	}
	if strings.Join(order, " ") != "past ready drying drying drying" {
		t.Errorf("shelf order = %v", order)
	}

	// Delete checks.
	first := p.Checks[0].ID
	call("DELETE", fmt.Sprintf("/api/checks/%d", p.Checks[1].ID), nil, 200, &p)
	if len(p.Checks) != 1 {
		t.Errorf("after deleting a check: %d checks", len(p.Checks))
	}
	call("DELETE", fmt.Sprintf("/api/checks/%d", first), nil, 409, nil)
	call("DELETE", "/api/checks/999", nil, 404, nil)

	// Delete the piece and its photo.
	call("DELETE", piecePath, nil, 204, nil)
	call("GET", piecePath, nil, 404, nil)
	if _, err := os.Stat(a.photoPath(id)); !os.IsNotExist(err) {
		t.Errorf("photo still there: %v", err)
	}

	// History.
	nowMin := time.Now().Unix() / 60 * 60
	for i, m := range []int64{nowMin - 3600, nowMin - 3540, nowMin - 600} {
		if err := saveMinute(db, m, 80+float64(i), 18); err != nil {
			t.Fatal(err)
		}
	}
	var h History
	call("GET", "/api/history?hours=24", nil, 200, &h)
	if h.Step != 300 || len(h.T) < 2 || len(h.T) != len(h.RH) || len(h.T) != len(h.Temp) {
		t.Fatalf("history = %+v", h)
	}
	for i, ts := range h.T {
		if ts%300 != 0 || (i > 0 && ts <= h.T[i-1]) {
			t.Errorf("history t = %v", h.T)
		}
	}
	call("GET", "/api/history?hours=1", nil, 200, &h)
	if h.Step != 60 || len(h.T) != 2 { // the readings 59 and 10 min ago; the one 60 min ago is out
		t.Errorf("history 1 h = %+v", h)
	}
	call("GET", "/api/history?hours=168", nil, 200, &h)
	if h.Step != 1800 {
		t.Errorf("history step = %d, want 1800", h.Step)
	}
	call("GET", "/api/history?hours=5", nil, 400, nil)

	// Backup.
	resp = call("GET", "/api/backup", nil, 200, nil)
	body, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(body, []byte("SQLite format 3\x00")) ||
		!strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Errorf("backup: %q, %d bytes", resp.Header.Get("Content-Disposition"), len(body))
	}

	// The homepage tile.
	var summary struct{ Text string }
	call("GET", "/api/summary", nil, 200, &summary)
	if summary.Text == "" {
		t.Error("summary has no text")
	}

	// Unknown API paths get JSON; anything else gets the app.
	call("GET", "/api/nope", nil, 404, nil)
	call("POST", "/api/now", nil, 404, nil)
	resp = call("GET", "/pieces/3", nil, 200, nil)
	if body, _ := io.ReadAll(resp.Body); string(body) != "the app" {
		t.Errorf("GET /pieces/3 = %q", body)
	}
}

func TestShelfSummary(t *testing.T) {
	cases := []struct {
		onShelf, ready int
		want           string
	}{
		{0, 0, "shelf empty"},
		{1, 0, "1 piece"},
		{1, 1, "1 piece, ready to trim"},
		{3, 0, "3 pieces"},
		{3, 2, "3 pieces, 2 ready to trim"},
	}
	for _, c := range cases {
		if got := shelfSummary(c.onShelf, c.ready); got != c.want {
			t.Errorf("shelfSummary(%d, %d) = %q, want %q", c.onShelf, c.ready, got, c.want)
		}
	}
}
