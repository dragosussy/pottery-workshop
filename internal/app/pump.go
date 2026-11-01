package app

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"humidity/internal/hw"
)

// See docs/plan.md §4.
const (
	decideEvery = 5 * time.Second
	minRise     = 0.3 // %RH a pulse must raise the room by to count as working
	maxMisses   = 6
)

var errAboveMax = errors.New("humidity is at or above the maximum")

// PumpStatus is the pump part of Now.
type PumpStatus struct {
	Enabled   bool       `json:"enabled"`
	On        bool       `json:"on"`
	Status    string     `json:"status"` // running | resting | idle | stopped
	Reason    *string    `json:"reason"`
	LastRunAt *time.Time `json:"last_run_at"`
}

// Pump is the controller. One goroutine (Loop) owns its state; HTTP
// handlers send it commands with Do and read Status.
type Pump struct {
	sw          hw.Switch
	live        *Live
	pulse, rest time.Duration
	maxRH       float64
	now         func() time.Time
	after       func(time.Duration) <-chan time.Time
	cmds        chan pumpCmd

	// Owned by the loop.
	phase   string    // idle | running | resting
	until   time.Time // end of the current run or rest
	manual  bool      // the current run was started by hand: no miss check
	stopped string    // why it is stopped; "" when it isn't
	idleWhy string
	on      bool
	startRH float64 // room RH when the automatic pulse started
	peakRH  float64 // highest room RH since then
	misses  int
	lastRun time.Time

	mu     sync.Mutex
	status PumpStatus // published copy of the above
}

type pumpCmd struct {
	kind  string // run | stop | resume | decide
	d     time.Duration
	reply chan error
}

func NewPump(sw hw.Switch, live *Live, pulse, rest time.Duration, maxRH int) *Pump {
	p := &Pump{
		sw: sw, live: live, pulse: pulse, rest: rest, maxRH: float64(maxRH),
		now: time.Now, after: time.After, cmds: make(chan pumpCmd),
		phase: "idle", idleWhy: "no_data",
	}
	p.publish()
	return p
}

// Loop runs the controller until ctx ends, then switches the pump off.
func (p *Pump) Loop(ctx context.Context) {
	defer func() {
		p.set(false)
		p.publish()
	}()
	p.step()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.after(p.wait()):
			p.step()
		case c := <-p.cmds:
			c.reply <- p.handle(c.kind, c.d)
		}
	}
}

// Do sends a command to the loop and waits until it has been applied.
// kind is "run" (for d), "stop", "resume", or "decide" (re-decide now,
// after a target change; a run in progress finishes).
func (p *Pump) Do(ctx context.Context, kind string, d time.Duration) error {
	c := pumpCmd{kind: kind, d: d, reply: make(chan error, 1)}
	select {
	case p.cmds <- c:
		return <-c.reply
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pump) Status() PumpStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// wait is how long the loop sleeps before the next step: until the current
// run or rest ends, and at most 5 s.
func (p *Pump) wait() time.Duration {
	if p.phase == "idle" {
		return decideEvery
	}
	return min(decideEvery, p.until.Sub(p.now()))
}

// step ends a run or rest that is due, keeps track of the highest RH during
// an automatic pulse and its rest, and decides when idle.
func (p *Pump) step() {
	defer p.publish()
	now := p.now()
	room, fresh := p.live.Room(now)
	if fresh && p.phase != "idle" {
		p.peakRH = max(p.peakRH, room.RH)
	}

	switch {
	case p.phase == "running" && !now.Before(p.until):
		if !p.set(false) {
			return
		}
		if p.manual && p.stopped != "" {
			p.phase = "idle" // a test run while stopped: straight back to stopped
		} else {
			p.phase, p.until = "resting", now.Add(p.rest)
		}
	case p.phase == "resting" && !now.Before(p.until):
		p.phase = "idle"
		if !p.manual && fresh {
			p.checkRise()
		}
	}

	if p.phase == "idle" && p.stopped == "" {
		p.decide(now, room, fresh)
	}
}

func (p *Pump) decide(now time.Time, room Room, fresh bool) {
	target := p.live.Target()
	switch {
	case target == 0:
		p.idleWhy = "no_target"
	case !fresh:
		p.idleWhy = "no_data"
	case room.Max >= p.maxRH:
		p.idleWhy = "above_max"
	case room.RH >= float64(target):
		p.idleWhy = "at_target"
	default:
		if p.set(true) {
			p.phase, p.until, p.manual, p.lastRun = "running", now.Add(p.pulse), false, now
			p.startRH, p.peakRH = room.RH, room.RH
		}
	}
}

// checkRise runs after an automatic pulse and its rest: too many pulses in
// a row that didn't raise the humidity mean an empty tank or a blocked
// nozzle.
func (p *Pump) checkRise() {
	if p.peakRH-p.startRH < minRise {
		p.misses++
	} else {
		p.misses = 0
	}
	if p.misses >= maxMisses {
		log.Printf("pump: %d pulses without a rise, stopping", p.misses)
		p.stopped = "no_rise"
	}
}

// handle applies a command, then steps so the reply reflects its outcome.
func (p *Pump) handle(kind string, d time.Duration) error {
	now := p.now()
	var err error
	switch kind {
	case "run":
		if room, ok := p.live.Room(now); ok && room.Max >= p.maxRH {
			err = errAboveMax
			break
		}
		if p.set(true) {
			p.phase, p.until, p.manual, p.lastRun = "running", now.Add(d), true, now
		}
	case "stop":
		p.stopped, p.phase = "user", "idle"
		p.set(false)
	case "resume":
		p.stopped, p.misses = "", 0
	}
	p.step()
	return err
}

// set switches the pump. A failure stops the controller with
// switch_error, after one more attempt to switch off.
func (p *Pump) set(on bool) bool {
	err := p.sw.Set(on)
	if err == nil {
		p.on = on
		return true
	}
	log.Printf("pump: switching on=%v: %v", on, err)
	if err := p.sw.Set(false); err != nil {
		log.Printf("pump: switching off: %v", err)
	}
	p.on, p.phase, p.stopped = false, "idle", "switch_error"
	return false
}

func (p *Pump) publish() {
	s := PumpStatus{Enabled: true, On: p.on}
	var reason string
	switch {
	case p.phase == "running":
		s.Status = "running"
	case p.stopped != "":
		s.Status, reason = "stopped", p.stopped
	case p.phase == "resting":
		s.Status = "resting"
	default:
		s.Status, reason = "idle", p.idleWhy
	}
	if reason != "" {
		s.Reason = &reason
	}
	if !p.lastRun.IsZero() {
		t := p.lastRun.UTC().Truncate(time.Second)
		s.LastRunAt = &t
	}
	p.mu.Lock()
	p.status = s
	p.mu.Unlock()
}
