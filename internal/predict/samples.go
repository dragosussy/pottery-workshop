package predict

import (
	"math"
	"slices"
	"time"
)

// Sample is one training sample: how fast a piece dried from its add
// observation to the target (mid-batch-changes.md §8.4).
type Sample struct {
	PieceID   int64
	GroupID   int64
	Prior     float64   // prior rate for the piece
	Rate      float64   // observed dryness units per effective hour
	Weight    float64   // 0.5–1, lower when the crossing is poorly bracketed
	ReachedAt time.Time // when it reached the target
}

// Resid is the sample's log residual against its prior.
func (s Sample) Resid() float64 { return math.Log(s.Rate / s.Prior) }

// clusterWindow merges observations this close together, e.g. two people
// checking the same piece, into their median.
const clusterWindow = 30 * time.Minute

// cluster replaces runs of observations within clusterWindow of the run's
// first one by one observation at the median dryness and median time.
func cluster(obs []Obs) []Obs {
	var out []Obs
	for i := 0; i < len(obs); {
		j := i + 1
		for j < len(obs) && obs[j].At.Sub(obs[i].At) <= clusterWindow {
			j++
		}
		run := obs[i:j]
		if len(run) == 1 {
			out = append(out, run[0])
		} else {
			ds := make([]float64, len(run))
			for k, o := range run {
				ds[k] = o.Dryness
			}
			slices.Sort(ds)
			med := (ds[(len(ds)-1)/2] + ds[len(ds)/2]) / 2
			out = append(out, Obs{ID: run[0].ID, Dryness: med, At: run[len(run)/2].At, Source: run[0].Source})
		}
		i = j
	}
	return out
}

// PieceSample extracts a piece's training sample, if it has a usable one:
//   - it starts at the add observation, below the band
//   - it reached the target, seen by a user, flip or end observation; the
//     crossing is interpolated in dose between the last observation below
//     and the first at or above the target
//   - no observation is inconsistent (checked after merging checks made
//     within 30 minutes into their median), no follow-up was skipped, and
//     no filled-in (offline or sensor-gap) dose lies between start and
//     crossing
//   - the dose gap between the bracketing observations is no larger than
//     the dose to the crossing
func PieceSample(p Piece, d Doser) (Sample, bool) {
	if p.Unknown || len(p.Obs) < 2 || p.Obs[0].Source != SourceAdd || p.Obs[0].Dryness >= BandLow {
		return Sample{}, false
	}
	obs := cluster(p.Obs)
	for i := 1; i < len(obs); i++ {
		if Inconsistent(obs[i-1].Dryness, obs[i].Dryness) {
			return Sample{}, false
		}
	}
	start := obs[0]
	b := slices.IndexFunc(obs, func(o Obs) bool { return o.Dryness >= Target })
	if b < 1 {
		return Sample{}, false
	}
	a := obs[b-1]
	ea, _ := d.Dose(start.At, a.At)
	eb, filled := d.Dose(start.At, obs[b].At)
	if filled > 0 || eb <= ea {
		return Sample{}, false
	}
	frac := (Target - a.Dryness) / (obs[b].Dryness - a.Dryness)
	cross := ea + frac*(eb-ea)
	gap := eb - ea
	if cross <= 0 || gap > cross {
		return Sample{}, false
	}
	reached := a.At.Add(time.Duration(frac * float64(obs[b].At.Sub(a.At))))
	return Sample{
		PieceID:   p.ID,
		GroupID:   p.GroupID,
		Prior:     PriorRate(p.Thickness, p.Size),
		Rate:      (Target - start.Dryness) / cross,
		Weight:    1 - 0.5*gap/cross,
		ReachedAt: reached,
	}, true
}

// collapseGroups keeps one sample per group of identical pieces: the
// member with the median residual, carrying the group's mean weight.
func collapseGroups(samples []Sample) []Sample {
	var out []Sample
	groups := make(map[int64][]Sample)
	var order []int64
	for _, s := range samples {
		if s.GroupID == 0 {
			out = append(out, s)
			continue
		}
		if _, seen := groups[s.GroupID]; !seen {
			order = append(order, s.GroupID)
		}
		groups[s.GroupID] = append(groups[s.GroupID], s)
	}
	for _, g := range order {
		members := groups[g]
		slices.SortFunc(members, func(a, b Sample) int { return cmpFloat(a.Resid(), b.Resid()) })
		med := members[(len(members)-1)/2]
		w := 0.0
		for _, m := range members {
			w += m.Weight
		}
		med.Weight = w / float64(len(members))
		out = append(out, med)
	}
	slices.SortFunc(out, func(a, b Sample) int { return a.ReachedAt.Compare(b.ReachedAt) })
	return out
}

// rejectOutliers drops samples whose residual is more than 3σ from the
// median residual: a piece that was covered but never marked, for example.
func rejectOutliers(samples []Sample) []Sample {
	if len(samples) < 3 {
		return samples
	}
	rs := make([]float64, len(samples))
	for i, s := range samples {
		rs[i] = s.Resid()
	}
	slices.Sort(rs)
	med := (rs[(len(rs)-1)/2] + rs[len(rs)/2]) / 2
	return slices.DeleteFunc(slices.Clone(samples), func(s Sample) bool {
		return math.Abs(s.Resid()-med) > 3*SigmaClass
	})
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
