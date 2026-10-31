package predict

import (
	"sort"
	"time"
)

// Doser returns the effective drying hours received between two past
// times, and the part of them filled in for missing data. The recorder's
// DoseIndex implements it.
type Doser interface {
	Dose(from, to time.Time) (eff, filled float64)
}

// maxHorizon caps forecasts, so a near-zero rate can't overflow a duration.
const maxHorizon = 3 * 365 * 24 * time.Hour

// Climate combines measured dose up to now with a forecast afterwards: the
// rate of the last 6 h for the next 6 h, then the rate of the last 24 h
// (mid-batch-changes.md §5.4, forecast_basis "recent").
type Climate struct {
	dose     Doser
	now      time.Time
	rate6    float64 // effective hours per hour
	rate24   float64
	minRate  float64
	forecast string
}

// NewClimate prepares a Climate as of now.
func NewClimate(d Doser, now time.Time) *Climate {
	c := &Climate{dose: d, now: now, minRate: EffRate(VPDFloor), forecast: "recent"}
	e6, _ := d.Dose(now.Add(-6*time.Hour), now)
	e24, _ := d.Dose(now.Add(-24*time.Hour), now)
	c.rate6 = max(e6/6, c.minRate)
	c.rate24 = max(e24/24, c.minRate)
	return c
}

// Now returns the time the climate was prepared for.
func (c *Climate) Now() time.Time { return c.now }

// Basis returns the forecast basis: "recent" (or "setpoint", later).
func (c *Climate) Basis() string { return c.forecast }

// Between returns the effective hours from a to b (measured, then
// forecast), and the filled part of the measured stretch.
func (c *Climate) Between(a, b time.Time) (eff, filled float64) {
	if !b.After(a) {
		return 0, 0
	}
	if a.Before(c.now) {
		eff, filled = c.dose.Dose(a, minTime(b, c.now))
	}
	if b.After(c.now) {
		eff += c.forecastDose(maxTime(a, c.now), b)
	}
	return eff, filled
}

// forecastDose integrates the forecast rate over [a, b], both ≥ now.
func (c *Climate) forecastDose(a, b time.Time) float64 {
	split := c.now.Add(6 * time.Hour)
	h := 0.0
	if a.Before(split) {
		h += hours(minTime(b, split).Sub(a)) * c.rate6
	}
	if b.After(split) {
		h += hours(b.Sub(maxTime(a, split))) * c.rate24
	}
	return h
}

// TimeAt returns when the effective hours counted from start reach need.
func (c *Climate) TimeAt(start time.Time, need float64) time.Time {
	if need <= 0 {
		return start
	}
	if start.Before(c.now) {
		measured, _ := c.dose.Dose(start, c.now)
		if measured >= need {
			// Binary search to the second inside the measured stretch.
			span := int(c.now.Sub(start) / time.Second)
			k := sort.Search(span, func(s int) bool {
				e, _ := c.dose.Dose(start, start.Add(time.Duration(s)*time.Second))
				return e >= need
			})
			return start.Add(time.Duration(k) * time.Second)
		}
		need -= measured
		start = c.now
	}
	split := c.now.Add(6 * time.Hour)
	if start.Before(split) {
		first := hours(split.Sub(start)) * c.rate6
		if need <= first {
			return start.Add(dur(need / c.rate6))
		}
		need -= first
		start = split
	}
	return start.Add(dur(need / c.rate24))
}

func hours(d time.Duration) float64 { return d.Hours() }

func dur(h float64) time.Duration {
	if h*float64(time.Hour) > float64(maxHorizon) {
		return maxHorizon
	}
	return time.Duration(h * float64(time.Hour)).Round(time.Second)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
