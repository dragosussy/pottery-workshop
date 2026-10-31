package predict

import "time"

// Candidate is a piece the visit planner considers: on the shelf, with a
// window, not yet past the band.
type Candidate struct {
	ID         int64
	ETA        time.Time // ready_now pieces: the time of the observation
	ETALow     time.Time
	ReadyUntil time.Time
}

// Visit is the recommended trip to the studio (api-contract §3.7).
type Visit struct {
	RecommendedAt time.Time
	Start, End    time.Time
	Deadline      time.Time
	Ready         []int64
	Maybe         []int64
	NotYet        []int64
	Score         float64
}

// score is a candidate's points at t: 1 when ready, 0.5 when maybe ready.
func (c Candidate) score(t time.Time) float64 {
	switch {
	case !t.Before(c.ETA) && !t.After(c.ReadyUntil):
		return 1
	case !t.Before(c.ETALow) && t.Before(c.ETA):
		return 0.5
	}
	return 0
}

// PlanVisit implements mid-batch-changes.md §7.2 over the given slots
// (Slots(now, loc); the first is now):
//
//  1. Candidates whose ready_until has already passed are not planned.
//  2. Deadline D: the earliest ready_until among the rest.
//  3. Each slot t ≤ D scores 1 per ready piece and 0.5 per maybe-ready one.
//  4. The best slot is the highest score, earliest on a tie; the window is
//     the run of following slots with the same score.
//  5. At that slot the pieces split into ready, maybe and not yet.
//
// Candidates are expected in display (shelf number) order; the ID lists
// keep it. It returns nil when no slot scores above 0.
func PlanVisit(slots []time.Time, cands []Candidate) *Visit {
	if len(slots) == 0 {
		return nil
	}
	now := slots[0]
	var rest []Candidate
	for _, c := range cands {
		if !c.ReadyUntil.Before(now) {
			rest = append(rest, c)
		}
	}
	if len(rest) == 0 {
		return nil
	}

	deadline := rest[0].ReadyUntil
	for _, c := range rest[1:] {
		if c.ReadyUntil.Before(deadline) {
			deadline = c.ReadyUntil
		}
	}

	scores := make([]float64, 0, len(slots))
	best := -1
	for i, t := range slots {
		if t.After(deadline) {
			break
		}
		s := 0.0
		for _, c := range rest {
			s += c.score(t)
		}
		scores = append(scores, s)
		if best < 0 || s > scores[best] {
			best = i
		}
	}
	if best < 0 || scores[best] <= 0 {
		return nil
	}

	end := best
	for end+1 < len(scores) && scores[end+1] == scores[best] {
		end++
	}
	at := slots[best]
	v := &Visit{
		RecommendedAt: at,
		Start:         at,
		End:           slots[end],
		Deadline:      deadline,
		Score:         scores[best],
		Ready:         []int64{},
		Maybe:         []int64{},
		NotYet:        []int64{},
	}
	for _, c := range rest {
		switch c.score(at) {
		case 1:
			v.Ready = append(v.Ready, c.ID)
		case 0.5:
			v.Maybe = append(v.Maybe, c.ID)
		default:
			v.NotYet = append(v.NotYet, c.ID)
		}
	}
	return v
}
