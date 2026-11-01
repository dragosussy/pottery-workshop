package hw

import (
	"context"
	"math/rand/v2"
	"sync/atomic"
)

// SimPump is a fake pump. Sensors attached to it read damper air while it
// runs and drift slowly drier otherwise, so the controller has something to
// regulate on a laptop.
type SimPump struct{ on atomic.Bool }

func (p *SimPump) Set(on bool) error {
	p.on.Store(on)
	return nil
}

func (p *SimPump) On() bool { return p.on.Load() }

// Per-read humidity changes of an attached sensor (one read per 5 s poll):
// about +5 %/min while pumping, about −7 %/h otherwise.
const (
	simPumpRise = 0.4
	simDrying   = 0.01
)

// SimSensor is a fake sensor starting at 60 %RH and 20 °C. Without a pump it
// wanders randomly. Only the sensor loop reads it, so it has no lock.
type SimSensor struct {
	pump     *SimPump
	rh, temp float64
}

// NewSimSensor returns a sim sensor attached to pump, which may be nil.
func NewSimSensor(pump *SimPump) *SimSensor {
	return &SimSensor{pump: pump, rh: 60, temp: 20}
}

func (s *SimSensor) Read(ctx context.Context) (rh, temp float64, err error) {
	s.temp += (rand.Float64() - 0.5) * 0.1
	if s.pump != nil {
		s.rh += (rand.Float64()-0.5)*0.2 - simDrying
		if s.pump.On() {
			s.rh += simPumpRise
		}
	} else {
		s.rh += (rand.Float64() - 0.5) * 1.0
	}
	s.rh = min(max(s.rh, 0), 100)
	return s.rh, s.temp, nil
}
