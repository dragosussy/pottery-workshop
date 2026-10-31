package predict

import (
	"fmt"
	"math"
	"time"
)

// LiveContext is the climate log as far as one prediction needs it.
type LiveContext struct {
	Offline    []Interval // the device was switched off
	SensorGaps []Interval // a sensor was down for over an hour
}

// staleAfter is how old an anchor must be before it can count as stale.
const staleAfter = 12 * time.Hour

// Live predicts a piece's drying from its latest observation
// (mid-batch-changes.md §8.2, §8.5):
//
//  1. Anchor at the last observation d₀ at t₀.
//  2. Walk forward: d(t) = d₀ + rate × eff_hours(t₀, t), measured dose up
//     to now and forecast dose after.
//  3. eta is when d reaches the target; the 80 % window combines the rate
//     uncertainty (log scale, applied to all the dose since the anchor, so
//     it widens with time) with σ_obs and the uncertainty of filled dose.
//  4. ready_until is a conservative best-before anchored on eta: eta plus
//     the time from the target to the top of the band at the fast end of
//     the 80 % rate range. For a piece observed in the band it is the
//     observation time plus the fast-rate time from its value to the top.
//     Either way ready_until ≥ eta (mid-batch-changes.md §7.1, §8.2).
//
// Statuses use the central estimate; only ready_until is conservative.
func (m *Model) Live(p Piece, c *Climate, lc LiveContext) Prediction {
	now := c.Now()
	pred := Prediction{
		Basis:      c.Basis(),
		Method:     "learned",
		PastPieces: m.Samples,
		Confidence: ConfDecent,
		Reasons:    []string{},
	}
	if m.Learning {
		pred.Method = "prior"
		pred.Confidence = ConfLearning
	}
	if len(p.Obs) == 0 {
		pred.Status = StatusDrying
		if p.Ended {
			pred.Status = StatusEnded
		}
		pred.Summary = m.summary(p, 0)
		return pred
	}
	anchor := p.Obs[len(p.Obs)-1]
	pred.Anchor = &anchor

	base, sib := m.baseLogRate(p)
	baseRate := math.Exp(base + sib)
	eP, wP := ePiece(p, c, baseRate)
	rate := baseRate * math.Exp(eP)
	pred.Summary = m.summary(p, sib+eP)

	if p.Ended {
		pred.Status = StatusEnded
		return pred
	}

	sigma := SigmaClass
	if m.Learning {
		sigma = SigmaLearning
	}
	sigma = max(sigma*math.Sqrt(n0Piece/(n0Piece+wP)), sigmaFloor)
	fast := math.Exp(z80 * sigma)

	t0, d0 := anchor.At, anchor.Dryness
	elapsed, filled := c.Between(t0, now)
	pred.DrynessAt = min(d0+rate*elapsed, 5)
	dn := RoundDryness(pred.DrynessAt)
	pred.DrynessNow = &dn

	fillSD := z80 * sigmaFill * filled
	obsSD := z80 * SigmaObs / rate
	need := func(target float64) float64 { return (target - d0) / rate }
	hwFast := func(x, fill float64) float64 { return math.Sqrt(sq(x*(1-1/fast)) + sq(obsSD) + sq(fill)) }
	hwSlow := func(x, fill float64) float64 { return math.Sqrt(sq(x*(fast-1)) + sq(obsSD) + sq(fill)) }

	if d0 > BandHigh+1e-9 {
		pred.Status = StatusPastTrim
		pred.Confidence = m.confidence(sigma, nil)
		return pred
	}
	if d0 >= BandLow-1e-9 {
		readyUntil := c.TimeAt(t0, max(need(BandHigh)/fast-fillSD, 0))
		pred.ReadyUntil = &readyUntil
		if pred.DrynessAt < BandHigh {
			pred.Status = StatusReadyNow
			eta := t0
			pred.ETA, pred.ETALow, pred.ETAHigh = &eta, &eta, &eta
		} else {
			pred.Status = StatusMayBePast
		}
	} else {
		x := need(Target)
		eta := c.TimeAt(t0, x)
		low := c.TimeAt(t0, max(x-hwFast(x, fillSD), 0))
		high := c.TimeAt(t0, x+hwSlow(x, fillSD))
		// Measured from the predicted crossing of the target, even when it
		// lies in the past (may_be_past_check).
		readyUntil := c.TimeAt(eta, (BandHigh-Target)/(rate*fast))
		pred.ReadyUntil = &readyUntil
		switch {
		case pred.DrynessAt > BandHigh:
			pred.Status = StatusMayBePast
		case pred.DrynessAt >= BandLow:
			pred.Status = StatusProbablyReady
		default:
			pred.Status = StatusDrying
		}
		if pred.Status != StatusMayBePast {
			pred.ETA, pred.ETALow, pred.ETAHigh = &eta, &low, &high
		}
		// Stale: the window is much wider than a check now would make it.
		fresh := max(need(Target)-elapsed, 0)
		width := hwFast(x, fillSD) + hwSlow(x, fillSD)
		freshWidth := hwFast(fresh, 0) + hwSlow(fresh, 0)
		pred.Stale = now.Sub(t0) >= staleAfter && width >= 1.5*freshWidth
	}

	// Uncertainty reasons, dominant first.
	off, gap := false, false
	for _, iv := range lc.Offline {
		off = off || iv.overlaps(t0, now)
	}
	for _, iv := range lc.SensorGaps {
		gap = gap || iv.overlaps(t0, now)
	}
	gap = gap || (filled > 0 && !off)
	userChecks := 0
	for _, o := range p.Obs {
		if o.Source != SourceAdd {
			userChecks++
		}
	}
	reasons := []string{}
	add := func(ok bool, r string) {
		if ok {
			reasons = append(reasons, r)
		}
	}
	add(m.Learning, ReasonLearning)
	add(pred.Stale, ReasonStale)
	add(off, ReasonDeviceWasOff)
	add(gap, ReasonSensorGap)
	add(userChecks == 0 && now.Sub(p.AddedAt) >= staleAfter, ReasonFewChecks)
	pred.Reasons = reasons
	pred.Confidence = m.confidence(sigma, reasons)
	return pred
}

// confidence grades a prediction: learning during the learning period,
// otherwise by the rate uncertainty left after the piece's own checks,
// capped by what went wrong.
func (m *Model) confidence(sigma float64, reasons []string) string {
	if m.Learning {
		return ConfLearning
	}
	level := 2 // good
	switch {
	case sigma > SigmaClass-1e-9:
		level = 0
	case sigma > 0.3:
		level = 1
	}
	if m.Samples < 2*LearningSamples {
		level = min(level, 1)
	}
	for _, r := range reasons {
		switch r {
		case ReasonDeviceWasOff, ReasonSensorGap:
			level = 0
		case ReasonStale:
			level = min(level, 1)
		}
	}
	return [...]string{ConfRough, ConfDecent, ConfGood}[level]
}

var sizeWords = map[string]string{"small": "small", "medium": "medium", "large": "large", "xlarge": "extra-large"}

// summary explains the basis of the rate in one line.
func (m *Model) summary(p Piece, own float64) string {
	what := fmt.Sprintf("%s %s", sizeWords[p.Size], p.Form)
	if p.Form == "other" {
		what = sizeWords[p.Size] + " piece"
	}
	if m.Learning {
		switch m.Samples {
		case 0:
			return fmt.Sprintf("Default rates for a %s; no finished pieces yet", what)
		case 1:
			return fmt.Sprintf("Default rates for a %s, adjusted by 1 finished piece", what)
		}
		return fmt.Sprintf("Default rates for a %s, adjusted by %d finished pieces", what, m.Samples)
	}
	s := fmt.Sprintf("Learned from %d finished pieces", m.Samples)
	if pct := math.Round((math.Exp(own) - 1) * 100); math.Abs(pct) >= 10 {
		dir := "faster"
		if pct < 0 {
			dir, pct = "slower", -pct
		}
		s += fmt.Sprintf("; this one is drying %.0f%% %s than usual", pct, dir)
	}
	return s
}

func sq(x float64) float64 { return x * x }
