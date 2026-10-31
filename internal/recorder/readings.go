package recorder

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"humidity/internal/store"
)

// Resolutions offered by GET /readings, finest first.
var Resolutions = []struct {
	Name string
	Step time.Duration
}{
	{"1m", time.Minute},
	{"5m", 5 * time.Minute},
	{"15m", 15 * time.Minute},
	{"1h", time.Hour},
	{"6h", 6 * time.Hour},
	{"1d", 24 * time.Hour},
}

// Bucket limits (api-contract §3.4).
const (
	AutoMaxBuckets     = 360
	ExplicitMaxBuckets = 2000
)

// ErrTooManyPoints is returned when an explicit resolution gives more than
// ExplicitMaxBuckets buckets.
var ErrTooManyPoints = errors.New("too many points")

// Readings is the GET /readings body.
type Readings struct {
	From              string         `json:"from"`
	To                string         `json:"to"`
	ResolutionSeconds int64          `json:"resolution_seconds"`
	T                 []int64        `json:"t"`
	Series            []SensorSeries `json:"series"`
}

// SensorSeries is one series, bucketed; empty buckets are null.
type SensorSeries struct {
	Sensor         string     `json:"sensor"`
	HumidityAvg    []*float64 `json:"humidity_avg"`
	HumidityMin    []*float64 `json:"humidity_min"`
	HumidityMax    []*float64 `json:"humidity_max"`
	TemperatureAvg []*float64 `json:"temperature_avg"`
	VPDAvg         []*float64 `json:"vpd_avg"`
	Samples        []int      `json:"samples"`
}

// buckets returns the bucket start times covering [from, to). Steps under
// a day align to multiples of the step in UTC; a day aligns to local
// midnight in loc, so DST days are 23 or 25 hours long.
func buckets(from, to time.Time, step time.Duration, loc *time.Location, limit int) ([]time.Time, error) {
	var out []time.Time
	next := func(t time.Time) time.Time { return t.Add(step) }
	start := from.UTC().Truncate(step)
	if step >= 24*time.Hour {
		l := from.In(loc)
		start = time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
		next = func(t time.Time) time.Time {
			y, m, d := t.In(loc).Date()
			return time.Date(y, m, d+1, 0, 0, 0, 0, loc)
		}
	}
	for t := start; t.Before(to); t = next(t) {
		if len(out) == limit {
			return nil, ErrTooManyPoints
		}
		out = append(out, t)
	}
	return out, nil
}

// PickResolution returns the finest step giving at most AutoMaxBuckets.
func PickResolution(from, to time.Time, loc *time.Location) time.Duration {
	for _, r := range Resolutions {
		if _, err := buckets(from, to, r.Step, loc, AutoMaxBuckets); err == nil {
			return r.Step
		}
	}
	return Resolutions[len(Resolutions)-1].Step
}

// Series aggregates stored minutes into buckets for each series.
func Series(ctx context.Context, q store.Querier, sensors []string, from, to time.Time, step time.Duration, loc *time.Location) (Readings, error) {
	starts, err := buckets(from, to, step, loc, ExplicitMaxBuckets)
	if err != nil {
		return Readings{}, err
	}
	out := Readings{
		From:              from.UTC().Truncate(time.Second).Format(time.RFC3339),
		To:                to.UTC().Truncate(time.Second).Format(time.RFC3339),
		ResolutionSeconds: int64(step / time.Second),
		T:                 make([]int64, len(starts)),
		Series:            []SensorSeries{},
	}
	for i, t := range starts {
		out.T[i] = t.Unix()
	}
	if len(starts) == 0 {
		for _, s := range sensors {
			out.Series = append(out.Series, SensorSeries{Sensor: s, HumidityAvg: []*float64{}, HumidityMin: []*float64{},
				HumidityMax: []*float64{}, TemperatureAvg: []*float64{}, VPDAvg: []*float64{}, Samples: []int{}})
		}
		return out, nil
	}
	for _, s := range sensors {
		minutes, err := store.MinutesBetween(ctx, q, s, starts[0], to)
		if err != nil {
			return Readings{}, err
		}
		out.Series = append(out.Series, aggregate(s, starts, minutes))
	}
	return out, nil
}

func aggregate(sensor string, starts []time.Time, minutes []store.Minute) SensorSeries {
	n := len(starts)
	type acc struct {
		hum, temp, vpd float64
		min, max       float64
		n              int
	}
	accs := make([]acc, n)
	for i := range accs {
		accs[i].min, accs[i].max = math.Inf(1), math.Inf(-1)
	}
	for _, m := range minutes {
		i := sort.Search(n, func(k int) bool { return starts[k].After(m.Minute) }) - 1
		if i < 0 {
			continue
		}
		a := &accs[i]
		w := float64(m.N)
		a.hum += m.HumAvg * w
		a.temp += m.TempAvg * w
		a.vpd += m.VPDAvg * w
		a.min = min(a.min, m.HumMin)
		a.max = max(a.max, m.HumMax)
		a.n += m.N
	}
	s := SensorSeries{
		Sensor:         sensor,
		HumidityAvg:    make([]*float64, n),
		HumidityMin:    make([]*float64, n),
		HumidityMax:    make([]*float64, n),
		TemperatureAvg: make([]*float64, n),
		VPDAvg:         make([]*float64, n),
		Samples:        make([]int, n),
	}
	r := func(x float64, digits float64) *float64 {
		p := math.Pow(10, digits)
		v := math.Round(x*p) / p
		return &v
	}
	for i, a := range accs {
		s.Samples[i] = a.n
		if a.n == 0 {
			continue
		}
		w := float64(a.n)
		s.HumidityAvg[i] = r(a.hum/w, 1)
		s.HumidityMin[i] = r(a.min, 1)
		s.HumidityMax[i] = r(a.max, 1)
		s.TemperatureAvg[i] = r(a.temp/w, 1)
		s.VPDAvg[i] = r(a.vpd/w, 2)
	}
	return s
}
