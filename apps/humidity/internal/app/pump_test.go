package app

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type fakeSwitch struct {
	on   bool
	sets []bool
	fail bool // every Set fails
}

func (s *fakeSwitch) Set(on bool) error {
	s.sets = append(s.sets, on)
	if s.fail {
		return errors.New("gpio broke")
	}
	s.on = on
	return nil
}

// fixture is a pump with a fake clock, a fake switch and one sensor.
type fixture struct {
	t     *testing.T
	clock time.Time
	sw    *fakeSwitch
	live  *Live
	p     *Pump
	rh    float64 // what the sensor reads; NaN for no reading
	rise  float64 // added to rh at each step while the pump is on
}

func newFixture(t *testing.T, target int, rh float64) *fixture {
	f := &fixture{t: t, clock: t0, sw: &fakeSwitch{}, rh: rh}
	f.live = &Live{sensors: []sensorValue{{addr: "0x44"}}, target: target}
	f.p = NewPump(f.sw, f.live, 10*time.Second, 3*time.Minute, 95)
	f.p.now = func() time.Time { return f.clock }
	f.read()
	return f
}

func (f *fixture) read() {
	if !math.IsNaN(f.rh) {
		f.live.record(0, f.rh, 20, f.clock, nil)
	}
}

// advance moves the clock on by d, stepping the controller whenever the
// loop's timer would fire, with a fresh sensor read before each step.
func (f *fixture) advance(d time.Duration) {
	end := f.clock.Add(d)
	for {
		next := f.clock.Add(f.p.wait())
		if next.After(end) {
			f.clock = end
			return
		}
		f.clock = next
		if f.sw.on {
			f.rh += f.rise
		}
		f.read()
		f.p.step()
	}
}

func (f *fixture) want(status, reason string) {
	f.t.Helper()
	s := f.p.Status()
	got := ""
	if s.Reason != nil {
		got = *s.Reason
	}
	if s.Status != status || got != reason {
		f.t.Fatalf("pump is %s (%s), want %s (%s)", s.Status, got, status, reason)
	}
	if s.On != f.sw.on || s.On != (status == "running") {
		f.t.Fatalf("status on = %v, switch on = %v, while %s", s.On, f.sw.on, status)
	}
}

const cycle = 10*time.Second + 3*time.Minute

func TestPulseRestCycle(t *testing.T) {
	f := newFixture(t, 80, 70)
	f.rise = 0.5
	f.p.step()
	f.want("running", "")
	if at := f.p.Status().LastRunAt; at == nil || !at.Equal(t0) {
		t.Errorf("last_run_at = %v, want %v", at, t0)
	}
	f.advance(5 * time.Second)
	f.want("running", "")
	f.advance(5 * time.Second)
	f.want("resting", "")
	f.advance(3*time.Minute - time.Second)
	f.want("resting", "")
	f.advance(time.Second) // still below target: the next pulse
	f.want("running", "")
	if !slices.Equal(f.sw.sets, []bool{true, false, true}) {
		t.Errorf("switch sets = %v", f.sw.sets)
	}
	if f.p.misses != 0 {
		t.Errorf("misses = %d, want 0", f.p.misses)
	}
}

func TestIdleReasons(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target int
		rh     float64
		setup  func(f *fixture)
		reason string
	}{
		{"no target", 0, 70, nil, "no_target"},
		{"no reading", 80, math.NaN(), nil, "no_data"},
		{"stale reading", 80, 70, func(f *fixture) { f.clock = f.clock.Add(31 * time.Second) }, "no_data"},
		{"at target", 80, 80, nil, "at_target"},
		{"above max", 80, 95, nil, "above_max"},
		{"one sensor above max", 90, 70, func(f *fixture) {
			// Room mean 83 is below the target, but the highest is 96.
			f.live.sensors = append(f.live.sensors, sensorValue{addr: "0x45"})
			f.live.record(1, 96, 20, f.clock, nil)
		}, "above_max"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.target, tc.rh)
			if tc.setup != nil {
				tc.setup(f)
			}
			f.p.step()
			f.want("idle", tc.reason)
			if len(f.sw.sets) != 0 {
				t.Errorf("switch sets = %v, want none", f.sw.sets)
			}
			if w := f.p.wait(); w != 5*time.Second {
				t.Errorf("next decision in %v, want 5s", w)
			}
		})
	}
}

func TestTargetChangeLetsPulseFinish(t *testing.T) {
	f := newFixture(t, 80, 70)
	f.p.step()
	f.live.setTarget(0)
	f.advance(9 * time.Second)
	f.want("running", "")
	f.advance(time.Second)
	f.want("resting", "")
	f.advance(3 * time.Minute)
	f.want("idle", "no_target")
}

func TestMissesStopThePump(t *testing.T) {
	f := newFixture(t, 80, 70) // nothing raises the humidity
	f.p.step()
	f.advance(5 * cycle)
	if f.p.misses != 5 {
		t.Fatalf("misses = %d, want 5", f.p.misses)
	}
	f.want("running", "")
	f.advance(cycle)
	f.want("stopped", "no_rise")
	n := len(f.sw.sets)
	f.advance(time.Hour)
	f.want("stopped", "no_rise")
	if len(f.sw.sets) != n {
		t.Errorf("switched while stopped: %v", f.sw.sets[n:])
	}

	// Resume clears the stop and the misses; the next decision follows.
	f.p.handle("resume", 0)
	f.want("running", "")
	if f.p.misses != 0 {
		t.Errorf("misses after resume = %d, want 0", f.p.misses)
	}
}

func TestRiseResetsMisses(t *testing.T) {
	f := newFixture(t, 80, 70)
	f.p.step()
	f.advance(5 * cycle)
	f.rise = 0.2 // two steps while on: +0.4
	f.advance(cycle)
	if f.p.misses != 0 {
		t.Errorf("misses = %d, want 0 after a rise", f.p.misses)
	}
	f.rise = 0.1 // +0.2: too little
	f.advance(cycle)
	if f.p.misses != 1 {
		t.Errorf("misses = %d, want 1", f.p.misses)
	}
}

func TestNoReadingAtEndDoesntCount(t *testing.T) {
	f := newFixture(t, 80, 70)
	f.p.step()
	f.advance(3 * cycle)
	f.rh = math.NaN() // the sensor stops answering
	f.advance(cycle)
	if f.p.misses != 3 {
		t.Errorf("misses = %d, want 3", f.p.misses)
	}
	f.want("idle", "no_data")
}

func TestManualRun(t *testing.T) {
	t.Run("then rests, no miss check", func(t *testing.T) {
		f := newFixture(t, 0, 70)
		f.p.misses = 5
		if err := f.p.handle("run", 5*time.Second); err != nil {
			t.Fatal(err)
		}
		f.want("running", "")
		f.advance(5 * time.Second)
		f.want("resting", "")
		f.advance(3 * time.Minute)
		f.want("idle", "no_target")
		if f.p.misses != 5 {
			t.Errorf("misses = %d, want 5", f.p.misses)
		}
	})
	t.Run("while stopped, then straight back to stopped", func(t *testing.T) {
		f := newFixture(t, 80, 70)
		f.p.handle("stop", 0)
		f.p.handle("run", 5*time.Second)
		f.want("running", "")
		f.advance(5 * time.Second)
		f.want("stopped", "user")
		f.advance(10 * time.Minute)
		f.want("stopped", "user")
	})
	t.Run("without a reading", func(t *testing.T) {
		f := newFixture(t, 80, math.NaN())
		if err := f.p.handle("run", 5*time.Second); err != nil {
			t.Fatal(err)
		}
		f.want("running", "")
	})
	t.Run("refused above max", func(t *testing.T) {
		f := newFixture(t, 80, 95)
		if err := f.p.handle("run", 5*time.Second); err != errAboveMax {
			t.Fatalf("run = %v, want errAboveMax", err)
		}
		f.want("idle", "above_max")
		if len(f.sw.sets) != 0 {
			t.Errorf("switch sets = %v", f.sw.sets)
		}
	})
	t.Run("again while running restarts the timer", func(t *testing.T) {
		f := newFixture(t, 0, 70)
		f.p.handle("run", 5*time.Second)
		f.advance(3 * time.Second)
		f.p.handle("run", 10*time.Second)
		f.advance(9 * time.Second)
		f.want("running", "")
		f.advance(time.Second)
		f.want("resting", "")
	})
}

func TestStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after time.Duration
		was   string
	}{
		{"during a pulse", 5 * time.Second, "running"},
		{"during rest", time.Minute, "resting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, 80, 70)
			f.p.step()
			f.advance(tc.after)
			f.want(tc.was, "")
			f.p.handle("stop", 0)
			f.want("stopped", "user")
			n := len(f.sw.sets)
			f.advance(time.Hour)
			f.want("stopped", "user")
			if len(f.sw.sets) != n {
				t.Errorf("switched while stopped: %v", f.sw.sets[n:])
			}
		})
	}
}

func TestSwitchError(t *testing.T) {
	f := newFixture(t, 80, 70)
	f.sw.fail = true
	f.p.step()
	f.want("stopped", "switch_error")
	if !slices.Equal(f.sw.sets, []bool{true, false}) {
		t.Errorf("switch sets = %v, want an attempt to switch off after the failure", f.sw.sets)
	}
	f.advance(time.Hour)
	f.want("stopped", "switch_error")
}

func TestLoopSwitchesOffOnExit(t *testing.T) {
	f := newFixture(t, 0, 70)
	f.p.after = func(time.Duration) <-chan time.Time { return nil } // only commands wake it
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		f.p.Loop(ctx)
		close(done)
	}()

	if err := f.p.Do(context.Background(), "run", time.Minute); err != nil {
		t.Fatal(err)
	}
	if s := f.p.Status(); !s.On || s.Status != "running" {
		t.Fatalf("after run: %+v", s)
	}
	cancel()
	<-done
	if f.sw.on || f.sw.sets[len(f.sw.sets)-1] {
		t.Errorf("pump left on: sets %v", f.sw.sets)
	}
	if f.p.Status().On {
		t.Error("status still on")
	}
}
