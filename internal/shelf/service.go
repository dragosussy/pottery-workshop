// Package shelf is the domain: pieces and their lifecycle, shelf numbers,
// observations and follow-ups, settings, and the glue that feeds
// stored history to the predict package. HTTP handlers call Service.
package shelf

import (
	"context"
	"sync"
	"time"

	"humidity/internal/predict"
	"humidity/internal/store"
)

// Notifier is told about every committed write (the SSE hub).
type Notifier interface {
	Changed(scopes []string, pieceIDs []int64)
}

// OffsetSetter applies sensor calibration offsets (the monitor).
type OffsetSetter interface {
	SetOffsets(map[string]float64)
}

// Change scopes for Notifier (api-contract §4.2).
const (
	ScopePieces       = "pieces"
	ScopeObservations = "observations"
	ScopePredictions  = "predictions"
	ScopeSettings     = "settings"
	ScopeClimateLog   = "climate_log"
)

// cacheTTL bounds how stale predictions may get: they move with time.
const cacheTTL = 60 * time.Second

// Service implements the domain on top of the store.
type Service struct {
	st        *store.Store
	dose      predict.Doser
	notify    Notifier
	offsets   OffsetSetter
	sensors   []string // configured sensor names, in order
	defaultTZ string

	// Now is the clock; tests replace it.
	Now func() time.Time

	mu    sync.Mutex
	cache *plan
}

// New returns a Service. sensors are the configured sensor names and
// defaultTZ the initial studio time zone (STUDIO_TZ).
func New(st *store.Store, dose predict.Doser, notify Notifier, offsets OffsetSetter, sensors []string, defaultTZ string) *Service {
	return &Service{
		st: st, dose: dose, notify: notify, offsets: offsets,
		sensors: sensors, defaultTZ: defaultTZ, Now: time.Now,
	}
}

// changed invalidates the prediction cache and notifies listeners. Call it
// after a commit.
func (s *Service) changed(scopes []string, pieceIDs []int64) {
	s.Invalidate()
	if pieceIDs == nil {
		pieceIDs = []int64{}
	}
	s.notify.Changed(scopes, pieceIDs)
}

// Invalidate drops cached predictions.
func (s *Service) Invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// plan is a cached prediction run and the inputs it was built from.
type plan struct {
	out      predict.Output
	model    *predict.Model
	lc       predict.LiveContext
	history  map[int64]predict.Piece
	settings Settings
	offline  []predict.Interval
}

// plan returns the current predictions, recomputing them when a write
// invalidated the cache or it is older than cacheTTL.
func (s *Service) plan(ctx context.Context) (*plan, error) {
	now := s.Now()
	s.mu.Lock()
	if c := s.cache; c != nil && now.Sub(c.out.ComputedAt) < cacheTTL && !now.Before(c.out.ComputedAt) {
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	p, err := s.computePlan(ctx, now)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache = p
	s.mu.Unlock()
	return p, nil
}

func (s *Service) computePlan(ctx context.Context, now time.Time) (*plan, error) {
	q := s.st.DB()
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	pieces, err := store.HistoryPieces(ctx, q)
	if err != nil {
		return nil, err
	}
	obs, err := store.HistoryObservations(ctx, q)
	if err != nil {
		return nil, err
	}
	unknown, err := store.UnansweredFollowUps(ctx, q)
	if err != nil {
		return nil, err
	}
	logEntries, err := store.LogEntries(ctx, q)
	if err != nil {
		return nil, err
	}
	offline, gaps := intervals(logEntries)

	history := make(map[int64]predict.Piece, len(pieces))
	in := predict.Input{
		Now:        now,
		Dose:       s.dose,
		Offline:    offline,
		SensorGaps: gaps,
		Loc:        settings.loc(),
	}
	for _, p := range pieces {
		mp := modelPiece(p, obs[p.ID], unknown[p.ID])
		history[p.ID] = mp
		in.Pieces = append(in.Pieces, mp)
	}
	out := predict.Plan(in)
	return &plan{
		out:      out,
		model:    out.Model,
		lc:       predict.LiveContext{Offline: offline, SensorGaps: gaps},
		history:  history,
		settings: settings,
		offline:  offline,
	}, nil
}

// modelPiece converts stored rows to the model's view of a piece.
func modelPiece(p store.Piece, obs []store.Observation, unknown bool) predict.Piece {
	mp := predict.Piece{
		ID: p.ID, Number: p.Number, Form: p.Form, Size: p.Size, Thickness: p.Thickness,
		AddedAt: p.AddedAt, Ended: p.EndedAt != nil, Unknown: unknown,
	}
	if p.GroupID != nil {
		mp.GroupID = *p.GroupID
	}
	for _, o := range obs {
		mp.Obs = append(mp.Obs, predict.Obs{ID: o.ID, Dryness: o.Dryness, At: o.ObservedAt, Source: o.Source})
	}
	return mp
}
