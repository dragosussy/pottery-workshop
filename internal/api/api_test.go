package api

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"humidity/internal/control"
	"humidity/internal/monitor"
	"humidity/internal/photos"
	"humidity/internal/recorder"
	"humidity/internal/sensor"
	"humidity/internal/sensor/sim"
	"humidity/internal/shelf"
	"humidity/internal/store"
)

type harness struct {
	t   *testing.T
	srv *httptest.Server
	st  *store.Store
	hub *Hub
	mon *monitor.Monitor
}

func newHarness(t *testing.T) *harness { return newHarnessWith(t, false) }

// newHarnessWith optionally adds a controller with a simulated pump.
func newHarnessWith(t *testing.T, pump bool) *harness {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mon := monitor.New([]sensor.Sensor{sim.New("main")}, time.Hour, log)
	hub := NewHub()
	dose := recorder.NewDoseIndex(time.Now)
	svc := shelf.New(st, dose, hub, mon, []string{"main"}, "UTC")
	ph, err := photos.New(st, filepath.Join(dir, "photos"), log)
	if err != nil {
		t.Fatal(err)
	}
	var ctl *control.Controller
	if pump {
		ctl = control.New(sim.NewPump("pump"), control.Config{}, shelf.ControlEvents{S: svc}, log)
		svc.OnTarget = ctl.SetTarget
	}
	_, handler := New(Deps{
		Store: st, Monitor: mon, Shelf: svc, Control: ctl, Photos: ph, Hub: hub, ClockSynced: func() bool { return true },
		Web:     fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}, "assets/app-1234.js": {Data: []byte("js")}},
		TempDir: dir, Version: "test", Commit: "abc", StartedAt: time.Now(), Log: log,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { hub.Close(); srv.Close() })
	// One poll so there is a reading.
	ctx, cancel := context.WithCancel(context.Background())
	go mon.Run(ctx)
	t.Cleanup(cancel)
	for len(mon.Latest()) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	return &harness{t: t, srv: srv, st: st, hub: hub, mon: mon}
}

// do sends a request and decodes a JSON response into a generic map.
func (h *harness) do(method, path, body string) (int, map[string]any) {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(raw, &out); err != nil {
			h.t.Fatalf("%s %s: bad JSON %q", method, path, raw)
		}
	}
	return resp.StatusCode, out
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func wantKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := keys(m); !slices.Equal(got, want) {
		t.Errorf("%s keys = %v\nwant %v", what, got, want)
	}
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func fieldCode(m map[string]any, field string) string {
	e, _ := m["error"].(map[string]any)
	fs, _ := e["fields"].([]any)
	for _, f := range fs {
		fm := f.(map[string]any)
		if fm["field"] == field {
			return fm["code"].(string)
		}
	}
	return ""
}

func TestHealthReferenceStatusShapes(t *testing.T) {
	h := newHarness(t)
	code, body := h.do("GET", "/api/v1/health", "")
	if code != 200 || body["status"] != "ok" || body["clock_synced"] != true {
		t.Errorf("health %d %v", code, body)
	}
	wantKeys(t, "health", body, "status", "version", "commit", "built_at", "api_version", "server_time", "clock_synced", "started_at", "checks", "features")

	code, body = h.do("GET", "/api/v1/reference", "")
	if code != 200 {
		t.Fatal(code)
	}
	wantKeys(t, "reference", body, "dryness", "stages", "forms", "sizes", "thicknesses", "piece_states", "prediction_statuses", "uncertainty_reasons", "check_reasons")

	code, body = h.do("GET", "/api/v1/status", "")
	if code != 200 {
		t.Fatal(code)
	}
	wantKeys(t, "status", body, "server_time", "poll_interval_seconds", "stale_after_seconds", "room", "sensors", "shelf", "trim", "learning", "last_offline", "control")
	wantKeys(t, "room", body["room"].(map[string]any), "humidity_pct", "temperature_c", "vpd_kpa", "state", "read_at")
	wantKeys(t, "sensor", body["sensors"].([]any)[0].(map[string]any), "sensor", "state", "humidity_pct", "temperature_c", "offset_pct", "read_at", "age_seconds", "failing_since", "last_error")
	wantKeys(t, "trim", body["trim"].(map[string]any), "ready_now_count", "ready_now_until", "check_count", "late_count", "next_visit")
	wantKeys(t, "control", body["control"].(map[string]any), "target_humidity_pct", "set_at", "min_pct", "max_pct", "controller", "pump", "safety", "limits")
	if body["control"].(map[string]any)["controller"] != false || body["control"].(map[string]any)["pump"] != nil {
		t.Errorf("control without a pump = %v", body["control"])
	}
	if body["room"].(map[string]any)["state"] != "ok" {
		t.Errorf("status = %v", body)
	}
}

func TestRoutingAndErrors(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"GET", "/api/v1/nope", "", 404, "not_found"},
		{"DELETE", "/api/v1/health", "", 404, "not_found"},
		{"POST", "/api/v1/control/override", `{"seconds":5}`, 409, "no_pump"},
		{"POST", "/api/v1/control/reset", "", 409, "no_pump"},
		{"PUT", "/api/v1/control/setpoint", "{}", 422, "validation_failed"},
		{"PUT", "/api/v1/control/setpoint", `{"target_humidity_pct":30}`, 422, "validation_failed"},
		{"GET", "/api/v1/control/preview?target_humidity_pct=x", "", 400, "bad_request"},
		{"GET", "/api/v1/pieces/abc", "", 404, "not_found"},
		{"GET", "/api/v1/pieces/42", "", 404, "not_found"},
		{"POST", "/api/v1/pieces", "{", 400, "bad_request"},
		{"POST", "/api/v1/pieces", `{"form":"mug","colour":"blue"}`, 422, "validation_failed"},
		{"POST", "/api/v1/pieces", `{"form":3}`, 422, "validation_failed"},
		{"GET", "/api/v1/pieces?state=lost", "", 422, "validation_failed"},
		{"GET", "/api/v1/observations?cursor=zzz", "", 400, "bad_request"},
		{"GET", "/api/v1/observations?limit=500", "", 400, "bad_request"},
		{"GET", "/api/v1/readings", "", 422, "validation_failed"},
		{"GET", "/api/v1/readings?from=-30d&resolution=1m", "", 422, "validation_failed"},
		{"POST", "/api/v1/pieces", `{"notes":"` + strings.Repeat("x", 70000) + `"}`, 413, "too_large"},
	} {
		code, body := h.do(tc.method, tc.path, tc.body)
		if code != tc.status || errCode(body) != tc.code {
			t.Errorf("%s %s: %d %v, want %d %s", tc.method, tc.path, code, body, tc.status, tc.code)
		}
		if e, ok := body["error"].(map[string]any); ok {
			wantKeys(t, "error", e, "code", "message", "fields")
		}
	}
	_, body := h.do("POST", "/api/v1/pieces", `{"form":"mug","colour":"blue"}`)
	if fieldCode(body, "colour") != "unknown_field" {
		t.Errorf("unknown field: %v", body)
	}
	_, body = h.do("POST", "/api/v1/pieces", `{"form":3}`)
	if fieldCode(body, "form") != "invalid_value" {
		t.Errorf("type error: %v", body)
	}
	_, body = h.do("GET", "/api/v1/readings?from=-30d&resolution=1m", "")
	if fieldCode(body, "resolution") != "too_many_points" {
		t.Errorf("too many points: %v", body)
	}
}

func TestPiecesAndObservationsFlow(t *testing.T) {
	h := newHarness(t)
	code, body := h.do("POST", "/api/v1/pieces", `{"quantity":2,"form":"bowl","initial_dryness":1.23,"size":"large","thickness":"thick","added_at":"2026-10-30T08:00:00+02:00"}`)
	if code != 201 {
		t.Fatalf("add: %d %v", code, body)
	}
	items := body["items"].([]any)
	p := items[0].(map[string]any)
	wantKeys(t, "piece", p, "id", "number", "group_id", "group_size", "form", "label", "size", "thickness", "photo_id", "notes", "state",
		"added_at", "flipped_at", "flip_dryness", "ended_at", "end_dryness", "end_note", "mistake", "initial_dryness", "current_dryness",
		"last_observed_at", "observation_count", "prediction")
	wantKeys(t, "prediction", p["prediction"].(map[string]any), "status", "eta", "eta_low", "eta_high", "ready_until", "dryness_now",
		"confidence", "uncertainty_reasons", "forecast_basis", "basis", "anchor", "computed_at")
	if p["added_at"] != "2026-10-30T06:00:00Z" || p["initial_dryness"] != 1.2 || p["group_size"] != 2.0 {
		t.Errorf("piece = %v", p)
	}

	code, body = h.do("PATCH", "/api/v1/pieces", `{"ids":[1,2],"state":"flipped","flip_dryness":2.0}`)
	if code != 200 || len(body["items"].([]any)) != 2 {
		t.Fatalf("bulk flip: %d %v", code, body)
	}
	code, body = h.do("PATCH", "/api/v1/pieces/1", `{"state":"trimmed","end_dryness":3.1,"end_note":"lovely"}`)
	if code != 200 || body["state"] != "trimmed" || body["end_dryness"] != 3.1 || body["prediction"].(map[string]any)["status"] != "ended" {
		t.Errorf("trim: %d %v", code, body)
	}
	code, body = h.do("PATCH", "/api/v1/pieces/1", `{"state":"in"}`)
	if code != 409 || errCode(body) != "invalid_transition" {
		t.Errorf("undo to the wrong state: %d %v", code, body)
	}
	code, body = h.do("PATCH", "/api/v1/pieces/1", `{"label":null}`)
	if code != 422 || fieldCode(body, "label") != "invalid_value" {
		t.Errorf("null label: %d %v", code, body)
	}

	code, body = h.do("GET", "/api/v1/pieces?state=ended", "")
	if code != 200 || len(body["items"].([]any)) != 1 || body["next_cursor"] != nil {
		t.Errorf("ended: %d %v", code, body)
	}
	code, body = h.do("GET", "/api/v1/pieces?state=all&limit=1", "")
	if code != 200 || body["next_cursor"] == nil {
		t.Fatalf("all page 1: %d %v", code, body)
	}
	code, body = h.do("GET", "/api/v1/pieces?state=all&limit=1&cursor="+body["next_cursor"].(string), "")
	if code != 200 || len(body["items"].([]any)) != 1 || body["next_cursor"] != nil {
		t.Errorf("all page 2: %d %v", code, body)
	}
	code, body = h.do("GET", "/api/v1/pieces", "")
	if code != 200 || len(body["items"].([]any)) != 1 {
		t.Errorf("on shelf: %d %v", code, body)
	}

	code, body = h.do("POST", "/api/v1/observations", `{"items":[{"piece_id":2,"dryness":2.5}],"note":"foot soft"}`)
	if code != 201 {
		t.Fatalf("observe: %d %v", code, body)
	}
	wantKeys(t, "created", body, "items", "follow_ups")
	o := body["items"].([]any)[0].(map[string]any)
	wantKeys(t, "observation", o, "id", "piece_id", "dryness", "observed_at", "source", "flag", "note", "created_at")
	code, body = h.do("POST", "/api/v1/observations", `{"items":[{"piece_id":1,"dryness":2.5}]}`)
	if code != 409 || errCode(body) != "piece_ended" {
		t.Errorf("observe ended: %d %v", code, body)
	}
	code, body = h.do("POST", "/api/v1/observations", `{"items":[{"piece_id":2,"dryness":0.5}]}`)
	if code != 201 || len(body["follow_ups"].([]any)) != 1 {
		t.Fatalf("went wetter: %d %v", code, body)
	}
	fu := body["follow_ups"].([]any)[0].(map[string]any)
	wantKeys(t, "follow-up", fu, "kind", "piece_id", "observation_id", "previous_observation_id", "question", "options")
	oid := strconv.Itoa(int(fu["observation_id"].(float64)))
	code, body = h.do("POST", "/api/v1/observations/"+oid+"/follow-up", `{"answer":"last_entry_wrong"}`)
	if code != 200 || len(body) != 0 {
		t.Errorf("answer: %d %v", code, body)
	}

	code, body = h.do("PATCH", "/api/v1/observations/"+oid, `{"dryness":2.6}`)
	if code != 200 || body["dryness"] != 2.6 {
		t.Errorf("patch obs: %d %v", code, body)
	}
	if code, _ := h.do("DELETE", "/api/v1/observations/"+oid, ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code, body := h.do("POST", "/api/v1/observations/"+oid+"/undelete", ""); code != 200 || body["id"] != fu["observation_id"] {
		t.Errorf("undelete: %d %v", code, body)
	}
	if code, body := h.do("POST", "/api/v1/observations/"+oid+"/undelete", `{"x":1}`); code != 422 {
		t.Errorf("undelete with a field: %d %v", code, body)
	}
	code, body = h.do("GET", "/api/v1/pieces/2/observations", "")
	if code != 200 || len(body["items"].([]any)) != 4 {
		t.Errorf("piece observations: %d %v", code, body)
	}
	code, body = h.do("GET", "/api/v1/observations?limit=2", "")
	if code != 200 || len(body["items"].([]any)) != 2 || body["next_cursor"] == nil {
		t.Errorf("recent: %d %v", code, body)
	}

	code, body = h.do("GET", "/api/v1/shelf", "")
	if code != 200 {
		t.Fatal(code)
	}
	wantKeys(t, "shelf", body, "computed_at", "items", "next_visit", "check_these")
}

func TestControlTarget(t *testing.T) {
	h := newHarness(t)
	code, body := h.do("GET", "/api/v1/control", "")
	if code != 200 || body["target_humidity_pct"] != nil {
		t.Fatalf("initial control: %d %v", code, body)
	}
	code, body = h.do("PUT", "/api/v1/control/setpoint", `{"target_humidity_pct":82.04}`)
	if code != 200 || body["target_humidity_pct"] != 82.0 || body["set_at"] == nil {
		t.Fatalf("set: %d %v", code, body)
	}
	code, body = h.do("GET", "/api/v1/control/preview?target_humidity_pct=90", "")
	if code != 200 || body["target_humidity_pct"] != 90.0 {
		t.Fatalf("preview: %d %v", code, body)
	}
	wantKeys(t, "preview", body, "target_humidity_pct", "forecast_temp_c", "next_visit", "ready_now_count")
	// The preview saved nothing.
	if _, body = h.do("GET", "/api/v1/control", ""); body["target_humidity_pct"] != 82.0 {
		t.Errorf("after preview: %v", body)
	}
	code, body = h.do("PUT", "/api/v1/control/setpoint", `{"target_humidity_pct":null}`)
	if code != 200 || body["target_humidity_pct"] != nil {
		t.Fatalf("clear: %d %v", code, body)
	}
	_, body = h.do("GET", "/api/v1/climate-log?from=-1h", "")
	n := 0
	for _, e := range body["items"].([]any) {
		if e.(map[string]any)["kind"] == "setpoint_set" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("setpoint_set entries = %d, want 2: %v", n, body)
	}
}

func TestControlPump(t *testing.T) {
	h := newHarnessWith(t, true)
	_, body := h.do("GET", "/api/v1/health", "")
	if body["features"].(map[string]any)["control"] != true {
		t.Errorf("features = %v", body["features"])
	}
	code, body := h.do("GET", "/api/v1/control", "")
	if code != 200 || body["controller"] != true {
		t.Fatalf("control: %d %v", code, body)
	}
	wantKeys(t, "pump", body["pump"].(map[string]any), "name", "on", "since", "reason", "rest_until", "runtime_24h_seconds", "last_run_at", "error")
	wantKeys(t, "safety", body["safety"].(map[string]any), "tripped", "reason", "since")
	if body["pump"].(map[string]any)["reason"] != "no_target" {
		t.Errorf("pump = %v", body["pump"])
	}

	if code, body = h.do("POST", "/api/v1/control/override", `{"seconds":600}`); code != 422 {
		t.Errorf("long manual run: %d %v", code, body)
	}
	code, body = h.do("POST", "/api/v1/control/override", `{"seconds":5}`)
	if code != 200 || body["pump"].(map[string]any)["on"] != true || body["pump"].(map[string]any)["reason"] != "manual" {
		t.Fatalf("manual: %d %v", code, body)
	}
	code, body = h.do("DELETE", "/api/v1/control/override", "")
	if code != 200 || body["pump"].(map[string]any)["on"] != false {
		t.Fatalf("stop: %d %v", code, body)
	}
	if code, _ = h.do("POST", "/api/v1/control/reset", ""); code != 200 {
		t.Errorf("reset: %d", code)
	}
	_, body = h.do("GET", "/api/v1/climate-log?from=-1h", "")
	if items := body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["kind"] != "pump_manual" {
		t.Errorf("log = %v", items)
	}
}

func TestSettingsAndClimate(t *testing.T) {
	h := newHarness(t)
	// The studio schedule is gone: its fields are unknown now.
	for _, f := range []string{`"studio_days":["mon"]`, `"studio_hours":{"start":"09:00","end":"18:00"}`, `"studio_configured":true`} {
		code, body := h.do("PATCH", "/api/v1/settings", "{"+f+"}")
		if code != 422 || errCode(body) != "validation_failed" || fieldCode(body, strings.Split(strings.Trim(f, `"`), `"`)[0]) != "unknown_field" {
			t.Errorf("PATCH %s: %d %v", f, code, body)
		}
	}
	code, body := h.do("PATCH", "/api/v1/settings", `{"studio_tz":"Europe/Bucharest","sensors":[{"sensor":"main","offset_pct":1.5}]}`)
	if code != 200 || body["studio_tz"] != "Europe/Bucharest" {
		t.Fatalf("settings: %d %v", code, body)
	}
	wantKeys(t, "settings", body, "studio_tz", "sensors")
	code, body = h.do("GET", "/api/v1/climate-log", "")
	if code != 200 || len(body["items"].([]any)) != 1 {
		t.Fatalf("log: %d %v", code, body)
	}
	e := body["items"].([]any)[0].(map[string]any)
	wantKeys(t, "log entry", e, "id", "kind", "value", "at")
	if v := e["value"].(map[string]any); v["offset_pct"] != 1.5 || v["previous_offset_pct"] != 0.0 {
		t.Errorf("value = %v", v)
	}

	now := time.Now().UTC().Truncate(time.Minute)
	store.InsertMinutes(context.Background(), h.st.DB(), []store.Minute{{Sensor: "room", Minute: now.Add(-10 * time.Minute), HumAvg: 80, HumMin: 79, HumMax: 81, TempAvg: 18, VPDAvg: 0.41, N: 12}})
	code, body = h.do("GET", "/api/v1/readings?from=-1h&resolution=5m", "")
	if code != 200 {
		t.Fatalf("readings: %d %v", code, body)
	}
	wantKeys(t, "readings", body, "from", "to", "resolution_seconds", "t", "series")
	s := body["series"].([]any)[0].(map[string]any)
	wantKeys(t, "series", s, "sensor", "humidity_avg", "humidity_min", "humidity_max", "temperature_avg", "vpd_avg", "samples")
	if len(body["t"].([]any)) < 12 || body["resolution_seconds"] != 300.0 {
		t.Errorf("readings = %v", body)
	}
}

func jpegBytes(w, h int) []byte {
	var b bytes.Buffer
	jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, w, h)), nil)
	return b.Bytes()
}

func TestPhotos(t *testing.T) {
	h := newHarness(t)
	post := func(ct string, body []byte) (*http.Response, map[string]any) {
		resp, err := http.Post(h.srv.URL+"/api/v1/photos", ct, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return resp, m
	}
	if r, m := post("image/png", jpegBytes(10, 10)); r.StatusCode != 415 || errCode(m) != "unsupported_media_type" {
		t.Errorf("png: %d %v", r.StatusCode, m)
	}
	if r, m := post("image/jpeg", make([]byte, photos.MaxBytes+1)); r.StatusCode != 413 || errCode(m) != "too_large" {
		t.Errorf("too large: %d %v", r.StatusCode, m)
	}
	if r, m := post("image/jpeg", []byte("not a jpeg")); r.StatusCode != 422 {
		t.Errorf("garbage: %d %v", r.StatusCode, m)
	}
	r, m := post("image/jpeg", jpegBytes(640, 480))
	if r.StatusCode != 201 {
		t.Fatalf("upload: %d %v", r.StatusCode, m)
	}
	wantKeys(t, "photo", m, "id", "width", "height", "created_at")

	resp, err := http.Get(h.srv.URL + "/api/v1/photos/1?size=thumb")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := jpeg.DecodeConfig(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" || cfg.Width != 320 {
		t.Errorf("thumb: %d %q %d", resp.StatusCode, resp.Header.Get("Cache-Control"), cfg.Width)
	}
	if code, _ := h.do("GET", "/api/v1/photos/99", ""); code != 404 {
		t.Errorf("missing photo: %d", code)
	}
	code, body := h.do("POST", "/api/v1/pieces", `{"form":"mug","initial_dryness":1,"size":"small","thickness":"thin","photo_id":1}`)
	if code != 201 || body["items"].([]any)[0].(map[string]any)["photo_id"] != 1.0 {
		t.Errorf("piece with photo: %d %v", code, body)
	}
	code, body = h.do("POST", "/api/v1/pieces", `{"form":"mug","initial_dryness":1,"size":"small","thickness":"thin","photo_id":7}`)
	if code != 422 || fieldCode(body, "photo_id") != "photo_not_found" {
		t.Errorf("missing photo id: %d %v", code, body)
	}
}

func TestBackup(t *testing.T) {
	h := newHarness(t)
	h.do("POST", "/api/v1/pieces", `{"form":"mug","initial_dryness":1,"size":"small","thickness":"thin"}`)
	resp, err := http.Get(h.srv.URL + "/api/v1/backup")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "application/vnd.sqlite3" ||
		!strings.HasPrefix(resp.Header.Get("Content-Disposition"), `attachment; filename="humidity-`) {
		t.Errorf("headers = %v", resp.Header)
	}
	path := filepath.Join(t.TempDir(), "b.db")
	f, _ := os.Create(path)
	io.Copy(f, resp.Body)
	f.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var res string
	var n int
	db.QueryRow(`PRAGMA integrity_check`).Scan(&res)
	db.QueryRow(`SELECT count(*) FROM pieces`).Scan(&n)
	if res != "ok" || n != 1 {
		t.Errorf("backup: integrity %q, %d pieces", res, n)
	}
}

func TestStream(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.srv.URL + "/api/v1/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" || resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Errorf("headers = %v", resp.Header)
	}
	events := make(chan [2]string, 10)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var ev string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "retry: "):
				events <- [2]string{"retry", line}
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				events <- [2]string{ev, strings.TrimPrefix(line, "data: ")}
			}
		}
		close(events)
	}()
	next := func() [2]string {
		select {
		case e := <-events:
			return e
		case <-time.After(3 * time.Second):
			t.Fatal("no event")
		}
		return [2]string{}
	}
	if e := next(); e[1] != "retry: 5000" {
		t.Errorf("first = %v", e)
	}
	hello := next()
	var hm map[string]any
	json.Unmarshal([]byte(hello[1]), &hm)
	if hello[0] != "hello" || hm["version"] != "test" || hm["api_version"] != 1.0 {
		t.Errorf("hello = %v", hello)
	}
	wantKeys(t, "hello", hm, "server_time", "version", "api_version", "poll_interval_seconds", "stale_after_seconds", "clock_synced")
	if e := next(); e[0] != "readings" || !strings.Contains(e[1], `"room"`) {
		t.Errorf("readings = %v", e)
	}

	h.do("POST", "/api/v1/pieces", `{"form":"mug","initial_dryness":1,"size":"small","thickness":"thin"}`)
	h.do("POST", "/api/v1/pieces", `{"form":"mug","initial_dryness":1,"size":"small","thickness":"thin"}`)
	e := next()
	var cm struct {
		Scopes   []string `json:"scopes"`
		PieceIDs []int64  `json:"piece_ids"`
	}
	json.Unmarshal([]byte(e[1]), &cm)
	if e[0] != "changed" || !slices.Contains(cm.Scopes, "pieces") || !slices.Equal(cm.PieceIDs, []int64{1, 2}) {
		t.Errorf("changed = %v (two writes should merge into one event)", e)
	}

	h.hub.Publish("readings", map[string]any{"x": 1}, true)
	if e := next(); e[0] != "readings" {
		t.Errorf("published readings = %v", e)
	}
}

func TestHub_LimitsAndBackpressure(t *testing.T) {
	hub := NewHub()
	var clients []*client
	for i := 0; i < maxStreams; i++ {
		c, err := hub.register()
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
	}
	if _, err := hub.register(); err != ErrTooManyStreams {
		t.Errorf("17th stream: %v", err)
	}
	c := clients[0]
	for i := 0; i < clientBuffer; i++ {
		hub.Publish("readings", i, true)
	}
	hub.Publish("changed", "x", false) // displaces the oldest readings
	q := c.take()
	if len(q) != clientBuffer || q[len(q)-1].event != "changed" || string(q[0].data) != "1" {
		t.Errorf("queue after overflow: %d, last %s, first %s", len(q), q[len(q)-1].event, q[0].data)
	}
	for i := 0; i < clientBuffer; i++ {
		hub.Publish("changed", i, false)
	}
	hub.Publish("changed", "overflow", false) // no readings to drop: disconnect
	select {
	case <-c.done:
	default:
		t.Error("client not disconnected when a changed event couldn't be queued")
	}
	hub.Close()
	select {
	case <-clients[1].done:
	default:
		t.Error("Close didn't disconnect clients")
	}
}

func TestStatic(t *testing.T) {
	h := newHarness(t)
	for path, want := range map[string]string{"/": "no-cache", "/shelf/12": "no-cache", "/assets/app-1234.js": "public, max-age=31536000, immutable"} {
		resp, err := http.Get(h.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != want {
			t.Errorf("%s: %d %q", path, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
		if path != "/assets/app-1234.js" && string(b) != "<html>app</html>" {
			t.Errorf("%s: body %q, want index.html", path, b)
		}
	}

	// A stale chunk from an older build must not get index.html.
	resp, err := http.Get(h.srv.URL + "/assets/app-old.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("/assets/app-old.js: %d, want 404", resp.StatusCode)
	}
}
