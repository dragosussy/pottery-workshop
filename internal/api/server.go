// Package api serves the HTTP API (api-contract.md) under /api/v1, the
// live event stream, and the embedded frontend.
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"humidity/internal/monitor"
	"humidity/internal/photos"
	"humidity/internal/shelf"
	"humidity/internal/store"
)

// Deps are the server's collaborators, wired in run().
type Deps struct {
	Store       *store.Store
	Monitor     *monitor.Monitor
	Shelf       *shelf.Service
	Photos      *photos.Service
	Hub         *Hub
	ClockSynced func() bool
	Web         fs.FS
	TempDir     string // where /backup snapshots are written
	Version     string
	Commit      string
	BuiltAt     string // RFC 3339, or ""
	StartedAt   time.Time
	Log         *slog.Logger
	Now         func() time.Time
}

// Server holds the handlers.
type Server struct {
	Deps
	log *slog.Logger
	now func() time.Time
}

// New returns the server and its handler.
func New(d Deps) (*Server, http.Handler) {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &Server{Deps: d, log: d.Log, now: d.Now}

	mux := http.NewServeMux()
	route := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, h) }

	route("GET /api/v1/health", s.health)
	route("GET /api/v1/reference", s.reference)
	route("GET /api/v1/status", s.status)
	route("GET /api/v1/stream", s.stream)
	route("GET /api/v1/readings", s.readings)
	route("GET /api/v1/climate-log", s.climateLog)
	route("GET /api/v1/shelf", s.shelf)
	route("POST /api/v1/pieces", s.addPieces)
	route("GET /api/v1/pieces", s.listPieces)
	route("PATCH /api/v1/pieces", s.patchPieces)
	route("GET /api/v1/pieces/{id}", s.getPiece)
	route("PATCH /api/v1/pieces/{id}", s.patchPiece)
	route("GET /api/v1/pieces/{id}/observations", s.pieceObservations)
	route("POST /api/v1/photos", s.uploadPhoto)
	route("GET /api/v1/photos/{id}", s.getPhoto)
	route("POST /api/v1/observations", s.observe)
	route("GET /api/v1/observations", s.listObservations)
	route("PATCH /api/v1/observations/{id}", s.patchObservation)
	route("DELETE /api/v1/observations/{id}", s.deleteObservation)
	route("POST /api/v1/observations/{id}/undelete", s.undeleteObservation)
	route("POST /api/v1/observations/{id}/follow-up", s.followUp)
	route("GET /api/v1/settings", s.getSettings)
	route("PATCH /api/v1/settings", s.patchSettings)
	route("GET /api/v1/backup", s.backup)
	notImplemented := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotImplemented, "not_implemented", "humidity control is not part of v1")
	}
	route("/api/v1/control", notImplemented)
	route("/api/v1/control/", notImplemented)
	route("/api/", func(w http.ResponseWriter, r *http.Request) { notFound(w, "no such endpoint") })
	route("/", s.static)

	return s, s.recoverer(s.logRequests(mux))
}

// pathID parses the {id} path value; it writes a 404 and reports false if
// it isn't a positive integer.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		notFound(w, "no such id")
		return 0, false
	}
	return id, true
}

// RunStream forwards monitor cycles to stream clients as readings events
// until ctx is cancelled.
func (s *Server) RunStream(ctx context.Context, cycles <-chan monitor.Cycle) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case c := <-cycles:
			s.Hub.Publish("readings", s.readingsEvent(c), true)
		}
	}
}

// staleAfter is how old a reading may be before a sensor counts as stale.
func (s *Server) staleAfter() time.Duration {
	return max(30*time.Second, 3*s.Monitor.Interval())
}
