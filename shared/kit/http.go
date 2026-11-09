// Package kit is the Go code every app shares: JSON over HTTP, the web app's
// static files, opening SQLite, daily snapshots and photo processing.
package kit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// Error is an error with the HTTP status to answer it with.
type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// Errorf makes an Error.
func Errorf(code int, format string, args ...any) error {
	return &Error{code, fmt.Sprintf(format, args...)}
}

// ReadJSON decodes a request body. Unknown fields are an error, so a
// misspelled field fails loudly.
func ReadJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return Errorf(400, "invalid JSON body: %v", err)
	}
	return nil
}

// WriteJSON answers with v as JSON.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// WriteError answers with {"error": message}. Anything that isn't an Error
// is logged and answered as a 500.
func WriteError(w http.ResponseWriter, err error) {
	var e *Error
	if !errors.As(err, &e) {
		log.Printf("error: %v", err)
		e = &Error{500, "internal error: " + err.Error()}
	}
	WriteJSON(w, e.Code, map[string]string{"error": e.Msg})
}

// Static serves the web app's files, and index.html for any other path so
// the app's own routes work on reload.
func Static(dist fs.FS) http.Handler {
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

// JSONHandler adapts a handler whose result is sent as JSON. A nil result is
// a 204.
func JSONHandler(h func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := h(r)
		switch {
		case err != nil:
			WriteError(w, err)
		case v == nil:
			w.WriteHeader(http.StatusNoContent)
		default:
			WriteJSON(w, http.StatusOK, v)
		}
	}
}

// PathID reads the {id} in a route. Anything but a number is a 404.
func PathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, Errorf(404, "not found: %s", r.URL.Path)
	}
	return id, nil
}
