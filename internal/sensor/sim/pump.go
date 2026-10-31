package sim

import (
	"sync/atomic"
)

// Pump is a simulated humidifier pump. Sensors attached to it read damper
// air while it runs, and the air slowly dries while it doesn't, so the
// controller has something to regulate on a laptop.
type Pump struct {
	name string
	on   atomic.Bool
}

// Per-read humidity changes for attached sensors (one read per poll, 5 s by
// default): about +5 %/min while pumping, about −7 %/h otherwise.
const (
	pumpRise  = 0.4
	airDrying = 0.01
)

// NewPump returns a simulated pump, off.
func NewPump(name string) *Pump { return &Pump{name: name} }

func (p *Pump) Name() string { return p.name }

func (p *Pump) Set(on bool) error {
	p.on.Store(on)
	return nil
}

func (p *Pump) Close() error { return p.Set(false) }

// On reports whether the pump is running.
func (p *Pump) On() bool { return p.on.Load() }

// Attach makes the sensor respond to the pump.
func (s *Sensor) Attach(p *Pump) { s.pump.Store(p) }
