package sim

import (
	"context"
	"math"
	"testing"
	"time"

	"humidity/internal/sensor"
)

// Compile-time check that *Sensor satisfies sensor.Sensor.
var _ sensor.Sensor = (*Sensor)(nil)

func mustRead(t *testing.T, s sensor.Sensor) sensor.Reading {
	t.Helper()
	r, err := s.Read(context.Background())
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	return r
}

func TestSensor_NameAndClose(t *testing.T) {
	s := New("rack1")

	if got := s.Name(); got != "rack1" {
		t.Errorf("Name() = %q, want %q", got, "rack1")
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestSensor_ReadFillsReading(t *testing.T) {
	s := New("rack1")

	before := time.Now()
	r := mustRead(t, s)
	after := time.Now()

	if r.Sensor != "rack1" {
		t.Errorf("Reading.Sensor = %q, want %q", r.Sensor, "rack1")
	}
	if r.At.Before(before) || r.At.After(after) {
		t.Errorf("Reading.At = %v, want between %v and %v", r.At, before, after)
	}
}

func TestSensor_StartsNearInitialValues(t *testing.T) {
	s := New("rack1")
	r := mustRead(t, s)

	// One step from the starting point (60 %RH, 20 °C) at most.
	if math.Abs(r.Humidity-60) > 0.5+1e-9 {
		t.Errorf("first humidity = %.3f, want within 0.5 of 60", r.Humidity)
	}
	if math.Abs(r.Temperature-20) > 0.05+1e-9 {
		t.Errorf("first temperature = %.3f, want within 0.05 of 20", r.Temperature)
	}
}

func TestSensor_StepsAreSmall(t *testing.T) {
	const (
		maxHumStep  = 0.5
		maxTempStep = 0.05
		eps         = 1e-9
	)

	s := New("rack1")
	prev := mustRead(t, s)

	for i := 0; i < 10000; i++ {
		cur := mustRead(t, s)

		if d := math.Abs(cur.Humidity - prev.Humidity); d > maxHumStep+eps {
			t.Fatalf("read %d: humidity jumped by %.3f, max %.3f", i, d, maxHumStep)
		}
		if d := math.Abs(cur.Temperature - prev.Temperature); d > maxTempStep+eps {
			t.Fatalf("read %d: temperature jumped by %.3f, max %.3f", i, d, maxTempStep)
		}
		prev = cur
	}
}

func TestSensor_HumidityStaysInRange(t *testing.T) {
	tests := []struct {
		name  string
		start float64
	}{
		{"default start", 60},
		{"pinned at upper bound", 100},
		{"pinned at lower bound", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New("rack1")
			s.hum = tc.start

			for i := 0; i < 100000; i++ {
				r := mustRead(t, s)
				if r.Humidity < 0 || r.Humidity > 100 {
					t.Fatalf("read %d: humidity %.3f out of range [0, 100]", i, r.Humidity)
				}
			}
		})
	}
}

func TestSensor_ReadingsActuallyVary(t *testing.T) {
	s := New("rack1")
	first := mustRead(t, s)

	for i := 0; i < 100; i++ {
		r := mustRead(t, s)
		if r.Humidity != first.Humidity || r.Temperature != first.Temperature {
			return
		}
	}
	t.Error("100 consecutive reads were identical; the simulator isn't drifting")
}
