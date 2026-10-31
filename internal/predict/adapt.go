package predict

import "math"

// Model holds what has been learned from history (mid-batch-changes.md
// §8.3): ln rate = ln prior + a_studio + e_recent + e_sibling + e_piece.
type Model struct {
	Samples  int  // training samples after collapsing groups and rejecting outliers
	Learning bool // fewer than LearningSamples
	AStudio  float64
	ERecent  float64
	// perPiece holds every member's own sample, for e_sibling.
	perPiece []Sample
}

// NewModel learns the studio and recent factors from every piece's
// history.
func NewModel(pieces []Piece, d Doser) *Model {
	var per []Sample
	for _, p := range pieces {
		if s, ok := PieceSample(p, d); ok {
			per = append(per, s)
		}
	}
	samples := rejectOutliers(collapseGroups(per))
	m := &Model{Samples: len(samples), Learning: len(samples) < LearningSamples, perPiece: per}

	// a_studio: every sample, shrunk toward 0.
	var sw, swr float64
	for _, s := range samples {
		sw += s.Weight
		swr += s.Weight * s.Resid()
	}
	m.AStudio = swr / (sw + n0Studio)

	// e_recent: the last pieces to reach the target, newest weighted 1 and
	// oldest 0.5, so a break between sessions doesn't erase it.
	recent := samples
	if len(recent) > recentPieces {
		recent = recent[len(recent)-recentPieces:]
	}
	sw, swr = 0, 0
	for i, s := range recent {
		order := 1.0
		if len(recent) > 1 {
			order = 0.5 + 0.5*float64(i)/float64(len(recent)-1)
		}
		w := s.Weight * order
		sw += w
		swr += w * (s.Resid() - m.AStudio)
	}
	m.ERecent = swr / (sw + n0Recent)
	return m
}

// baseLogRate is ln of the rate for a piece before its own observations:
// prior, studio, recent and sibling factors.
func (m *Model) baseLogRate(p Piece) (float64, float64) {
	base := math.Log(PriorRate(p.Thickness, p.Size)) + m.AStudio + m.ERecent
	return base, m.eSibling(p)
}

// eSibling learns from members of the same group that already reached the
// target.
func (m *Model) eSibling(p Piece) float64 {
	if p.GroupID == 0 {
		return 0
	}
	var sw, swr float64
	for _, s := range m.perPiece {
		if s.GroupID == p.GroupID && s.PieceID != p.ID {
			sw += s.Weight
			swr += s.Weight * (s.Resid() - m.AStudio - m.ERecent)
		}
	}
	return swr / (sw + n0Sibling)
}

// ePiece learns from the piece's own observations: the slope of dryness
// against effective hours from its first to its last observation, as a log
// residual against the base rate. The weight grows with the progress the
// base rate expects over that dose, in steps of half a dryness unit (about
// the size of the observation noise).
func ePiece(p Piece, c *Climate, baseRate float64) (e, w float64) {
	if len(p.Obs) < 2 {
		return 0, 0
	}
	first, last := p.Obs[0], p.Obs[len(p.Obs)-1]
	dose, _ := c.Between(first.At, last.At)
	if dose <= 0 {
		return 0, 0
	}
	slope := (last.Dryness - first.Dryness) / dose
	slope = max(slope, 0.1*baseRate) // no progress (covered?) still can't stop the clock
	w = min(dose*baseRate/0.5, 6)
	resid := math.Log(slope / baseRate)
	return w * resid / (w + n0Piece), w
}
