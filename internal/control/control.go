// Package control holds the shelf at the humidity target with the pump.
//
// The shelf has no fan, so mist takes a while to reach the sensor. The
// controller therefore works in pulses: when the shelf is below the target
// by more than the hysteresis, it runs the pump for one short pulse, then
// rests (pump off) long enough for the air to settle before deciding again.
// The pump can only add moisture; a shelf above the target dries on its own
// or with the plastic opened.
//
// Safety, in order of precedence: the pump is off without fresh sensor
// data, above MaxRH, and after a trip. There is no runtime cap: holding a
// damp shelf can take a lot of pumping, for days. It trips
// when NoRiseAfter pulses in a row didn't raise the humidity (usually an
// empty tank or a blocked nozzle) and stays off until Reset.
package control

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"humidity/internal/actuator"
	"humidity/internal/monitor"
)

// Config tunes the controller. Zero values take the defaults.
type Config struct {
	Pulse       time.Duration // one pump run, default 10s
	Rest        time.Duration // pause after a pulse, default 3m
	Hysteresis  float64       // pulse when below target − this, default 2 %
	MaxRH       float64       // never pump at or above this, default 95 %
	NoRiseAfter int           // trip after this many ineffective pulses in a row, default 6
	MinRise     float64       // a pulse is effective if humidity rose this much by the end of its rest, default 0.3 %
	MaxManual   time.Duration // longest manual run, default 60s
	StaleAfter  time.Duration // sensor data older than this counts as missing, default 30s
}

func (c Config) withDefaults() Config {
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&c.Pulse, 10*time.Second)
	def(&c.Rest, 3*time.Minute)
	def(&c.MaxManual, time.Minute)
	def(&c.StaleAfter, 30*time.Second)
	if c.Hysteresis <= 0 {
		c.Hysteresis = 2
	}
	if c.MaxRH <= 0 {
		c.MaxRH = 95
	}
	if c.NoRiseAfter <= 0 {
		c.NoRiseAfter = 6
	}
	if c.MinRise <= 0 {
		c.MinRise = 0.3
	}
	return c
}

// Climate-log kinds the controller writes.
const (
	LogTripped = "pump_tripped" // {reason}
	LogReset   = "pump_reset"   // {}
	LogManual  = "pump_manual"  // {seconds}
)

// Reasons for the pump's state (State.Pump.Reason).
const (
	ReasonBelowTarget  = "below_target"   // pulsing
	ReasonManual       = "manual"         // a manual run
	ReasonResting      = "resting"        // waiting for the air to settle
	ReasonAtTarget     = "at_target"      // within the hysteresis, or above
	ReasonNoTarget     = "no_target"      // no target set
	ReasonNoSensorData = "no_sensor_data" // no fresh reading
	ReasonAboveMax     = "above_max"      // at or above MaxRH
	ReasonTripped      = "tripped"        // see State.Safety
)

// Errors from Manual.
var (
	ErrManualRange = errors.New("control: manual run must be between 1 s and the maximum")
	ErrSwitch      = errors.New("control: the pump switch failed")
)

// Trip reasons.
const TripNotRising = "not_rising"

// Events receives what the controller wants recorded and announced.
type Events interface {
	// Log appends a climate-log entry.
	Log(ctx context.Context, kind string, value any)
	// Changed announces a state change to live clients.
	Changed()
}

type phase int

const (
	idle phase = iota
	pulsing
	resting
)

type pulse struct {
	start time.Time
	dur   time.Duration
}

// Controller runs the pump. Its methods are safe for concurrent use.
type Controller struct {
	sw     actuator.Switch
	cfg    Config
	events Events
	log    *slog.Logger
	now    func() time.Time

	mu        sync.Mutex
	target    *float64
	rh        float64
	rhAt      time.Time
	hasRH     bool
	on        bool
	since     time.Time
	phase     phase
	until     time.Time
	manual    bool
	reason    string
	startRH   float64 // humidity when the current automatic pulse started
	noRise    int
	pulses    []pulse // the last 24 h
	trip      string
	trippedAt time.Time
	swErr     string
}

// New returns a controller for sw, with the pump off and no target.
func New(sw actuator.Switch, cfg Config, events Events, log *slog.Logger) *Controller {
	return &Controller{sw: sw, cfg: cfg.withDefaults(), events: events, log: log, now: time.Now, reason: ReasonNoTarget}
}

// SetTarget changes the target (nil: none). It takes effect at the next
// decision, not mid-pulse.
func (c *Controller) SetTarget(pct *float64) {
	c.mu.Lock()
	if pct != nil {
		v := *pct
		pct = &v
	}
	c.target = pct
	c.mu.Unlock()
	c.Tick()
}

// Run follows the monitor's cycles and a 1 s clock until ctx is done, then
// switches the pump off.
func (c *Controller) Run(ctx context.Context, cycles <-chan monitor.Cycle) error {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	defer c.off()
	for {
		select {
		case <-ctx.Done():
			return nil
		case cy := <-cycles:
			c.Observe(cy)
		case <-t.C:
			c.Tick()
		}
	}
}

// Observe takes the room humidity (mean of the sensors read in this cycle)
// and decides.
func (c *Controller) Observe(cy monitor.Cycle) {
	sum, n := 0.0, 0
	for _, s := range cy.Sensors {
		if s.OK {
			sum += s.Reading.Humidity
			n++
		}
	}
	c.mu.Lock()
	if n > 0 {
		c.rh, c.rhAt, c.hasRH = sum/float64(n), cy.At, true
	}
	c.mu.Unlock()
	c.Tick()
}

// Tick makes a decision at the current time.
func (c *Controller) Tick() {
	c.mu.Lock()
	changed, logs := c.step(c.now())
	c.mu.Unlock()
	c.emit(changed, logs)
}

type logEntry struct {
	kind  string
	value any
}

func (c *Controller) emit(changed bool, logs []logEntry) {
	if c.events == nil {
		return
	}
	for _, l := range logs {
		c.events.Log(context.Background(), l.kind, l.value)
	}
	if changed || len(logs) > 0 {
		c.events.Changed()
	}
}

// step advances the state machine. It reports whether the visible state
// changed, and the log entries to write (outside the lock).
func (c *Controller) step(now time.Time) (changed bool, logs []logEntry) {
	prevOn, prevReason := c.on, c.reason
	defer func() { changed = changed || c.on != prevOn || c.reason != prevReason }()

	// A failed switch-off is retried every tick.
	if !c.on && c.swErr != "" {
		c.set(false, now)
	}

	switch c.phase {
	case pulsing:
		stop := !now.Before(c.until)
		if !c.manual {
			stop = stop || c.trip != "" || !c.fresh(now) || c.rh >= c.cfg.MaxRH
		}
		if !stop {
			return
		}
		c.endPulse(now)
		return
	case resting:
		if now.Before(c.until) {
			return
		}
		c.phase = idle
		if !c.manual && c.target != nil && c.fresh(now) {
			if c.rh-c.startRH < c.cfg.MinRise && c.rh < *c.target-c.cfg.Hysteresis {
				c.noRise++
			} else {
				c.noRise = 0
			}
			if c.noRise >= c.cfg.NoRiseAfter && c.trip == "" {
				c.trip, c.trippedAt = TripNotRising, now
				c.log.Warn("pump tripped: humidity not rising", "pulses", c.noRise)
				logs = append(logs, logEntry{LogTripped, map[string]any{"reason": TripNotRising, "pulses": c.noRise}})
			}
		}
		c.manual = false
	}

	c.reason = c.decide(now)
	if c.reason == ReasonBelowTarget {
		c.startRH = c.rh
		c.startPulse(now, c.cfg.Pulse, false)
	}
	return
}

func (c *Controller) decide(now time.Time) string {
	switch {
	case c.trip != "":
		return ReasonTripped
	case c.target == nil:
		return ReasonNoTarget
	case !c.fresh(now):
		return ReasonNoSensorData
	case c.rh >= c.cfg.MaxRH:
		return ReasonAboveMax
	case c.rh >= *c.target-c.cfg.Hysteresis:
		return ReasonAtTarget
	}
	return ReasonBelowTarget
}

func (c *Controller) fresh(now time.Time) bool {
	return c.hasRH && now.Sub(c.rhAt) <= c.cfg.StaleAfter
}

func (c *Controller) startPulse(now time.Time, d time.Duration, manual bool) {
	if !c.set(true, now) {
		c.reason = "switch_error"
		return
	}
	c.phase, c.until, c.manual = pulsing, now.Add(d), manual
	c.pulses = append(c.pulses, pulse{start: now, dur: d})
	if manual {
		c.reason = ReasonManual
	} else {
		c.reason = ReasonBelowTarget
	}
}

// endPulse switches off, trims the pulse to what actually ran, and rests.
func (c *Controller) endPulse(now time.Time) {
	c.set(false, now)
	if n := len(c.pulses); n > 0 {
		last := &c.pulses[n-1]
		last.dur = min(last.dur, now.Sub(last.start))
	}
	c.phase, c.until = resting, now.Add(c.cfg.Rest)
	c.reason = ReasonResting
}

// set drives the switch, recording the error if it fails. For "off" the
// state is off even if the write failed (it is retried every tick).
func (c *Controller) set(on bool, now time.Time) bool {
	err := c.sw.Set(on)
	if err != nil {
		if c.swErr == "" {
			c.log.Error("pump switch", "on", on, "err", err)
		}
		c.swErr = err.Error()
		if on {
			return false
		}
	} else {
		c.swErr = ""
	}
	if c.on != on {
		c.since = now
	}
	c.on = on
	return err == nil
}

// off switches the pump off (shutdown).
func (c *Controller) off() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.set(false, c.now())
	c.phase = idle
}

// runtime is the pump time within the last 24 h; it also drops older pulses.
func (c *Controller) runtime(now time.Time) time.Duration {
	from := now.Add(-24 * time.Hour)
	keep := c.pulses[:0]
	var total time.Duration
	for _, p := range c.pulses {
		end := minTime(p.start.Add(p.dur), now) // a running pulse counts so far
		if !end.After(from) {
			continue
		}
		keep = append(keep, p)
		total += end.Sub(maxTime(p.start, from))
	}
	c.pulses = keep
	return total
}

// Manual runs the pump for d now, whatever the target, the humidity or a
// trip: it is for testing the wiring and priming after a refill. d is
// capped at MaxManual.
func (c *Controller) Manual(d time.Duration) error {
	if d <= 0 || d > c.cfg.MaxManual {
		return ErrManualRange
	}
	c.mu.Lock()
	now := c.now()
	c.startPulse(now, d, true)
	ok := c.on
	c.mu.Unlock()
	if !ok {
		c.emit(true, nil)
		return ErrSwitch
	}
	c.emit(true, []logEntry{{LogManual, map[string]any{"seconds": d.Seconds()}}})
	return nil
}

// Stop switches the pump off now; the controller then rests before deciding
// again.
func (c *Controller) Stop() {
	c.mu.Lock()
	now := c.now()
	if c.phase == pulsing {
		c.endPulse(now)
	}
	c.mu.Unlock()
	c.emit(true, nil)
}

// Reset clears a trip.
func (c *Controller) Reset() {
	c.mu.Lock()
	was := c.trip
	c.trip, c.noRise = "", 0
	c.mu.Unlock()
	if was != "" {
		c.emit(true, []logEntry{{LogReset, map[string]any{"reason": was}}})
	}
	c.Tick()
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

// State is the controller's visible state (the pump part of GET /control).
type State struct {
	Pump   PumpState   `json:"pump"`
	Safety SafetyState `json:"safety"`
	Limits Limits      `json:"limits"`
}

// PumpState is the pump now.
type PumpState struct {
	Name              string  `json:"name"`
	On                bool    `json:"on"`
	Since             *string `json:"since"`
	Reason            string  `json:"reason"`
	RestUntil         *string `json:"rest_until"`
	Runtime24hSeconds int     `json:"runtime_24h_seconds"`
	LastRunAt         *string `json:"last_run_at"`
	Error             *string `json:"error"`
}

// SafetyState says whether the controller tripped, and why.
type SafetyState struct {
	Tripped bool    `json:"tripped"`
	Reason  *string `json:"reason"`
	Since   *string `json:"since"`
}

// Limits are the configured bounds, for display.
type Limits struct {
	PulseSeconds     int     `json:"pulse_seconds"`
	RestSeconds      int     `json:"rest_seconds"`
	HysteresisPct    float64 `json:"hysteresis_pct"`
	MaxRHPct         float64 `json:"max_rh_pct"`
	MaxManualSeconds int     `json:"max_manual_seconds"`
}

// State returns the current state.
func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	st := State{
		Pump: PumpState{
			Name: c.sw.Name(), On: c.on, Reason: c.reason,
			Runtime24hSeconds: int(c.runtime(now).Round(time.Second) / time.Second),
		},
		Limits: Limits{
			PulseSeconds: int(c.cfg.Pulse / time.Second), RestSeconds: int(c.cfg.Rest / time.Second),
			HysteresisPct: c.cfg.Hysteresis, MaxRHPct: c.cfg.MaxRH,
			MaxManualSeconds: int(c.cfg.MaxManual / time.Second),
		},
	}
	if !c.since.IsZero() {
		st.Pump.Since = ts(c.since)
	}
	if c.phase == resting {
		st.Pump.RestUntil = ts(c.until)
	}
	if n := len(c.pulses); n > 0 {
		st.Pump.LastRunAt = ts(c.pulses[n-1].start)
	}
	if c.swErr != "" {
		e := c.swErr
		st.Pump.Error = &e
	}
	if c.trip != "" {
		r := c.trip
		st.Safety = SafetyState{Tripped: true, Reason: &r, Since: ts(c.trippedAt)}
	}
	return st
}

func ts(t time.Time) *string {
	s := t.UTC().Truncate(time.Second).Format(time.RFC3339)
	return &s
}
