package shelf

import (
	"context"
	"time"

	"humidity/internal/predict"
	"humidity/internal/store"
)

// PieceView is the Piece resource (api-contract §3.3).
type PieceView struct {
	ID               int64          `json:"id"`
	Number           int64          `json:"number"`
	GroupID          *int64         `json:"group_id"`
	GroupSize        int            `json:"group_size"`
	Form             string         `json:"form"`
	Label            string         `json:"label"`
	Size             string         `json:"size"`
	Thickness        string         `json:"thickness"`
	PhotoID          *int64         `json:"photo_id"`
	Notes            string         `json:"notes"`
	State            string         `json:"state"`
	AddedAt          string         `json:"added_at"`
	FlippedAt        *string        `json:"flipped_at"`
	FlipDryness      *float64       `json:"flip_dryness"`
	EndedAt          *string        `json:"ended_at"`
	EndDryness       *float64       `json:"end_dryness"`
	EndNote          string         `json:"end_note"`
	Mistake          bool           `json:"mistake"`
	InitialDryness   *float64       `json:"initial_dryness"`
	CurrentDryness   *float64       `json:"current_dryness"`
	LastObservedAt   *string        `json:"last_observed_at"`
	ObservationCount int            `json:"observation_count"`
	Prediction       PredictionView `json:"prediction"`
}

// PredictionView is the PiecePrediction resource.
type PredictionView struct {
	Status             string      `json:"status"`
	ETA                *string     `json:"eta"`
	ETALow             *string     `json:"eta_low"`
	ETAHigh            *string     `json:"eta_high"`
	ReadyUntil         *string     `json:"ready_until"`
	DrynessNow         *float64    `json:"dryness_now"`
	Confidence         string      `json:"confidence"`
	UncertaintyReasons []string    `json:"uncertainty_reasons"`
	ForecastBasis      string      `json:"forecast_basis"`
	Basis              BasisView   `json:"basis"`
	Anchor             *AnchorView `json:"anchor"`
	ComputedAt         string      `json:"computed_at"`
}

// BasisView explains where a prediction's rate comes from.
type BasisView struct {
	Method     string `json:"method"`
	PastPieces int    `json:"past_pieces"`
	Summary    string `json:"summary"`
}

// AnchorView is the observation a prediction starts from.
type AnchorView struct {
	Dryness    float64 `json:"dryness"`
	ObservedAt string  `json:"observed_at"`
}

// state derives the lifecycle state from a row.
func state(p store.Piece) string {
	switch {
	case p.EndedAt != nil:
		return p.EndKind
	case p.FlippedAt != nil:
		return StateFlipped
	}
	return StateIn
}

// pieceView builds the resource. obs are the piece's live observations,
// oldest first.
func pieceView(p store.Piece, obs []store.Observation, groupSizes map[int64]int, pred predict.Prediction, computedAt time.Time) PieceView {
	v := PieceView{
		ID: p.ID, Number: p.Number, GroupID: p.GroupID, GroupSize: 1,
		Form: p.Form, Label: p.Label, Size: p.Size, Thickness: p.Thickness, PhotoID: p.PhotoID,
		Notes: p.Notes, State: state(p), AddedAt: ts(p.AddedAt), FlippedAt: tsp(p.FlippedAt),
		EndedAt: tsp(p.EndedAt), EndNote: p.EndNote, Mistake: p.Mistake,
		ObservationCount: len(obs),
		Prediction:       predictionView(pred, computedAt),
	}
	if p.GroupID != nil {
		v.GroupSize = max(groupSizes[*p.GroupID], 1)
	}
	for _, o := range obs {
		d := round1(o.Dryness)
		switch o.Source {
		case store.SourceAdd:
			v.InitialDryness = &d
		case store.SourceFlip:
			v.FlipDryness = &d
		case store.SourceEnd:
			v.EndDryness = &d
		}
	}
	if n := len(obs); n > 0 {
		last := obs[n-1]
		v.CurrentDryness = ptr(round1(last.Dryness))
		v.LastObservedAt = tsp(&last.ObservedAt)
	}
	return v
}

func predictionView(p predict.Prediction, computedAt time.Time) PredictionView {
	v := PredictionView{
		Status:             p.Status,
		ETA:                tsp(p.ETA),
		ETALow:             tsp(p.ETALow),
		ETAHigh:            tsp(p.ETAHigh),
		ReadyUntil:         tsp(p.ReadyUntil),
		DrynessNow:         p.DrynessNow,
		Confidence:         p.Confidence,
		UncertaintyReasons: p.Reasons,
		ForecastBasis:      p.Basis,
		Basis:              BasisView{Method: p.Method, PastPieces: p.PastPieces, Summary: p.Summary},
		ComputedAt:         ts(computedAt),
	}
	if v.UncertaintyReasons == nil {
		v.UncertaintyReasons = []string{}
	}
	if v.ForecastBasis == "" {
		v.ForecastBasis = "recent"
	}
	if p.Anchor != nil {
		v.Anchor = &AnchorView{Dryness: round1(p.Anchor.Dryness), ObservedAt: ts(p.Anchor.At)}
	}
	return v
}

// views builds resources for pieces, loading their observations and using
// the current predictions. Pieces missing from the plan (mistakes, or rows
// written since it was computed) are predicted on the spot.
func (s *Service) views(ctx context.Context, pieces []store.Piece) ([]PieceView, error) {
	q := s.st.DB()
	ids := make([]int64, len(pieces))
	for i, p := range pieces {
		ids[i] = p.ID
	}
	obs, err := store.PieceObservations(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	sizes, err := store.GroupSizes(ctx, q)
	if err != nil {
		return nil, err
	}
	pl, err := s.plan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PieceView, 0, len(pieces))
	for _, p := range pieces {
		pred, ok := pl.out.Predictions[p.ID]
		if !ok {
			climate := s.climate(pl, pl.out.ComputedAt)
			pred = pl.model.Live(modelPiece(p, obs[p.ID], false), climate, pl.lc)
		}
		out = append(out, pieceView(p, obs[p.ID], sizes, pred, pl.out.ComputedAt))
	}
	return out, nil
}

func (s *Service) view(ctx context.Context, p store.Piece) (PieceView, error) {
	vs, err := s.views(ctx, []store.Piece{p})
	if err != nil {
		return PieceView{}, err
	}
	return vs[0], nil
}
