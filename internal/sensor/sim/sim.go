// Package sim provides a simulated sensor that drifts slowly. It lets the
// app run on a laptop without hardware and doubles as a test fake.
package sim

import (
	"context"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"humidity/internal/sensor"
)

// Sensor produces a slowly drifting fake reading.
type Sensor struct {
	name string
	temp float64
	hum  float64
	pump atomic.Pointer[Pump]
}

// New returns a simulated sensor starting at 20 °C and 60 %RH.
func New(name string) *Sensor {
	return &Sensor{name: name, temp: 20, hum: 60}
}

func (s *Sensor) Name() string { return s.name }

func (s *Sensor) Close() error { return nil }

func (s *Sensor) Read(ctx context.Context) (sensor.Reading, error) {
	s.temp += (rand.Float64() - 0.5) * 0.1
	if p := s.pump.Load(); p != nil {
		// Attached to a pump: smaller noise, slow drying, a rise while it runs.
		s.hum += (rand.Float64()-0.5)*0.2 - airDrying
		if p.On() {
			s.hum += pumpRise
		}
	} else {
		s.hum += (rand.Float64() - 0.5) * 1.0
	}
	s.hum = min(max(s.hum, 0), 100)

	return sensor.Reading{
		Sensor:      s.name,
		Temperature: s.temp,
		Humidity:    s.hum,
		At:          time.Now(),
	}, nil
}
