// Package recorder turns monitor cycles into the stored climate history:
// per-minute aggregates for every sensor and for the room, offline and
// sensor-gap entries in the climate log, and the in-memory dose index the
// predictions run on.
package recorder

import (
	"context"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"humidity/internal/monitor"
	"humidity/internal/predict"
	"humidity/internal/store"
)

// gapAfter is how long the device must have been off, or a sensor down,
// before it is logged (api-contract §3.4).
const gapAfter = time.Hour

// RoomSeries is the series name of the calibrated room mean.
const RoomSeries = "room"

// Notifier is told after the recorder writes to the climate log.
type Notifier interface {
	Changed(scopes []string, pieceIDs []int64)
}

// Recorder aggregates cycles per minute and writes closed minutes.
type Recorder struct {
	store  *store.Store
	index  *DoseIndex
	notify Notifier
	log    *slog.Logger
	synced func() bool
	now    func() time.Time

	clockOK atomic.Bool

	started    bool
	lastStored time.Time // last minute written; nothing at or before it is written again
	open       time.Time // the minute being aggregated
	acc        map[string]*accumulator
	gapStart   map[string]time.Time
	warnedAt   time.Time
}

// New returns a Recorder. synced reports whether the system clock can be
// trusted (ClockSynced in production).
func New(st *store.Store, index *DoseIndex, notify Notifier, synced func() bool, log *slog.Logger) *Recorder {
	return &Recorder{
		store:    st,
		index:    index,
		notify:   notify,
		log:      log,
		synced:   synced,
		now:      time.Now,
		acc:      make(map[string]*accumulator),
		gapStart: make(map[string]time.Time),
	}
}

// ClockSynced reports the clock state as of the last cycle (for /health).
func (r *Recorder) ClockSynced() bool {
	if r.clockOK.Load() {
		return true
	}
	ok := r.synced()
	r.clockOK.Store(ok)
	return ok
}

// Run consumes cycles until ctx is cancelled, then flushes the open minute.
func (r *Recorder) Run(ctx context.Context, cycles <-chan monitor.Cycle) error {
	for {
		select {
		case <-ctx.Done():
			// Flush with a fresh context: ctx is already cancelled.
			fctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			r.flush(fctx)
			cancel()
			r.log.Info("recorder stopped")
			return nil
		case c := <-cycles:
			r.handle(ctx, c)
		}
	}
}

func (r *Recorder) handle(ctx context.Context, c monitor.Cycle) {
	ok := r.synced()
	r.clockOK.Store(ok)
	if !ok {
		if r.now().Sub(r.warnedAt) > 10*time.Minute {
			r.log.Warn("clock not synced yet; holding readings")
			r.warnedAt = r.now()
		}
		return
	}

	if !r.started {
		if err := r.start(ctx, c.At); err != nil {
			r.log.Error("recorder start", "err", err)
			return
		}
	}

	r.trackGaps(ctx, c)

	minute := c.At.UTC().Truncate(time.Minute)
	if !minute.After(r.lastStored) || minute.Before(r.open) {
		return // never write backwards, e.g. after the clock stepped back
	}
	if minute.After(r.open) {
		r.flush(ctx)
		r.open = minute
	}

	for _, s := range c.Sensors {
		if s.OK {
			r.add(s.Sensor, s.Reading.Humidity, s.Reading.Temperature)
		}
	}
	if h, t, ok := c.RoomNow(); ok {
		r.add(RoomSeries, h, t)
	}
}

// start runs once the clock is trusted: it finds the last stored minute and
// logs the time the device was off.
func (r *Recorder) start(ctx context.Context, now time.Time) error {
	last, err := store.LastMinute(ctx, r.store.DB())
	if err != nil {
		return err
	}
	r.started = true
	r.lastStored = last
	if last.IsZero() {
		return nil
	}
	from := last.Add(time.Minute)
	if now.Sub(from) <= gapAfter {
		return nil
	}
	to := now.UTC().Truncate(time.Second)
	value := map[string]string{"from": rfc(from), "to": rfc(to)}
	if _, err := store.InsertLog(ctx, r.store.DB(), store.LogOffline, value, to); err != nil {
		return err
	}
	r.log.Info("device was off", "from", from, "to", to, "for", to.Sub(from).Round(time.Minute))
	r.notify.Changed([]string{"climate_log", "predictions"}, nil)
	return nil
}

// trackGaps logs a sensor_gap when a sensor recovers after failing for more
// than an hour. While it is still down, /status shows failing_since.
func (r *Recorder) trackGaps(ctx context.Context, c monitor.Cycle) {
	for _, s := range c.Sensors {
		start, tracking := r.gapStart[s.Sensor]
		switch {
		case !s.OK && !s.FailingSince.IsZero() && !tracking:
			r.gapStart[s.Sensor] = s.FailingSince
		case s.OK && tracking:
			delete(r.gapStart, s.Sensor)
			if c.At.Sub(start) <= gapAfter {
				continue
			}
			value := map[string]string{"sensor": s.Sensor, "from": rfc(start), "to": rfc(c.At)}
			if _, err := store.InsertLog(ctx, r.store.DB(), store.LogSensorGap, value, c.At); err != nil {
				r.log.Error("log sensor gap", "err", err)
				continue
			}
			r.notify.Changed([]string{"climate_log", "predictions"}, nil)
		}
	}
}

func (r *Recorder) add(series string, h, t float64) {
	a := r.acc[series]
	if a == nil {
		a = &accumulator{min: math.Inf(1), max: math.Inf(-1)}
		r.acc[series] = a
	}
	a.hum += h
	a.temp += t
	a.vpd += predict.VPD(t, h)
	a.min = min(a.min, h)
	a.max = max(a.max, h)
	a.n++
}

// flush writes the open minute, if it has data.
func (r *Recorder) flush(ctx context.Context) {
	if r.open.IsZero() || len(r.acc) == 0 {
		return
	}
	rows := make([]store.Minute, 0, len(r.acc))
	for series, a := range r.acc {
		n := float64(a.n)
		rows = append(rows, store.Minute{
			Sensor: series, Minute: r.open, N: a.n,
			HumAvg: a.hum / n, HumMin: a.min, HumMax: a.max, TempAvg: a.temp / n, VPDAvg: a.vpd / n,
		})
	}
	err := r.store.Tx(ctx, func(q store.Querier) error { return store.InsertMinutes(ctx, q, rows) })
	room := r.acc[RoomSeries]
	clear(r.acc)
	if err != nil {
		r.log.Error("write minute", "minute", r.open, "err", err)
		return
	}
	if room != nil {
		r.index.Append(r.open, room.vpd/float64(room.n))
	}
	r.lastStored = r.open
}

type accumulator struct {
	hum, temp, vpd float64
	min, max       float64
	n              int
}

func rfc(t time.Time) string { return t.UTC().Truncate(time.Second).Format(time.RFC3339) }
