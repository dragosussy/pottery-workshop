// Package predict estimates when pieces on the shelf will be leather hard.
// See docs/apps/humidity/design.md §3.
//
// Clay dries at a speed set by how dry the air is, 100 − RH. Rates are kept
// at a reference humidity (RefRH): what a piece did is converted using the
// humidity recorded while it was watched, and its future is projected at the
// humidity the shelf is expected to have (the target), so a target change
// moves every estimate at once.
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

// RefRH is the humidity the priors are for: a damp, plastic-wrapped shelf.
const RefRH = 80

// Speed is how fast clay dries at a humidity, relative to RefRH.
func Speed(rh float64) float64 { return max(100-rh, 1) / (100 - RefRH) }

// Climate is the shelf's humidity, past and expected.
type Climate struct {
	// MeanRH is the mean recorded humidity between two times, or false when
	// nothing was recorded then.
	MeanRH func(from, to time.Time) (float64, bool)
	// Ahead is the humidity expected from now on: the target, or the latest
	// reading without one.
	Ahead float64
}

// speed is the drying speed over a past stretch; without readings for it,
// the expected humidity is the best guess.
func (c Climate) speed(from, to time.Time) float64 {
	if c.MeanRH != nil {
		if rh, ok := c.MeanRH(from, to); ok {
			return Speed(rh)
		}
	}
	return Speed(c.Ahead)
}

var hoursPerUnit = map[string]float64{"thin": 6, "medium": 12, "thick": 24}

var sizeFactor = map[string]float64{"small": 0.75, "medium": 1, "large": 1.4, "xlarge": 1.9}

// Prior is the expected drying rate (dryness units per hour at RefRH) from
// the build.
func Prior(size, thickness string) float64 {
	return 1 / (hoursPerUnit[thickness] * sizeFactor[size])
}

// StudioFactor learns how much faster (k > 1) or slower than the priors this
// studio dries, from trimmed pieces watched from below 2.8 to at least 2.8
// over an hour or more. n is the number of such samples.
func StudioFactor(pieces []Piece, c Climate) (k float64, n int) {
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
		r := (last.Dryness - first.Dryness) / h / c.speed(first.At, last.At)
		sum += math.Log(r / Prior(p.Size, p.Thickness))
		n++
	}
	// The +1 is a pseudo-sample at k = 1, so a single odd piece can't swing it.
	return math.Exp(sum / float64(n+1)), n
}

// Rate is the piece's drying rate at RefRH: the studio's rate for its build,
// blended with its own observed slope the longer it has been watched.
func Rate(p Piece, k float64, c Climate) float64 {
	base := k * Prior(p.Size, p.Thickness)
	first, last := p.Checks[0], p.Checks[len(p.Checks)-1]
	h := hours(first.At, last.At)
	if h < 1 {
		return base
	}
	own := (last.Dryness - first.Dryness) / h / c.speed(first.At, last.At)
	if own <= 0 {
		return base
	}
	w := min(h/12, 6)
	own = min(max(own, 0.1*base), 10*base)
	return math.Exp((w*math.Log(own) + 2*math.Log(base)) / (w + 2))
}

// Run estimates every piece still on the shelf (ended pieces get none) and
// returns the estimates by piece ID with the number of samples behind k.
// Since the last check a piece dried at the humidity recorded; from now on
// it dries at c.Ahead.
func Run(pieces []Piece, now time.Time, c Climate) (map[int64]Estimate, int) {
	k, n := StudioFactor(pieces, c)
	ahead := Speed(c.Ahead)
	out := make(map[int64]Estimate)
	for _, p := range pieces {
		if p.Ended {
			continue
		}
		rate := Rate(p, k, c)
		last := p.Checks[len(p.Checks)-1]
		dried := rate * c.speed(last.At, now) * hours(last.At, now)
		e := Estimate{Dryness: min(last.Dryness+dried, 5)}
		future := rate * ahead
		switch {
		case e.Dryness < ReadyFrom:
			e.Status = "drying"
			e.ReadyAt = after(now, (Target-e.Dryness)/future)
			e.ReadyUntil = after(now, (ReadyUntil-e.Dryness)/future)
		case e.Dryness <= ReadyUntil:
			e.Status = "ready"
			e.ReadyUntil = after(now, (ReadyUntil-e.Dryness)/future)
		default:
			e.Status = "past"
		}
		out[p.ID] = e
	}
	return out, n
}

func hours(from, to time.Time) float64 { return to.Sub(from).Hours() }

func after(t time.Time, hours float64) *time.Time {
	x := t.Add(time.Duration(hours * float64(time.Hour))).Truncate(time.Second).UTC()
	return &x
}
