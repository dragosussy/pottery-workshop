package control

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"humidity/internal/monitor"
	"humidity/internal/sensor"
)

type fakeSwitch struct {
	on   bool
	fail error
	sets int
}

func (f *fakeSwitch) Name() string { return "pump" }
func (f *fakeSwitch) Set(on bool) error {
	f.sets++
	if f.fail != nil {
		return f.fail
	}
	f.on = on
	return nil
}
func (f *fakeSwitch) Close() error { return f.Set(false) }

type fakeEvents struct{ kinds []string }

func (f *fakeEvents) Log(_ context.Context, kind string, _ any) { f.kinds = append(f.kinds, kind) }
func (f *fakeEvents) Changed()                                  {}

type rig struct {
	t   *testing.T
	c   *Controller
	sw  *fakeSwitch
	ev  *fakeEvents
	now time.Time
}

func newRig(t *testing.T) *rig {
	r := &rig{t: t, sw: &fakeSwitch{}, ev: &fakeEvents{}, now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	r.c = New(r.sw, Config{}, r.ev, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.c.now = func() time.Time { return r.now }
	return r
}

// read feeds one sensor reading at the current time.
func (r *rig) read(rh float64) {
	r.c.Observe(monitor.Cycle{At: r.now, Sensors: []monitor.SensorStatus{{Sensor: "main", OK: true, Reading: sensor.Reading{Humidity: rh}}}})
}

// advance moves the clock in 1 s ticks, feeding rh every 5 s.
func (r *rig) advance(d time.Duration, rh float64) {
	for end := r.now.Add(d); r.now.Before(end); {
		r.now = r.now.Add(time.Second)
		if r.now.Unix()%5 == 0 {
			r.read(rh)
		} else {
			r.c.Tick()
		}
	}
}

func (r *rig) want(on bool, reason string) {
	r.t.Helper()
	st := r.c.State()
	if r.sw.on != on || st.Pump.On != on || st.Pump.Reason != reason {
		r.t.Fatalf("pump on=%v (switch %v) reason=%s, want on=%v reason=%s", st.Pump.On, r.sw.on, st.Pump.Reason, on, reason)
	}
}

func pct(v float64) *float64 { return &v }

func TestPulseRestCycle(t *testing.T) {
	r := newRig(t)
	r.read(70)
	r.want(false, ReasonNoTarget)

	r.c.SetTarget(pct(80))
	r.want(true, ReasonBelowTarget)
	r.advance(9*time.Second, 70)
	r.want(true, ReasonBelowTarget)
	r.advance(time.Second, 70)
	r.want(false, ReasonResting)
	r.advance(3*time.Minute-time.Second, 75)
	r.want(false, ReasonResting)
	r.advance(time.Second, 75)
	r.want(true, ReasonBelowTarget) // still below 78: the next pulse

	r.advance(10*time.Second, 79)
	r.advance(3*time.Minute, 79)
	r.want(false, ReasonAtTarget) // within the 2 % hysteresis
	if got := r.c.State().Pump.Runtime24hSeconds; got != 20 {
		t.Errorf("runtime = %d s, want 20", got)
	}
}

func TestNoPumpingWithoutFreshData(t *testing.T) {
	r := newRig(t)
	r.c.SetTarget(pct(80))
	r.want(false, ReasonNoSensorData)

	r.read(60)
	r.want(true, ReasonBelowTarget)
	// The sensor goes quiet mid-pulse: the pulse stops once data is stale.
	for range 31 {
		r.now = r.now.Add(time.Second)
		r.c.Tick()
	}
	r.want(false, ReasonResting)
	r.now = r.now.Add(3 * time.Minute)
	r.c.Tick()
	r.want(false, ReasonNoSensorData)
}

func TestAboveMaxAndTargetCleared(t *testing.T) {
	r := newRig(t)
	r.read(96)
	r.c.SetTarget(pct(98))
	r.want(false, ReasonAboveMax)
	r.c.SetTarget(nil)
	r.want(false, ReasonNoTarget)
}

func TestTripsWhenHumidityDoesNotRise(t *testing.T) {
	r := newRig(t)
	r.read(60)
	r.c.SetTarget(pct(80))
	for range 6 {
		r.advance(10*time.Second+3*time.Minute, 60)
	}
	r.want(false, ReasonTripped)
	if st := r.c.State(); !st.Safety.Tripped || *st.Safety.Reason != TripNotRising {
		t.Fatalf("safety = %+v", st.Safety)
	}
	if len(r.ev.kinds) != 1 || r.ev.kinds[0] != LogTripped {
		t.Fatalf("log = %v", r.ev.kinds)
	}
	// It stays off until reset.
	r.advance(10*time.Minute, 60)
	r.want(false, ReasonTripped)
	r.c.Reset()
	r.want(true, ReasonBelowTarget)
}

func TestRisingHumidityDoesNotTrip(t *testing.T) {
	r := newRig(t)
	r.read(60)
	r.c.SetTarget(pct(80))
	rh := 60.0
	for range 20 {
		rh += 0.5
		r.advance(10*time.Second+3*time.Minute, rh)
	}
	if r.c.State().Safety.Tripped {
		t.Fatal("tripped while humidity was rising")
	}
}

func TestNoRuntimeCap(t *testing.T) {
	r := newRig(t)
	r.c.cfg.NoRiseAfter = 1 << 30
	r.read(60)
	r.c.SetTarget(pct(80))
	// A week of pulsing below the target: it never stops on its own.
	cycle := 10*time.Second + 3*time.Minute
	for range int(7 * 24 * time.Hour / cycle) {
		r.advance(cycle, 60)
	}
	r.want(true, ReasonBelowTarget)
	if got := r.c.State().Pump.Runtime24hSeconds; got <= 3600 {
		t.Errorf("runtime over the last day = %d s, want more than the old 1 h cap", got)
	}
}

func TestManualAndStop(t *testing.T) {
	r := newRig(t)
	if err := r.c.Manual(2 * time.Minute); !errors.Is(err, ErrManualRange) {
		t.Fatalf("over the cap: %v", err)
	}
	if err := r.c.Manual(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	r.want(true, ReasonManual) // no target, no data: manual runs anyway
	r.advance(5*time.Second, 0)
	r.want(false, ReasonResting)

	if err := r.c.Manual(30 * time.Second); err != nil {
		t.Fatal(err)
	}
	r.c.Stop()
	r.want(false, ReasonResting)
	if got := r.c.State().Pump.Runtime24hSeconds; got != 5 {
		t.Errorf("runtime = %d s, want 5 (the stopped run counts 0 s)", got)
	}
	if len(r.ev.kinds) != 2 || r.ev.kinds[0] != LogManual {
		t.Errorf("log = %v", r.ev.kinds)
	}
}

func TestSwitchFailure(t *testing.T) {
	r := newRig(t)
	r.sw.fail = errors.New("gpio gone")
	r.read(60)
	r.c.SetTarget(pct(80))
	st := r.c.State()
	if st.Pump.On || st.Pump.Error == nil {
		t.Fatalf("state = %+v", st.Pump)
	}
	if err := r.c.Manual(5 * time.Second); !errors.Is(err, ErrSwitch) {
		t.Fatalf("manual: %v", err)
	}
	r.sw.fail = nil
	r.advance(time.Second, 60)
	if st := r.c.State(); !st.Pump.On || st.Pump.Error != nil {
		t.Fatalf("after recovery: %+v", st.Pump)
	}
}

func TestRunSwitchesOffOnShutdown(t *testing.T) {
	r := newRig(t)
	r.read(60)
	r.c.SetTarget(pct(80))
	r.want(true, ReasonBelowTarget)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.c.Run(ctx, make(chan monitor.Cycle)); err != nil {
		t.Fatal(err)
	}
	if r.sw.on {
		t.Fatal("pump still on after shutdown")
	}
}
