package recorder

import (
	"context"
	"sort"
	"sync"
	"time"

	"humidity/internal/predict"
	"humidity/internal/store"
)

// fillAfter is the gap length from which missing minutes are filled with
// the mean of the day before the gap instead of carrying the last value
// forward (mid-batch-changes.md §5.3).
const fillAfter = time.Hour

// DoseIndex is an in-memory prefix sum of effective drying hours over the
// recorded room minutes. It answers Dose(from, to) in O(log n) and grows as
// the recorder closes minutes. It is safe for concurrent use.
//
// Missing minutes are handled as in mid-batch-changes.md §5.3: a gap under
// an hour carries the last VPD forward; a longer one (device off, sensor
// down) is filled with the mean VPD of the 24 h before it and counted as
// filled. Time before the first recorded minute is filled with the mean of
// the first day, or DefaultVPD when nothing has been recorded yet.
type DoseIndex struct {
	now func() time.Time

	mu     sync.RWMutex
	minute []int64   // unix seconds, ascending
	rate   []float64 // effective hours per hour during the minute
	vpdCum []float64 // Σ vpd of minutes before i (len n+1), for gap means
	cum    []float64 // effective hours from the first minute to the start of minute i
	fcum   []float64 // the filled part of cum
	gap    []gapFill // the gap after minute i (zero-length if contiguous)
}

type gapFill struct {
	rate   float64
	filled bool
}

// NewDoseIndex returns an empty index. now is used to decide how the time
// since the last recorded minute is treated.
func NewDoseIndex(now func() time.Time) *DoseIndex {
	return &DoseIndex{now: now, vpdCum: []float64{0}}
}

// Load replaces the index with the room minutes stored in the database.
func (d *DoseIndex) Load(ctx context.Context, q store.Querier) error {
	rows, err := store.MinutesBetween(ctx, q, "room", time.Unix(0, 0), time.Unix(1<<40, 0))
	if err != nil {
		return err
	}
	fresh := NewDoseIndex(d.now)
	for _, m := range rows {
		fresh.appendLocked(m.Minute.Unix(), m.VPDAvg)
	}
	d.mu.Lock()
	d.minute, d.rate, d.vpdCum, d.cum, d.fcum, d.gap = fresh.minute, fresh.rate, fresh.vpdCum, fresh.cum, fresh.fcum, fresh.gap
	d.mu.Unlock()
	return nil
}

// Append adds a closed minute. Minutes at or before the last one are
// ignored.
func (d *DoseIndex) Append(minute time.Time, vpd float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.appendLocked(minute.Unix(), vpd)
}

func (d *DoseIndex) appendLocked(m int64, vpd float64) {
	n := len(d.minute)
	if n > 0 && m <= d.minute[n-1] {
		return
	}
	r := predict.EffRate(vpd)
	cum, fcum := 0.0, 0.0
	if n > 0 {
		last := n - 1
		end := d.minute[last] + 60
		g := d.gapAfterLocked(last, m-end)
		d.gap[last] = g
		hours := float64(m-end) / 3600
		cum = d.cum[last] + d.rate[last]/60 + hours*g.rate
		fcum = d.fcum[last]
		if g.filled {
			fcum += hours * g.rate
		}
	}
	d.minute = append(d.minute, m)
	d.rate = append(d.rate, r)
	d.vpdCum = append(d.vpdCum, d.vpdCum[n]+vpd)
	d.cum = append(d.cum, cum)
	d.fcum = append(d.fcum, fcum)
	d.gap = append(d.gap, gapFill{})
}

// gapAfterLocked decides how a gap of length secs after minute i is filled.
func (d *DoseIndex) gapAfterLocked(i int, secs int64) gapFill {
	if secs < int64(fillAfter/time.Second) {
		return gapFill{rate: d.rate[i]}
	}
	return gapFill{rate: predict.EffRate(d.meanVPDLocked(d.minute[i]+60-86400, d.minute[i]+60)), filled: true}
}

// meanVPDLocked is the mean VPD of the recorded minutes in [from, to).
func (d *DoseIndex) meanVPDLocked(from, to int64) float64 {
	i := sort.Search(len(d.minute), func(k int) bool { return d.minute[k] >= from })
	j := sort.Search(len(d.minute), func(k int) bool { return d.minute[k] >= to })
	if j <= i {
		return predict.DefaultVPD
	}
	return (d.vpdCum[j] - d.vpdCum[i]) / float64(j-i)
}

// at returns the cumulative effective hours, and their filled part, at
// unix second t, relative to the first recorded minute.
func (d *DoseIndex) atLocked(t int64) (float64, float64) {
	n := len(d.minute)
	if n == 0 {
		h := float64(t) / 3600 * predict.EffRate(predict.DefaultVPD)
		return h, h
	}
	if t < d.minute[0] {
		r := predict.EffRate(d.meanVPDLocked(d.minute[0], d.minute[0]+86400))
		h := -float64(d.minute[0]-t) / 3600 * r
		return h, h
	}
	i := sort.Search(n, func(k int) bool { return d.minute[k] > t }) - 1
	off := t - d.minute[i]
	if off < 60 {
		return d.cum[i] + float64(off)/3600*d.rate[i], d.fcum[i]
	}
	cum := d.cum[i] + d.rate[i]/60
	fcum := d.fcum[i]
	g := d.gap[i]
	if i == n-1 {
		// The open tail: carry forward until it has lasted an hour, as of now.
		g = d.gapAfterLocked(i, d.now().Unix()-(d.minute[i]+60))
	}
	h := float64(off-60) / 3600 * g.rate
	if g.filled {
		fcum += h
	}
	return cum + h, fcum
}

// Dose returns the effective drying hours between from and to, and how many
// of them were filled in for missing data. It returns zeros if to is not
// after from.
func (d *DoseIndex) Dose(from, to time.Time) (eff, filled float64) {
	if !to.After(from) {
		return 0, 0
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	a, af := d.atLocked(from.Unix())
	b, bf := d.atLocked(to.Unix())
	return max(b-a, 0), max(bf-af, 0)
}
