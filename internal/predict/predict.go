// Package predict estimates when pieces on the shelf will be leather hard.
// See docs/plan.md §3. Times are plain wall-clock hours.
package predict

import (
	"math"
	"time"
)

// Check is one dryness check (0.0–5.0).
type Check struct {
	Dryness float64
	At      time.Time
}

// Piece is what the model needs to know about a piece.
type Piece struct {
	ID        int64
	Size      string
	Thickness string
	Ended     bool
	Trimmed   bool    // ended as trimmed: a possible sample for k
	Checks    []Check // ordered by (at, id); never empty
}

// Estimate is the outcome for a piece on the shelf.
type Estimate struct {
	Dryness    float64    // estimated dryness now
	Status     string     // drying | ready | past
	ReadyAt    *time.Time // when it reaches 3.0; drying only
	ReadyUntil *time.Time // when it passes 3.6; drying or ready
}

// The trimmable band, and the trim target inside it.
const (
	ReadyFrom  = 2.8
	Target     = 3.0
	ReadyUntil = 3.6
)

var hoursPerUnit = map[string]float64{"thin": 6, "medium": 12, "thick": 24}

var sizeFactor = map[string]float64{"small": 0.75, "medium": 1, "large": 1.4, "xlarge": 1.9}

// Prior is the expected drying rate (dryness units per hour) from the build.
func Prior(size, thickness string) float64 {
	return 1 / (hoursPerUnit[thickness] * sizeFactor[size])
}

// StudioFactor learns how much faster (k > 1) or slower than the priors this
// studio dries, from trimmed pieces watched from below 2.8 to at least 2.8
// over an hour or more. n is the number of such samples.
func StudioFactor(pieces []Piece) (k float64, n int) {
	var sum float64
	for _, p := range pieces {
		if !p.Trimmed {
			continue
		}
		first, last := p.Checks[0], p.Checks[len(p.Checks)-1]
		h := hours(first.At, last.At)
		if first.Dryness >= ReadyFrom || last.Dryness < ReadyFrom || h < 1 {
			continue
		}
		r := (last.Dryness - first.Dryness) / h
		sum += math.Log(r / Prior(p.Size, p.Thickness))
		n++
	}
	// The +1 is a pseudo-sample at k = 1, so a single odd piece can't swing it.
	return math.Exp(sum / float64(n+1)), n
}

// Rate is the piece's drying rate: the studio's rate for its build, blended
// with its own observed slope the longer it has been watched.
func Rate(p Piece, k float64) float64 {
	base := k * Prior(p.Size, p.Thickness)
	first, last := p.Checks[0], p.Checks[len(p.Checks)-1]
	h := hours(first.At, last.At)
	if h < 1 {
		return base
	}
	own := (last.Dryness - first.Dryness) / h
	if own <= 0 {
		return base
	}
	w := min(h/12, 6)
	own = min(max(own, 0.1*base), 10*base)
	return math.Exp((w*math.Log(own) + 2*math.Log(base)) / (w + 2))
}

// Run estimates every piece still on the shelf (ended pieces get none) and
// returns the estimates by piece ID with the number of samples behind k.
func Run(pieces []Piece, now time.Time) (map[int64]Estimate, int) {
	k, n := StudioFactor(pieces)
	out := make(map[int64]Estimate)
	for _, p := range pieces {
		if p.Ended {
			continue
		}
		rate := Rate(p, k)
		last := p.Checks[len(p.Checks)-1]
		e := Estimate{Dryness: min(last.Dryness+rate*hours(last.At, now), 5)}
		switch {
		case e.Dryness < ReadyFrom:
			e.Status = "drying"
			e.ReadyAt = after(last.At, (Target-last.Dryness)/rate)
			e.ReadyUntil = after(last.At, (ReadyUntil-last.Dryness)/rate)
		case e.Dryness <= ReadyUntil:
			e.Status = "ready"
			e.ReadyUntil = after(last.At, (ReadyUntil-last.Dryness)/rate)
		default:
			e.Status = "past"
		}
		out[p.ID] = e
	}
	return out, n
}

func hours(from, to time.Time) float64 { return to.Sub(from).Hours() }

func after(t time.Time, hours float64) *time.Time {
	x := t.Add(time.Duration(hours * float64(time.Hour))).Truncate(time.Second)
	return &x
}
