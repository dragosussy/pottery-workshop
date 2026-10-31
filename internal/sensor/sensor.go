// Package sensor defines the shared vocabulary for environment sensors.
// Concrete drivers live in sub-packages (sht, sim).
package sensor

import (
	"context"
	"time"
)

// Reading is a single measurement from one sensor.
type Reading struct {
	Sensor      string
	Temperature float64 // °C
	Humidity    float64 // % relative humidity
	At          time.Time
}

// Sensor is anything that can report temperature and humidity.
type Sensor interface {
	Name() string
	Read(ctx context.Context) (Reading, error)
	Close() error
}
