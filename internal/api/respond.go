package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"humidity/internal/shelf"
)

// maxJSONBody limits request bodies (api-contract §1).
const maxJSONBody = 64 << 10

// errorBody is the error envelope (api-contract §1.1).
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string             `json:"code"`
	Message string             `json:"message"`
	Fields  []shelf.FieldError `json:"fields"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string, fields ...shelf.FieldError) {
	if fields == nil {
		fields = []shelf.FieldError{}
	}
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: msg, Fields: fields}})
}

func writeValidation(w http.ResponseWriter, fields ...shelf.FieldError) {
	v := &shelf.ValidationError{Fields: fields}
	writeError(w, http.StatusUnprocessableEntity, "validation_failed", v.Error(), fields...)
}

func notFound(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusNotFound, "not_found", msg)
}

// fail maps a domain error to a response. Unknown errors are logged and
// become a bare 500.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ve *shelf.ValidationError
	switch {
	case errors.As(err, &ve):
		writeValidation(w, ve.Fields...)
	case errors.Is(err, shelf.ErrNotFound):
		notFound(w, err.Error())
	case errors.Is(err, shelf.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid_transition", err.Error())
	case errors.Is(err, shelf.ErrPieceEnded):
		writeError(w, http.StatusConflict, "piece_ended", err.Error())
	case errors.Is(err, shelf.ErrLastObservation):
		writeError(w, http.StatusConflict, "last_observation", err.Error())
	case errors.Is(err, shelf.ErrBadCursor):
		writeError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
	default:
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "something went wrong; details are in the server log")
	}
}

// decodeJSON reads a JSON body into dst, rejecting unknown fields. It
// writes the error response itself and reports false on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		if _, extra := dec.Token(); extra != io.EOF {
			writeError(w, http.StatusBadRequest, "bad_request", "unexpected data after the JSON body")
			return false
		}
		return true
	}

	var maxErr *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &maxErr):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "request body over 64 KB")
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			field = "body"
		}
		writeValidation(w, shelf.FieldError{Field: field, Code: "invalid_value", Message: fmt.Sprintf("must be %s", jsonKind(typeErr.Type.Kind().String()))})
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		name := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		writeValidation(w, shelf.FieldError{Field: name, Code: "unknown_field", Message: "unknown field"})
	case errors.Is(err, io.EOF):
		writeError(w, http.StatusBadRequest, "bad_request", "empty body; send a JSON object")
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "malformed JSON: "+err.Error())
	}
	return false
}

func jsonKind(goKind string) string {
	switch {
	case strings.HasPrefix(goKind, "int"), strings.HasPrefix(goKind, "uint"), strings.HasPrefix(goKind, "float"):
		return "a number"
	case goKind == "string":
		return "a string"
	case goKind == "bool":
		return "true or false"
	case goKind == "slice":
		return "an array"
	case goKind == "struct", goKind == "map":
		return "an object"
	}
	return "a different type"
}

// recoverer turns a panic into a 500 and logs it.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic", "method", r.Method, "path", r.URL.Path, "panic", v)
				writeError(w, http.StatusInternalServerError, "internal", "something went wrong; details are in the server log")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// logRequests logs every request at Debug, and 5xx responses at Warn.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := s.now()
		next.ServeHTTP(rec, r)
		level := slog.LevelDebug
		if rec.status >= 500 {
			level = slog.LevelWarn
		}
		s.log.Log(r.Context(), level, "http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "took", s.now().Sub(start))
	})
}
