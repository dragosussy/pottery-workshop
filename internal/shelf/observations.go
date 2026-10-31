package shelf

import (
	"context"
	"errors"
	"fmt"
	"time"

	"humidity/internal/predict"
	"humidity/internal/store"
)

// ObservationView is the Observation resource (api-contract §3.9).
type ObservationView struct {
	ID         int64   `json:"id"`
	PieceID    int64   `json:"piece_id"`
	Dryness    float64 `json:"dryness"`
	ObservedAt string  `json:"observed_at"`
	Source     string  `json:"source"`
	Flag       *string `json:"flag"`
	Note       string  `json:"note"`
	CreatedAt  string  `json:"created_at"`
}

// NewObservations is the POST /observations body.
type NewObservations struct {
	ObservedAt *string       `json:"observed_at"`
	Items      *[]NewObsItem `json:"items"`
	Note       *string       `json:"note"`
}

// NewObsItem is one piece's dryness in NewObservations.
type NewObsItem struct {
	PieceID *int64   `json:"piece_id"`
	Dryness *float64 `json:"dryness"`
}

// ObservationPatch is the PATCH /observations/{id} body.
type ObservationPatch struct {
	Dryness    Opt[float64] `json:"dryness"`
	ObservedAt Opt[string]  `json:"observed_at"`
	Note       Opt[string]  `json:"note"`
}

// FollowUpView is a question about a new observation.
type FollowUpView struct {
	Kind                  string           `json:"kind"`
	PieceID               int64            `json:"piece_id"`
	ObservationID         int64            `json:"observation_id"`
	PreviousObservationID *int64           `json:"previous_observation_id"`
	Question              string           `json:"question"`
	Options               []FollowUpOption `json:"options"`
}

// FollowUpOption is one answer, with what the FE should do for it.
type FollowUpOption struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Action string `json:"action"`
}

// Created is the POST /observations response.
type Created struct {
	Items     []ObservationView `json:"items"`
	FollowUps []FollowUpView    `json:"follow_ups"`
}

var followUpText = map[string]struct {
	question string
	options  []FollowUpOption
}{
	predict.FollowUpWentWetter: {"Wetter than last time. What happened?", []FollowUpOption{
		{"last_entry_wrong", "Last entry was wrong", "correct_observation"},
		{"its_fine", "It's fine", "none"},
	}},
	predict.FollowUpMuchSlower: {"Drying much slower than expected. What happened?", []FollowUpOption{
		{"still_drying", "Still drying", "none"},
		{"covered_or_removed", "I covered it / took it off", "end_piece"},
	}},
}

// flags returns the inconsistent observations among a piece's live ones
// (more than 0.5 wetter than the one before).
func flags(obs []store.Observation) map[int64]bool {
	out := map[int64]bool{}
	for i := 1; i < len(obs); i++ {
		if predict.Inconsistent(obs[i-1].Dryness, obs[i].Dryness) {
			out[obs[i].ID] = true
		}
	}
	return out
}

func obsView(o store.Observation, inconsistent bool) ObservationView {
	v := ObservationView{
		ID: o.ID, PieceID: o.PieceID, Dryness: round1(o.Dryness), ObservedAt: ts(o.ObservedAt),
		Source: o.Source, Note: o.Note, CreatedAt: ts(o.CreatedAt),
	}
	if inconsistent {
		v.Flag = ptr("inconsistent")
	}
	return v
}

// obsViews builds views for observations, computing flags from each piece's
// live history.
func (s *Service) obsViews(ctx context.Context, q store.Querier, list []store.Observation) ([]ObservationView, error) {
	var ids []int64
	seen := map[int64]bool{}
	for _, o := range list {
		if !seen[o.PieceID] {
			ids = append(ids, o.PieceID)
			seen[o.PieceID] = true
		}
	}
	byPiece, err := store.PieceObservations(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	flagged := map[int64]bool{}
	for _, obs := range byPiece {
		for id := range flags(obs) {
			flagged[id] = true
		}
	}
	out := make([]ObservationView, len(list))
	for i, o := range list {
		out[i] = obsView(o, flagged[o.ID])
	}
	return out, nil
}

// Observe records dryness for one or more pieces, all or nothing, and
// returns any follow-up questions (api-contract §3.9). For each new
// observation it stores what was predicted for that moment, before the
// observation was known (mid-batch-changes.md §8.6).
func (s *Service) Observe(ctx context.Context, in NewObservations) (Created, error) {
	now := s.Now().UTC().Truncate(time.Second)
	v := &ValidationError{}
	at := now
	if in.ObservedAt != nil {
		if t, ok := parseTime(v, "observed_at", *in.ObservedAt, now); ok {
			at = t
		}
	}
	note := checkText(v, "note", in.Note, maxNotes)
	var items []NewObsItem
	if in.Items == nil || len(*in.Items) == 0 {
		v.add("items", CodeRequired, "list at least one piece")
	} else if items = *in.Items; len(items) > 50 {
		v.add("items", CodeOutOfRange, "at most 50 pieces at once")
	}
	seen := map[int64]bool{}
	ds := make([]float64, len(items))
	for i, it := range items {
		f := fmt.Sprintf("items[%d]", i)
		if it.PieceID == nil {
			v.add(f+".piece_id", CodeRequired, "required")
		} else if seen[*it.PieceID] {
			v.add(f+".piece_id", CodeDuplicate, "listed twice")
		} else {
			seen[*it.PieceID] = true
		}
		if it.Dryness == nil {
			v.add(f+".dryness", CodeRequired, "required")
		} else {
			ds[i], _ = dryness(v, f+".dryness", *it.Dryness)
		}
	}
	if err := v.err(); err != nil {
		return Created{}, err
	}

	pl, err := s.plan(ctx)
	if err != nil {
		return Created{}, err
	}
	climate := s.climate(pl, at)

	var out Created
	var created []store.Observation
	var touched []int64
	err = s.st.Tx(ctx, func(q store.Querier) error {
		for i, it := range items {
			f := fmt.Sprintf("items[%d]", i)
			p, err := store.GetPiece(ctx, q, *it.PieceID)
			if errors.Is(err, store.ErrNotFound) || err == nil && p.Mistake {
				v.add(f+".piece_id", CodeInvalidValue, "no such piece")
				continue
			}
			if err != nil {
				return err
			}
			if p.EndedAt != nil {
				return errorf(ErrPieceEnded, "piece %d is %s; put it back on the shelf first", p.ID, p.EndKind)
			}
			if at.Before(p.AddedAt) {
				v.add("observed_at", CodeBeforeAdded, fmt.Sprintf("must not be before piece %d was added", p.ID))
				continue
			}
			byPiece, err := store.PieceObservations(ctx, q, []int64{p.ID})
			if err != nil {
				return err
			}
			var prior []store.Observation
			for _, o := range byPiece[p.ID] {
				if !o.ObservedAt.After(at) {
					prior = append(prior, o)
				}
			}

			// What the model expected at this moment, before this check.
			pred := pl.model.Live(modelPiece(p, prior, false), climate, pl.lc)
			o := store.Observation{PieceID: p.ID, Dryness: ds[i], ObservedAt: at, Source: store.SourceUser, Note: note, CreatedAt: now}
			if o.ID, err = store.InsertObservation(ctx, q, o); err != nil {
				return err
			}
			created = append(created, o)
			touched = append(touched, p.ID)
			snap := store.Snapshot{
				ObservationID: o.ID, PieceID: p.ID, At: now, ETA: pred.ETA, ETALow: pred.ETALow,
				ETAHigh: pred.ETAHigh, ReadyUntil: pred.ReadyUntil, Method: pred.Method,
			}
			var predicted *float64
			if pred.Anchor != nil {
				predicted = ptr(pred.DrynessAt)
				snap.DrynessPred = predicted
			}
			if err := store.InsertSnapshot(ctx, q, snap); err != nil {
				return err
			}

			var prev *store.Observation
			if n := len(prior); n > 0 {
				prev = &prior[n-1]
			}
			var prevD *float64
			if prev != nil {
				prevD = &prev.Dryness
			}
			kind := predict.FollowUp(o.Dryness, prevD, predicted, pl.out.Learning.Learning)
			if kind == "" {
				continue
			}
			asked, err := store.InsertFollowUp(ctx, q, store.FollowUp{PieceID: p.ID, ObservationID: o.ID, Kind: kind, AskedAt: now})
			if err != nil {
				return err
			}
			if asked {
				fu := FollowUpView{Kind: kind, PieceID: p.ID, ObservationID: o.ID,
					Question: followUpText[kind].question, Options: followUpText[kind].options}
				if prev != nil {
					fu.PreviousObservationID = &prev.ID
				}
				out.FollowUps = append(out.FollowUps, fu)
			}
		}
		if err := v.err(); err != nil {
			return err
		}
		out.Items, err = s.obsViews(ctx, q, created)
		return err
	})
	if err != nil {
		return Created{}, err
	}
	if out.FollowUps == nil {
		out.FollowUps = []FollowUpView{}
	}
	s.changed([]string{ScopeObservations, ScopePredictions, ScopePieces}, touched)
	return out, nil
}

// liveObservation loads an observation that isn't deleted.
func liveObservation(ctx context.Context, q store.Querier, id int64) (store.Observation, error) {
	o, err := store.GetObservation(ctx, q, id)
	if errors.Is(err, store.ErrNotFound) || err == nil && o.DeletedAt != nil {
		return store.Observation{}, errorf(ErrNotFound, "no observation %d", id)
	}
	return o, err
}

// PieceObservations returns a piece's live observations, oldest first.
func (s *Service) PieceObservations(ctx context.Context, pieceID int64) ([]ObservationView, error) {
	q := s.st.DB()
	if _, err := store.GetPiece(ctx, q, pieceID); errors.Is(err, store.ErrNotFound) {
		return nil, errorf(ErrNotFound, "no piece %d", pieceID)
	} else if err != nil {
		return nil, err
	}
	byPiece, err := store.PieceObservations(ctx, q, []int64{pieceID})
	if err != nil {
		return nil, err
	}
	return s.obsViews(ctx, q, byPiece[pieceID])
}

// RecentObservations lists observations newest first.
func (s *Service) RecentObservations(ctx context.Context, cursor string, limit int) (Page[ObservationView], error) {
	var after *store.ObsCursor
	if cursor != "" {
		after = &store.ObsCursor{}
		if err := decodeCursor(cursor, after); err != nil {
			return Page[ObservationView]{}, err
		}
	}
	q := s.st.DB()
	list, next, err := store.ObservationsPage(ctx, q, after, limit)
	if err != nil {
		return Page[ObservationView]{}, err
	}
	items, err := s.obsViews(ctx, q, list)
	if err != nil {
		return Page[ObservationView]{}, err
	}
	page := Page[ObservationView]{Items: items}
	if next != nil {
		page.NextCursor = encodeCursor(next)
	}
	return page, nil
}

// UpdateObservation corrects an observation. Moving an add, flip or end
// observation moves the piece's added_at, flipped_at or ended_at with it.
func (s *Service) UpdateObservation(ctx context.Context, id int64, patch ObservationPatch) (ObservationView, error) {
	now := s.Now().UTC().Truncate(time.Second)
	var out ObservationView
	var pieceID int64
	err := s.st.Tx(ctx, func(q store.Querier) error {
		o, err := liveObservation(ctx, q, id)
		if err != nil {
			return err
		}
		pieceID = o.PieceID
		p, err := store.GetPiece(ctx, q, o.PieceID)
		if err != nil {
			return err
		}
		byPiece, err := store.PieceObservations(ctx, q, []int64{p.ID})
		if err != nil {
			return err
		}
		v := &ValidationError{}
		if patch.Dryness.Set {
			if d, ok := patch.Dryness.Value(); !ok {
				v.add("dryness", CodeRequired, "must not be null")
			} else if d, ok = dryness(v, "dryness", d); ok {
				o.Dryness = d
			}
		}
		if patch.Note.Set {
			n, ok := patch.Note.Value()
			switch {
			case !ok:
				v.add("note", CodeInvalidValue, "must not be null; send \"\" to clear")
			case len([]rune(n)) > maxNotes:
				v.add("note", CodeTooLong, "too long")
			default:
				o.Note = n
			}
		}
		if patch.ObservedAt.Set {
			raw, ok := patch.ObservedAt.Value()
			if !ok {
				v.add("observed_at", CodeInvalidValue, "must not be null")
			} else if t, ok := parseTime(v, "observed_at", raw, now); ok {
				checkObsTime(v, p, o, byPiece[p.ID], t)
				o.ObservedAt = t
				switch o.Source {
				case store.SourceAdd:
					p.AddedAt = t
				case store.SourceFlip:
					if p.FlippedAt != nil {
						p.FlippedAt = &t
					}
				case store.SourceEnd:
					if p.EndedAt != nil {
						p.EndedAt = &t
					}
				}
			}
		}
		if err := v.err(); err != nil {
			return err
		}
		if err := store.UpdateObservation(ctx, q, o); err != nil {
			return err
		}
		if err := store.UpdatePiece(ctx, q, p); err != nil {
			return err
		}
		views, err := s.obsViews(ctx, q, []store.Observation{o})
		if err != nil {
			return err
		}
		out = views[0]
		return nil
	})
	if err != nil {
		return ObservationView{}, err
	}
	s.changed([]string{ScopeObservations, ScopePredictions, ScopePieces}, []int64{pieceID})
	return out, nil
}

// checkObsTime applies the time rules to a moved observation.
func checkObsTime(v *ValidationError, p store.Piece, o store.Observation, all []store.Observation, t time.Time) {
	switch o.Source {
	case store.SourceAdd:
		for _, x := range all {
			if x.ID != o.ID && x.ObservedAt.Before(t) {
				v.add("observed_at", CodeInvalidTime, "the add check must come before the piece's other checks")
				return
			}
		}
		if p.FlippedAt != nil && p.FlippedAt.Before(t) || p.EndedAt != nil && p.EndedAt.Before(t) {
			v.add("observed_at", CodeInvalidTime, "the add check must come before the flip and end")
		}
	case store.SourceEnd:
		if t.Before(p.AddedAt) {
			v.add("observed_at", CodeBeforeAdded, "must not be before the piece was added")
			return
		}
		for _, x := range all {
			if x.ID != o.ID && x.ObservedAt.After(t) {
				v.add("observed_at", CodeInvalidTime, "the end must come after the piece's other checks")
				return
			}
		}
		if p.FlippedAt != nil && t.Before(*p.FlippedAt) {
			v.add("observed_at", CodeInvalidTime, "must not be before the flip")
		}
	default:
		if t.Before(p.AddedAt) {
			v.add("observed_at", CodeBeforeAdded, "must not be before the piece was added")
		} else if p.EndedAt != nil && t.After(*p.EndedAt) {
			v.add("observed_at", CodeAfterEnded, "must not be after the piece left the shelf")
		}
	}
}

// DeleteObservation soft-deletes an observation. A piece's only
// observation can't be deleted.
func (s *Service) DeleteObservation(ctx context.Context, id int64) error {
	now := s.Now().UTC()
	var pieceID int64
	err := s.st.Tx(ctx, func(q store.Querier) error {
		o, err := liveObservation(ctx, q, id)
		if err != nil {
			return err
		}
		pieceID = o.PieceID
		byPiece, err := store.PieceObservations(ctx, q, []int64{o.PieceID})
		if err != nil {
			return err
		}
		if len(byPiece[o.PieceID]) <= 1 {
			return errorf(ErrLastObservation, "observation %d is the piece's only one", id)
		}
		o.DeletedAt = &now
		return store.UpdateObservation(ctx, q, o)
	})
	if err != nil {
		return err
	}
	s.changed([]string{ScopeObservations, ScopePredictions, ScopePieces}, []int64{pieceID})
	return nil
}

// UndeleteObservation restores a deleted observation.
func (s *Service) UndeleteObservation(ctx context.Context, id int64) (ObservationView, error) {
	var out ObservationView
	err := s.st.Tx(ctx, func(q store.Querier) error {
		o, err := store.GetObservation(ctx, q, id)
		if errors.Is(err, store.ErrNotFound) {
			return errorf(ErrNotFound, "no observation %d", id)
		}
		if err != nil {
			return err
		}
		o.DeletedAt = nil
		if err := store.UpdateObservation(ctx, q, o); err != nil {
			return err
		}
		views, err := s.obsViews(ctx, q, []store.Observation{o})
		if err != nil {
			return err
		}
		out = views[0]
		return nil
	})
	if err != nil {
		return ObservationView{}, err
	}
	s.changed([]string{ScopeObservations, ScopePredictions, ScopePieces}, []int64{out.PieceID})
	return out, nil
}

// FollowUpAnswer is the POST /observations/{id}/follow-up body.
type FollowUpAnswer struct {
	Answer *string `json:"answer"`
}

// AnswerFollowUp records the answer to the question asked about an
// observation.
func (s *Service) AnswerFollowUp(ctx context.Context, obsID int64, in FollowUpAnswer) error {
	q := s.st.DB()
	if _, err := liveObservation(ctx, q, obsID); err != nil {
		return err
	}
	fu, err := store.FollowUpForObservation(ctx, q, obsID)
	if errors.Is(err, store.ErrNotFound) {
		return errorf(ErrNotFound, "no question was asked about observation %d", obsID)
	}
	if err != nil {
		return err
	}
	if in.Answer == nil {
		return Invalid("answer", CodeRequired, "required")
	}
	valid := false
	for _, o := range followUpText[fu.Kind].options {
		valid = valid || o.Key == *in.Answer
	}
	if !valid {
		return Invalid("answer", CodeInvalidEnum, "not an option for this question")
	}
	if err := store.AnswerFollowUp(ctx, q, fu.PieceID, *in.Answer); err != nil {
		return err
	}
	s.changed([]string{ScopeObservations, ScopePredictions}, []int64{fu.PieceID})
	return nil
}
