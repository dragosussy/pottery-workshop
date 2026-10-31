// Package monitor polls sensors on an interval, applies calibration offsets,
// tracks each sensor's health and publishes a snapshot after every poll
// cycle to its subscribers (the recorder and the live stream).
package monitor

import (
	"cmp"
	"context"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"humidity/internal/sensor"
)

// SensorStatus is the state of one sensor after a poll cycle.
type SensorStatus struct {
	Sensor string
	// OK reports whether this cycle's read succeeded.
	OK bool
	// Reading is the latest good reading, offset applied. Reading.At is zero
	// if the sensor has never been read successfully.
	Reading      sensor.Reading
	Offset       float64 // RH offset applied, in %
	LastError    string  // "" if the last read succeeded
	FailingSince time.Time
}

// Cycle is a snapshot of every sensor after one poll cycle, sorted by name.
type Cycle struct {
	At      time.Time
	Sensors []SensorStatus
}

// Monitor polls a fixed set of sensors. The caller owns the sensors and is
// responsible for closing them.
type Monitor struct {
	sensors  []sensor.Sensor
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time

	mu      sync.RWMutex
	status  map[string]*SensorStatus
	offsets map[string]float64
	last    Cycle
	subs    map[chan Cycle]struct{}
}

// New creates a Monitor. interval must be positive.
func New(sensors []sensor.Sensor, interval time.Duration, log *slog.Logger) *Monitor {
	m := &Monitor{
		sensors:  sensors,
		interval: interval,
		log:      log,
		now:      time.Now,
		status:   make(map[string]*SensorStatus, len(sensors)),
		offsets:  make(map[string]float64),
		subs:     make(map[chan Cycle]struct{}),
	}
	for _, s := range sensors {
		m.status[s.Name()] = &SensorStatus{Sensor: s.Name()}
	}
	return m
}

// Interval returns the poll interval.
func (m *Monitor) Interval() time.Duration { return m.interval }

// Run polls immediately, then once per interval, until ctx is cancelled.
// It returns nil on a clean shutdown.
func (m *Monitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		m.poll(ctx)

		select {
		case <-ctx.Done():
			m.log.Info("monitor stopped")
			return nil
		case <-ticker.C:
		}
	}
}

// SetOffsets replaces the RH calibration offsets (sensor name → %). They
// apply to readings taken from now on.
func (m *Monitor) SetOffsets(offsets map[string]float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offsets = make(map[string]float64, len(offsets))
	for k, v := range offsets {
		m.offsets[k] = v
	}
}

// Subscribe returns a channel that receives every cycle from now on, and a
// function that cancels the subscription. Sends never block: a subscriber
// that falls behind by more than buf cycles misses some.
func (m *Monitor) Subscribe(buf int) (<-chan Cycle, func()) {
	ch := make(chan Cycle, buf)
	m.mu.Lock()
	m.subs[ch] = struct{}{}
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		delete(m.subs, ch)
		m.mu.Unlock()
	}
}

// Last returns the most recent cycle. Before the first poll it lists every
// sensor with no reading.
func (m *Monitor) Last() Cycle {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.last.At.IsZero() {
		return m.snapshotLocked(time.Time{})
	}
	return m.last
}

// Latest returns the most recent successful reading of each sensor, sorted by
// sensor name. A sensor that has never been read successfully is absent, and
// a sensor that has started failing keeps its last good reading, so consumers
// should check Reading.At to detect stale data.
func (m *Monitor) Latest() []sensor.Reading {
	c := m.Last()
	out := make([]sensor.Reading, 0, len(c.Sensors))
	for _, s := range c.Sensors {
		if !s.Reading.At.IsZero() {
			out = append(out, s.Reading)
		}
	}
	return out
}

// poll reads every sensor once and publishes the cycle. A failing sensor is
// logged and skipped: one flaky wire must not take the whole process down.
func (m *Monitor) poll(ctx context.Context) {
	type result struct {
		r   sensor.Reading
		err error
	}
	results := make(map[string]result, len(m.sensors))
	for _, s := range m.sensors {
		r, err := s.Read(ctx)
		if err != nil && ctx.Err() != nil {
			return // shutting down, not a sensor fault
		}
		results[s.Name()] = result{r, err}
	}

	m.mu.Lock()
	now := m.now()
	for name, res := range results {
		st := m.status[name]
		if res.err != nil {
			if st.FailingSince.IsZero() {
				st.FailingSince = now
				m.log.Warn("sensor failing", "sensor", name, "err", res.err)
			}
			st.OK = false
			st.LastError = res.err.Error()
			continue
		}
		if !st.FailingSince.IsZero() {
			m.log.Info("sensor recovered", "sensor", name, "failed_for", now.Sub(st.FailingSince).Round(time.Second))
		}
		r := res.r
		st.Offset = m.offsets[name]
		r.Humidity = min(max(r.Humidity+st.Offset, 0), 100)
		st.OK, st.Reading, st.LastError, st.FailingSince = true, r, "", time.Time{}

		m.log.Debug("reading",
			"sensor", r.Sensor,
			"humidity_pct", round1(r.Humidity),
			"temp_c", round1(r.Temperature),
		)
	}
	c := m.snapshotLocked(now)
	m.last = c
	for ch := range m.subs {
		select {
		case ch <- c:
		default:
		}
	}
	m.mu.Unlock()
}

func (m *Monitor) snapshotLocked(at time.Time) Cycle {
	c := Cycle{At: at, Sensors: make([]SensorStatus, 0, len(m.status))}
	for _, st := range m.status {
		c.Sensors = append(c.Sensors, *st)
	}
	slices.SortFunc(c.Sensors, func(a, b SensorStatus) int { return cmp.Compare(a.Sensor, b.Sensor) })
	return c
}

// RoomNow returns the mean humidity and temperature of the sensors read
// successfully in this cycle, and false if there were none.
func (c Cycle) RoomNow() (humidity, temperature float64, ok bool) {
	n := 0
	for _, s := range c.Sensors {
		if s.OK {
			humidity += s.Reading.Humidity
			temperature += s.Reading.Temperature
			n++
		}
	}
	if n == 0 {
		return 0, 0, false
	}
	return humidity / float64(n), temperature / float64(n), true
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }
