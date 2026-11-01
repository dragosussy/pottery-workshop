// Package hw talks to the hardware: SHT humidity sensors over I²C, the pump
// relay on a GPIO line, and simulated versions of both for a laptop.
package hw

import "context"

// Sensor reads relative humidity (%) and temperature (°C).
type Sensor interface {
	Read(ctx context.Context) (rh, temp float64, err error)
}

// Switch turns the pump on or off. Setting the same value twice is fine.
type Switch interface {
	Set(on bool) error
}
