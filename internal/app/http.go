package app

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disintegration/imaging"
)

// maxPhoto is the largest upload: a phone camera's original photo.
const maxPhoto = 25 << 20

// photoEdge is the long edge, in pixels, photos are stored at.
const photoEdge = 1600

// App holds what the HTTP handlers and the sensor loop need.
type App struct {
	DB      *sql.DB
	DataDir string
	MaxRH   int
	Live    *Live
	Pump    *Pump // nil without a pump (PUMP=none)
	Web     fs.FS // the built web app
}

// Handler serves the API (docs/plan.md §6) and the web app.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()

	// api registers a handler whose result is sent as JSON; a nil result
	// is a 204.
	api := func(pattern string, h func(*http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			v, err := h(r)
			switch {
			case err != nil:
				writeError(w, err)
			case v == nil:
				w.WriteHeader(http.StatusNoContent)
			default:
				writeJSON(w, http.StatusOK, v)
			}
		})
	}
	api("GET /api/now", a.getNow)
	api("GET /api/history", a.getHistory)
	api("PUT /api/target", a.putTarget)
	api("POST /api/pump/run", a.pumpRun)
	api("POST /api/pump/stop", a.pumpCommand("stop"))
	api("POST /api/pump/resume", a.pumpCommand("resume"))
	api("PUT /api/sensors/{addr}/offset", a.putOffset)
	api("GET /api/pieces", a.listPieces)
	api("POST /api/pieces", a.postPieces)
	api("GET /api/pieces/{id}", a.getPiece)
	api("PATCH /api/pieces/{id}", a.patchPieceHandler)
	api("DELETE /api/pieces/{id}", a.deletePieceHandler)
	api("PUT /api/pieces/{id}/photo", a.putPhoto)
	mux.HandleFunc("GET /api/pieces/{id}/photo", a.getPhoto)
	api("POST /api/pieces/{id}/checks", a.postCheck)
	api("DELETE /api/checks/{id}", a.deleteCheckHandler)
	mux.HandleFunc("GET /api/backup", a.getBackup)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, errorf(404, "no such API route: %s %s", r.Method, r.URL.Path))
	})
	mux.Handle("/", static(a.Web))
	return mux
}

// --- Climate and pump

type Now struct {
	Room     *roomJSON    `json:"room"`
	Sensors  []sensorJSON `json:"sensors"`
	Target   *int         `json:"target"`
	MaxRH    int          `json:"max_rh"`
	Pump     PumpStatus   `json:"pump"`
	Learning bool         `json:"learning"`
}

func (a *App) now(ctx context.Context) (*Now, error) {
	_, n, err := a.loadPieces(ctx)
	if err != nil {
		return nil, err
	}
	now := &Now{Sensors: a.Live.sensorsJSON(), MaxRH: a.MaxRH, Learning: n < 5}
	if room, ok := a.Live.Room(time.Now()); ok {
		now.Room = &roomJSON{RH: round2(room.RH), Temp: round2(room.Temp), At: room.At}
	}
	if t := a.Live.Target(); t != 0 {
		now.Target = &t
	}
	if a.Pump != nil {
		now.Pump = a.Pump.Status()
	} else {
		now.Pump = PumpStatus{Status: "idle"}
	}
	return now, nil
}

func (a *App) getNow(r *http.Request) (any, error) { return a.now(r.Context()) }

func (a *App) getHistory(r *http.Request) (any, error) {
	switch r.URL.Query().Get("hours") {
	case "1":
		return history(r.Context(), a.DB, time.Now(), 60, 60)
	case "", "24":
		return history(r.Context(), a.DB, time.Now(), 300, 288)
	case "168":
		return history(r.Context(), a.DB, time.Now(), 1800, 336)
	}
	return nil, errorf(400, "hours must be 1, 24 or 168")
}

func (a *App) putTarget(r *http.Request) (any, error) {
	var in struct {
		Pct *int `json:"pct"`
	}
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	if in.Pct == nil {
		if err := deleteSetting(r.Context(), a.DB, "target"); err != nil {
			return nil, err
		}
		a.Live.setTarget(0)
		return a.targetChanged(r)
	}
	if *in.Pct < 50 || *in.Pct >= a.MaxRH {
		return nil, errorf(400, "pct must be from 50 to %d", a.MaxRH-1)
	}
	if err := setSetting(r.Context(), a.DB, "target", strconv.Itoa(*in.Pct)); err != nil {
		return nil, err
	}
	a.Live.setTarget(*in.Pct)
	return a.targetChanged(r)
}

// targetChanged has the pump decide now, so the reply shows the outcome.
func (a *App) targetChanged(r *http.Request) (any, error) {
	if a.Pump != nil {
		if err := a.Pump.Do(r.Context(), "decide", 0); err != nil {
			return nil, err
		}
	}
	return a.now(r.Context())
}

func (a *App) pumpRun(r *http.Request) (any, error) {
	if a.Pump == nil {
		return nil, errorf(409, "no pump is configured")
	}
	var in struct {
		Seconds *int `json:"seconds"`
	}
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	if in.Seconds == nil || *in.Seconds < 1 || *in.Seconds > 60 {
		return nil, errorf(400, "seconds must be from 1 to 60")
	}
	err := a.Pump.Do(r.Context(), "run", time.Duration(*in.Seconds)*time.Second)
	if errors.Is(err, errAboveMax) {
		return nil, errorf(409, "humidity is at or above %d%%", a.MaxRH)
	}
	if err != nil {
		return nil, err
	}
	return a.now(r.Context())
}

func (a *App) pumpCommand(kind string) func(*http.Request) (any, error) {
	return func(r *http.Request) (any, error) {
		if a.Pump == nil {
			return nil, errorf(409, "no pump is configured")
		}
		if err := a.Pump.Do(r.Context(), kind, 0); err != nil {
			return nil, err
		}
		return a.now(r.Context())
	}
}

func (a *App) putOffset(r *http.Request) (any, error) {
	addr := r.PathValue("addr")
	if !a.Live.hasSensor(addr) {
		return nil, errorf(404, "no sensor %s", addr)
	}
	var in struct {
		Offset *float64 `json:"offset"`
	}
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	if in.Offset == nil || *in.Offset < -10 || *in.Offset > 10 {
		return nil, errorf(400, "offset must be from -10 to 10")
	}
	value := strconv.FormatFloat(*in.Offset, 'f', -1, 64)
	if err := setSetting(r.Context(), a.DB, "offset:"+addr, value); err != nil {
		return nil, err
	}
	a.Live.setOffset(addr, *in.Offset)
	return a.now(r.Context())
}

// --- Pieces

func (a *App) listPieces(r *http.Request) (any, error) {
	all, _, err := a.loadPieces(r.Context())
	if err != nil {
		return nil, err
	}
	ended := r.URL.Query().Get("ended") == "1"
	list := []*Piece{}
	for _, p := range all {
		if (p.EndedAt != nil) == ended {
			p.Checks = nil
			list = append(list, p)
		}
	}
	if ended {
		slices.SortFunc(list, func(x, y *Piece) int {
			return cmp.Or(y.EndedAt.Compare(*x.EndedAt), cmp.Compare(y.ID, x.ID))
		})
		return list, nil
	}
	// past first, then ready by ready_until, then drying by ready_at.
	rank := map[string]int{"past": 0, "ready": 1, "drying": 2}
	slices.SortFunc(list, func(x, y *Piece) int {
		c := cmp.Compare(rank[x.Estimate.Status], rank[y.Estimate.Status])
		if c == 0 && x.Estimate.Status == "ready" {
			c = x.Estimate.ReadyUntil.Compare(*y.Estimate.ReadyUntil)
		}
		if c == 0 && x.Estimate.Status == "drying" {
			c = x.Estimate.ReadyAt.Compare(*y.Estimate.ReadyAt)
		}
		return cmp.Or(c, cmp.Compare(x.Number, y.Number))
	})
	return list, nil
}

func (a *App) postPieces(r *http.Request) (any, error) {
	var in newPieces
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	err := cmp.Or(oneOf("form", in.Form, forms), oneOf("size", in.Size, sizes),
		oneOf("thickness", in.Thickness, thicknesses),
		checkText("label", in.Label), checkText("notes", in.Notes))
	if err != nil {
		return nil, err
	}
	if in.Dryness == nil {
		return nil, errorf(400, "dryness is required")
	}
	d, err := checkDryness(*in.Dryness)
	if err != nil {
		return nil, err
	}
	in.Dryness = &d
	if in.Quantity == nil {
		one := 1
		in.Quantity = &one
	}
	if *in.Quantity < 1 || *in.Quantity > 20 {
		return nil, errorf(400, "quantity must be from 1 to 20")
	}

	ids, err := a.addPieces(r.Context(), in)
	if err != nil {
		return nil, err
	}
	all, _, err := a.loadPieces(r.Context())
	if err != nil {
		return nil, err
	}
	created := []*Piece{}
	for _, p := range all {
		if slices.Contains(ids, p.ID) {
			p.Checks = nil
			created = append(created, p)
		}
	}
	slices.SortFunc(created, func(x, y *Piece) int { return cmp.Compare(x.Number, y.Number) })
	return created, nil
}

func (a *App) getPiece(r *http.Request) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return a.piece(r.Context(), id)
}

func (a *App) patchPieceHandler(r *http.Request) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := readJSON(r, &raw); err != nil {
		return nil, err
	}
	p := piecePatch{edits: map[string]string{}}
	for key, v := range raw {
		if key == "dryness" {
			var d float64
			if err := json.Unmarshal(v, &d); err != nil || string(v) == "null" {
				return nil, errorf(400, "dryness must be a number")
			}
			if d, err = checkDryness(d); err != nil {
				return nil, err
			}
			p.dryness = &d
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil || string(v) == "null" {
			return nil, errorf(400, "%s must be a string", key)
		}
		switch key {
		case "label", "notes":
			err = checkText(key, s)
		case "form":
			err = oneOf(key, s, forms)
		case "size":
			err = oneOf(key, s, sizes)
		case "thickness":
			err = oneOf(key, s, thicknesses)
		case "state":
			err = oneOf(key, s, states)
			p.state = s
		default:
			err = errorf(400, "unknown field %q", key)
		}
		if err != nil {
			return nil, err
		}
		if key != "state" {
			p.edits[key] = s
		}
	}
	if p.dryness != nil && p.state != "trimmed" && p.state != "discarded" {
		return nil, errorf(400, "dryness goes only with state trimmed or discarded")
	}

	if err := a.patchPiece(r.Context(), id, p); err != nil {
		return nil, err
	}
	return a.piece(r.Context(), id)
}

func (a *App) deletePieceHandler(r *http.Request) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return nil, a.deletePiece(r.Context(), id)
}

func (a *App) postCheck(r *http.Request) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		Dryness *float64 `json:"dryness"`
	}
	if err := readJSON(r, &in); err != nil {
		return nil, err
	}
	if in.Dryness == nil {
		return nil, errorf(400, "dryness is required")
	}
	d, err := checkDryness(*in.Dryness)
	if err != nil {
		return nil, err
	}
	if err := a.addCheck(r.Context(), id, d); err != nil {
		return nil, err
	}
	return a.piece(r.Context(), id)
}

func (a *App) deleteCheckHandler(r *http.Request) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	pieceID, err := a.deleteCheck(r.Context(), id)
	if err != nil {
		return nil, err
	}
	return a.piece(r.Context(), pieceID)
}

// putPhoto stores a photo straight from the phone's camera (JPEG or PNG),
// turned upright from its EXIF orientation and shrunk to photoEdge. It's
// done here rather than on the phone because browsers with fingerprinting
// protection scramble what a canvas produces.
func (a *App) putPhoto(r *http.Request) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	if _, err := a.piece(r.Context(), id); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxPhoto+1))
	if err != nil {
		return nil, errorf(400, "reading the photo: %v", err)
	}
	if len(data) > maxPhoto {
		return nil, errorf(413, "the photo is over 25 MB")
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, errorf(400, "the photo must be a JPEG or PNG")
	}
	img = imaging.Fit(img, photoEdge, photoEdge, imaging.Lanczos)
	// Write then rename, so a reader never sees half a photo.
	var out bytes.Buffer
	if err := imaging.Encode(&out, img, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		return nil, err
	}
	tmp := a.photoPath(id) + ".tmp"
	if err := os.WriteFile(tmp, out.Bytes(), 0o644); err != nil {
		return nil, err
	}
	return nil, os.Rename(tmp, a.photoPath(id))
}

func (a *App) getPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := os.Stat(a.photoPath(id)); err != nil {
		writeError(w, errorf(404, "no photo"))
		return
	}
	http.ServeFile(w, r, a.photoPath(id))
}

// getBackup sends a fresh snapshot of the database.
func (a *App) getBackup(w http.ResponseWriter, r *http.Request) {
	f, err := os.CreateTemp("", "humidity-*.db")
	if err != nil {
		writeError(w, err)
		return
	}
	f.Close()
	defer os.Remove(f.Name())
	// VACUUM INTO accepts an existing file only if it is empty, as it is.
	if _, err := a.DB.ExecContext(r.Context(), `VACUUM INTO ?`, f.Name()); err != nil {
		writeError(w, err)
		return
	}
	name := "humidity-" + time.Now().Format("2006-01-02") + ".db"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, f.Name())
}

// --- Helpers

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errorf(400, "invalid JSON body: %v", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	var e *apiError
	if !errors.As(err, &e) {
		log.Printf("error: %v", err)
		e = &apiError{500, "internal error: " + err.Error()}
	}
	writeJSON(w, e.code, map[string]string{"error": e.msg})
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errorf(404, "not found: %s", r.URL.Path)
	}
	return id, nil
}

// checkDryness rounds d to 0.1 after checking it is on the 0–5 scale.
func checkDryness(d float64) (float64, error) {
	if d < 0 || d > 5 {
		return 0, errorf(400, "dryness must be from 0 to 5")
	}
	return math.Round(d*10) / 10, nil
}

func oneOf(field, value string, values []string) error {
	if !slices.Contains(values, value) {
		return errorf(400, "%s must be one of %s", field, strings.Join(values, ", "))
	}
	return nil
}

// checkText checks the length of a label or notes.
func checkText(field, s string) error {
	limit := map[string]int{"label": 100, "notes": 2000}[field]
	if utf8.RuneCountInString(s) > limit {
		return errorf(400, "%s is over %d characters", field, limit)
	}
	return nil
}

// static serves the web app's files, and index.html for any other path so
// the app's own routes work on reload.
func static(dist fs.FS) http.Handler {
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if st, err := fs.Stat(dist, name); err != nil || st.IsDir() {
			http.ServeFileFS(w, r, dist, "index.html")
			return
		}
		if strings.HasPrefix(name, "assets/") { // file names carry a content hash
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
