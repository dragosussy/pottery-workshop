package predict

import "math"

// The dryness scale and the trim band (mid-batch-changes.md §2). The band is
// a constant in v1.
const (
	Target   = 3.0 // leather hard
	BandLow  = 2.8
	BandHigh = 3.6
)

// Learning period and noise (mid-batch-changes.md §8.1, §8.2).
const (
	LearningSamples = 5   // training samples needed to leave the learning period
	SigmaLearning   = 0.7 // σ of ln rate during the learning period
	SigmaClass      = 0.4 // σ of ln rate afterwards
	SigmaObs        = 0.3 // gut-feeling noise of one observation, in dryness units
	sigmaFloor      = 0.15
	sigmaFill       = 0.5 // relative uncertainty of gap-filled dose
	z80             = 1.2816
)

// Shrinkage strengths of the adaptation factors (mid-batch-changes.md §8.3).
const (
	n0Studio     = 5
	n0Recent     = 3
	n0Sibling    = 2
	n0Piece      = 2
	recentPieces = 10
)

// Observation steps (mid-batch-changes.md §9).
const (
	InconsistentStep = 0.5 // wetter than the previous by more than this: flagged
	FollowUpStep     = 1.5 // wetter, or below the prediction, by at least this: ask
)

// hoursPerUnit is the prior in effective hours per unit of dryness for a
// medium piece, by wall thickness (§8.1). It only needs to be right in
// relative terms; a_studio corrects the overall scale.
var hoursPerUnit = map[string]float64{"thin": 3, "medium": 6, "thick": 12}

// sizeFactor multiplies the time by size.
var sizeFactor = map[string]float64{"small": 0.75, "medium": 1, "large": 1.4, "xlarge": 1.9}

// PriorRate returns the prior drying rate in dryness units per effective
// hour. Unknown keys fall back to medium.
func PriorRate(thickness, size string) float64 {
	h, ok := hoursPerUnit[thickness]
	if !ok {
		h = hoursPerUnit["medium"]
	}
	f, ok := sizeFactor[size]
	if !ok {
		f = 1
	}
	return 1 / (h * f)
}

// RoundDryness rounds to the 0.1 step of the slider.
func RoundDryness(d float64) float64 { return math.Round(d*10) / 10 }

// Inconsistent reports whether an observation is more than InconsistentStep
// wetter than the one before it. It is still used as the live anchor, but
// excluded from training.
func Inconsistent(prev, cur float64) bool { return prev-cur > InconsistentStep+1e-9 }
