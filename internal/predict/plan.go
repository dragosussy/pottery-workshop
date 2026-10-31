package predict

import (
	"slices"
	"time"
)

// Input is everything Plan needs.
type Input struct {
	Now time.Time
	// Pieces holds every piece not entered by mistake, on the shelf and
	// ended: the ended ones are the training history.
	Pieces     []Piece
	Dose       Doser
	Offline    []Interval // oldest first
	SensorGaps []Interval
	Loc        *time.Location // studio time zone: slots fall on its :00 and :30
}

// Learning is the learning-period state (api-contract §3.2).
type Learning struct {
	Learning bool
	Samples  int
	Needed   int
}

// Output is the shelf-wide result.
type Output struct {
	ComputedAt  time.Time
	Learning    Learning
	Predictions map[int64]Prediction // every piece in Input.Pieces
	Visit       *Visit
	VisitConf   string
	Checks      []Check
	Model       *Model // what was learned, for evaluating single pieces
}

// Plan predicts every piece and plans the next visit.
func Plan(in Input) Output {
	model := NewModel(in.Pieces, in.Dose)
	climate := NewClimate(in.Dose, in.Now)
	lc := LiveContext{Offline: in.Offline, SensorGaps: in.SensorGaps}

	out := Output{
		ComputedAt:  in.Now,
		Learning:    Learning{Learning: model.Learning, Samples: model.Samples, Needed: LearningSamples},
		Predictions: make(map[int64]Prediction, len(in.Pieces)),
		Model:       model,
	}

	var onShelf []Piece
	for _, p := range in.Pieces {
		out.Predictions[p.ID] = model.Live(p, climate, lc)
		if !p.Ended {
			onShelf = append(onShelf, p)
		}
	}
	slices.SortFunc(onShelf, func(a, b Piece) int { return int(a.Number - b.Number) })

	var cands []Candidate
	var checks []CheckInput
	for _, p := range onShelf {
		pr := out.Predictions[p.ID]
		checks = append(checks, CheckInput{Piece: p, Pred: pr})
		switch pr.Status {
		case StatusDrying, StatusProbablyReady, StatusReadyNow:
			cands = append(cands, Candidate{ID: p.ID, ETA: *pr.ETA, ETALow: *pr.ETALow, ReadyUntil: *pr.ReadyUntil})
		}
	}

	loc := in.Loc
	if loc == nil {
		loc = time.UTC
	}
	out.Visit = PlanVisit(Slots(in.Now, loc), cands)
	if out.Visit != nil {
		out.VisitConf = visitConfidence(model, out.Visit, out.Predictions)
	}

	var last *Interval
	if n := len(in.Offline); n > 0 {
		last = &in.Offline[n-1]
	}
	out.Checks = Checks(in.Now, model.Learning, last, checks)
	return out
}

// visitConfidence is the weakest confidence among the pieces the visit
// counts on.
func visitConfidence(m *Model, v *Visit, preds map[int64]Prediction) string {
	if m.Learning {
		return ConfLearning
	}
	rank := map[string]int{ConfRough: 0, ConfDecent: 1, ConfGood: 2}
	level := 2
	for _, id := range append(slices.Clone(v.Ready), v.Maybe...) {
		level = min(level, rank[preds[id].Confidence])
	}
	return [...]string{ConfRough, ConfDecent, ConfGood}[level]
}
