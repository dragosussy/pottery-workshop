package monitor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"humidity/internal/sensor"
	"humidity/internal/sensor/sim"
)

// failing is a sensor whose reads always fail.
type failing struct{ name string }

func (f failing) Name() string { return f.name }
func (failing) Close() error   { return nil }
func (failing) Read(context.Context) (sensor.Reading, error) {
	return sensor.Reading{}, errors.New("boom")
}

// flaky succeeds until fail is set, then fails.
type flaky struct {
	name string
	fail bool
}

func (f *flaky) Name() string { return f.name }
func (f *flaky) Close() error { return nil }
func (f *flaky) Read(context.Context) (sensor.Reading, error) {
	if f.fail {
		return sensor.Reading{}, errors.New("boom")
	}
	return sensor.Reading{Sensor: f.name, Humidity: 55, At: time.Now()}, nil
}

func newTestMonitor(sensors ...sensor.Sensor) *Monitor {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(sensors, time.Hour, log)
}

func TestPoll_StoresLatestReadingsSortedByName(t *testing.T) {
	m := newTestMonitor(sim.New("b"), sim.New("a"))

	m.poll(context.Background())

	got := m.Latest()
	if len(got) != 2 || got[0].Sensor != "a" || got[1].Sensor != "b" {
		t.Fatalf("Latest() = %+v, want readings for a then b", got)
	}
}

func TestPoll_FailingSensorDoesNotBlockOthers(t *testing.T) {
	m := newTestMonitor(failing{"bad"}, sim.New("good"))

	m.poll(context.Background())

	got := m.Latest()
	if len(got) != 1 || got[0].Sensor != "good" {
		t.Fatalf("Latest() = %+v, want only the reading for good", got)
	}
}

func TestPoll_KeepsLastGoodReadingWhenSensorStartsFailing(t *testing.T) {
	f := &flaky{name: "rack1"}
	m := newTestMonitor(f)

	m.poll(context.Background())
	f.fail = true
	m.poll(context.Background())

	got := m.Latest()
	if len(got) != 1 || got[0].Humidity != 55 {
		t.Fatalf("Latest() = %+v, want the last good reading (55%%) to be kept", got)
	}
}

func TestRun_StopsWhenContextCancelled(t *testing.T) {
	m := newTestMonitor(sim.New("a"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()

	// Run polls once immediately; wait for that reading to appear.
	deadline := time.Now().Add(2 * time.Second)
	for len(m.Latest()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no reading appeared after Run started")
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() = %v, want nil on clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context was cancelled")
	}
}

func TestPoll_AppliesOffsetsAndClamps(t *testing.T) {
	f := &flaky{name: "main"}
	m := newTestMonitor(f)
	m.SetOffsets(map[string]float64{"main": 1.5})

	m.poll(context.Background())
	if got := m.Latest()[0].Humidity; got != 56.5 {
		t.Errorf("humidity = %v, want 56.5 (55 + 1.5)", got)
	}

	m.SetOffsets(map[string]float64{"main": 50})
	m.poll(context.Background())
	if got := m.Latest()[0].Humidity; got != 100 {
		t.Errorf("humidity = %v, want clamped to 100", got)
	}
}

func TestPoll_TracksHealth(t *testing.T) {
	f := &flaky{name: "main"}
	m := newTestMonitor(f)
	t0 := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	now := t0
	m.now = func() time.Time { return now }

	m.poll(context.Background())
	f.fail = true
	now = t0.Add(5 * time.Second)
	m.poll(context.Background())
	now = t0.Add(10 * time.Second)
	m.poll(context.Background())

	st := m.Last().Sensors[0]
	if st.OK || st.LastError != "boom" || !st.FailingSince.Equal(t0.Add(5*time.Second)) {
		t.Errorf("status = %+v, want failing since the first failure with its error", st)
	}
	if st.Reading.At.IsZero() || st.Reading.Humidity != 55 {
		t.Errorf("status lost the last good reading: %+v", st.Reading)
	}

	f.fail = false
	m.poll(context.Background())
	st = m.Last().Sensors[0]
	if !st.OK || st.LastError != "" || !st.FailingSince.IsZero() {
		t.Errorf("status after recovery = %+v, want ok", st)
	}
}

func TestSubscribe_ReceivesCyclesWithoutBlocking(t *testing.T) {
	m := newTestMonitor(sim.New("a"), failing{"b"})
	ch, cancel := m.Subscribe(1)
	defer cancel()

	m.poll(context.Background())
	m.poll(context.Background()) // buffer full: dropped, must not block

	c := <-ch
	if len(c.Sensors) != 2 || !c.Sensors[0].OK || c.Sensors[1].OK {
		t.Fatalf("cycle = %+v, want a ok and b failing", c)
	}
	if _, _, ok := c.RoomNow(); !ok {
		t.Error("RoomNow() reported no data with one ok sensor")
	}
	select {
	case <-ch:
		t.Error("second cycle should have been dropped")
	default:
	}
}
