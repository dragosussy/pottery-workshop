package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"humidity/internal/photos"
	"humidity/internal/recorder"
	"humidity/internal/shelf"
)

func jsonBytes(v any) ([]byte, error) { return json.Marshal(v) }

func (s *Server) reference(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, shelf.ReferenceData())
}

func (s *Server) shelf(w http.ResponseWriter, r *http.Request) {
	v, err := s.Shelf.Shelf(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// --- pieces ---

type itemsBody[T any] struct {
	Items []T `json:"items"`
}

func (s *Server) addPieces(w http.ResponseWriter, r *http.Request) {
	var in shelf.AddPieces
	if !decodeJSON(w, r, &in) {
		return
	}
	items, err := s.Shelf.Add(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, itemsBody[shelf.PieceView]{Items: items})
}

// pageLimit parses ?limit (default 20, 1–100).
func pageLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 20, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		writeError(w, http.StatusBadRequest, "bad_request", "limit must be an integer from 1 to 100")
		return 0, false
	}
	return n, true
}

func (s *Server) listPieces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch state := q.Get("state"); state {
	case "", "on_shelf":
		items, err := s.Shelf.OnShelf(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if items == nil {
			items = []shelf.PieceView{}
		}
		writeJSON(w, http.StatusOK, itemsBody[shelf.PieceView]{Items: items})
	case "ended", "all":
		limit, ok := pageLimit(w, r)
		if !ok {
			return
		}
		page, err := s.Shelf.ListPieces(r.Context(), state == "all", q.Get("cursor"), limit)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if page.Items == nil {
			page.Items = []shelf.PieceView{}
		}
		writeJSON(w, http.StatusOK, page)
	default:
		writeValidation(w, shelf.FieldError{Field: "state", Code: "invalid_enum", Message: "must be on_shelf, ended or all"})
	}
}

func (s *Server) getPiece(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := s.Shelf.Get(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) patchPiece(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var p shelf.PiecePatch
	if !decodeJSON(w, r, &p) {
		return
	}
	items, err := s.Shelf.Update(r.Context(), []int64{id}, p, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items[0])
}

func (s *Server) patchPieces(w http.ResponseWriter, r *http.Request) {
	var p shelf.BulkPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	if p.IDs == nil {
		writeValidation(w, shelf.FieldError{Field: "ids", Code: "required", Message: "list the pieces to change"})
		return
	}
	items, err := s.Shelf.Update(r.Context(), *p.IDs, p.PiecePatch, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, itemsBody[shelf.PieceView]{Items: items})
}

// --- photos ---

func (s *Server) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "image/jpeg" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "send the photo as image/jpeg")
		return
	}
	if r.ContentLength > photos.MaxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "photo over 3 MB")
		return
	}
	body, over, err := photos.ReadLimited(r.Body, photos.MaxBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "couldn't read the upload")
		return
	}
	if over {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "photo over 3 MB")
		return
	}
	c, err := s.Photos.Upload(r.Context(), body)
	switch {
	case errors.Is(err, photos.ErrNotJPEG):
		writeValidation(w, shelf.FieldError{Field: "body", Code: "invalid_value", Message: "not a readable JPEG image"})
	case errors.Is(err, photos.ErrTooManyPix):
		writeValidation(w, shelf.FieldError{Field: "body", Code: "out_of_range", Message: "image larger than 20 megapixels"})
	case err != nil:
		s.fail(w, r, err)
	default:
		writeJSON(w, http.StatusCreated, c)
	}
}

func (s *Server) getPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var thumb bool
	switch r.URL.Query().Get("size") {
	case "", "full":
	case "thumb":
		thumb = true
	default:
		writeValidation(w, shelf.FieldError{Field: "size", Code: "invalid_enum", Message: "must be thumb or full"})
		return
	}
	f, p, err := s.Photos.Open(r.Context(), id, thumb)
	if errors.Is(err, photos.ErrNotFound) {
		notFound(w, fmt.Sprintf("no photo %d", id))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, "", p.CreatedAt, f)
}

// --- observations ---

func (s *Server) observe(w http.ResponseWriter, r *http.Request) {
	var in shelf.NewObservations
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := s.Shelf.Observe(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) pieceObservations(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	items, err := s.Shelf.PieceObservations(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []shelf.ObservationView{}
	}
	writeJSON(w, http.StatusOK, itemsBody[shelf.ObservationView]{Items: items})
}

func (s *Server) listObservations(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	page, err := s.Shelf.RecentObservations(r.Context(), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if page.Items == nil {
		page.Items = []shelf.ObservationView{}
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) patchObservation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var p shelf.ObservationPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	v, err := s.Shelf.UpdateObservation(r.Context(), id, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteObservation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Shelf.DeleteObservation(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// emptyOrObject accepts an empty body or "{}".
func emptyOrObject(w http.ResponseWriter, r *http.Request) bool {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "couldn't read the body")
		return false
	}
	if t := strings.TrimSpace(string(b)); t != "" && t != "{}" {
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "malformed JSON")
			return false
		}
		for k := range m {
			writeValidation(w, shelf.FieldError{Field: k, Code: "unknown_field", Message: "unknown field"})
			return false
		}
	}
	return true
}

func (s *Server) undeleteObservation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok || !emptyOrObject(w, r) {
		return
	}
	v, err := s.Shelf.UndeleteObservation(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) followUp(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in shelf.FollowUpAnswer
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.Shelf.AnswerFollowUp(r.Context(), id, in); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

// --- settings ---

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	v, err := s.Shelf.Settings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var p shelf.SettingsPatch
	if !decodeJSON(w, r, &p) {
		return
	}
	v, err := s.Shelf.UpdateSettings(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// --- climate ---

var relTime = regexp.MustCompile(`^-(\d+)([mhd])$`)

// parseWhen parses an RFC 3339 time or a relative one (-6h, -24h, -7d).
func parseWhen(s string, now time.Time) (time.Time, bool) {
	if m := relTime.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{"m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		return now.Add(-time.Duration(n) * unit), true
	}
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

func (s *Server) readings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := s.now()
	var fields []shelf.FieldError
	from, to := time.Time{}, now
	if raw := q.Get("from"); raw == "" {
		fields = append(fields, shelf.FieldError{Field: "from", Code: "required", Message: "required"})
	} else if t, ok := parseWhen(raw, now); !ok {
		fields = append(fields, shelf.FieldError{Field: "from", Code: "invalid_time", Message: "RFC 3339, or -6h, -24h, -7d"})
	} else {
		from = t
	}
	if raw := q.Get("to"); raw != "" {
		if t, ok := parseWhen(raw, now); ok {
			to = t
		} else {
			fields = append(fields, shelf.FieldError{Field: "to", Code: "invalid_time", Message: "must be an RFC 3339 time"})
		}
	}
	if len(fields) == 0 && !to.After(from) {
		fields = append(fields, shelf.FieldError{Field: "to", Code: "invalid_value", Message: "must be after from"})
	}
	sensors := []string{recorder.RoomSeries}
	if raw := q.Get("sensors"); raw != "" {
		sensors = nil
		for _, name := range strings.Split(raw, ",") {
			if name = strings.TrimSpace(name); name != "" {
				sensors = append(sensors, name)
			}
		}
		if len(sensors) == 0 {
			fields = append(fields, shelf.FieldError{Field: "sensors", Code: "invalid_value", Message: "list room or sensor names"})
		}
	}
	settings, err := s.Shelf.Settings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	loc, err := time.LoadLocation(settings.StudioTZ)
	if err != nil {
		loc = time.UTC
	}
	var step time.Duration
	switch res := q.Get("resolution"); res {
	case "", "auto":
		step = recorder.PickResolution(from, to, loc)
	default:
		for _, rr := range recorder.Resolutions {
			if rr.Name == res {
				step = rr.Step
			}
		}
		if step == 0 {
			fields = append(fields, shelf.FieldError{Field: "resolution", Code: "invalid_enum", Message: "must be auto, 1m, 5m, 15m, 1h, 6h or 1d"})
		}
	}
	if len(fields) > 0 {
		writeValidation(w, fields...)
		return
	}
	out, err := recorder.Series(r.Context(), s.Store.DB(), sensors, from, to, step, loc)
	if errors.Is(err, recorder.ErrTooManyPoints) {
		writeValidation(w, shelf.FieldError{Field: "resolution", Code: "too_many_points", Message: "more than 2000 points; pick a coarser resolution"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) climateLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := s.now()
	from, to := time.Unix(0, 0), now.Add(24*time.Hour)
	var fields []shelf.FieldError
	for _, p := range []struct {
		name string
		dst  *time.Time
	}{{"from", &from}, {"to", &to}} {
		if raw := q.Get(p.name); raw != "" {
			if t, ok := parseWhen(raw, now); ok {
				*p.dst = t
			} else {
				fields = append(fields, shelf.FieldError{Field: p.name, Code: "invalid_time", Message: "RFC 3339, or -6h, -24h, -7d"})
			}
		}
	}
	if len(fields) > 0 {
		writeValidation(w, fields...)
		return
	}
	items, err := s.Shelf.ClimateLog(r.Context(), from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, itemsBody[shelf.LogEntryView]{Items: items})
}

// --- backup ---

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	now := s.now().UTC()
	if err := os.MkdirAll(s.TempDir, 0o755); err != nil {
		s.fail(w, r, err)
		return
	}
	tmp := filepath.Join(s.TempDir, fmt.Sprintf(".download-%d.db", now.UnixNano()))
	defer os.Remove(tmp)
	if err := s.Store.Snapshot(r.Context(), tmp); err != nil {
		s.fail(w, r, err)
		return
	}
	f, err := os.Open(tmp)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	name := "humidity-" + now.Format("20060102T150405Z") + ".db"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	if st, err := f.Stat(); err == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	io.Copy(w, f)
}
