// Package hw talks to the hardware: SHT humidity sensors over I²C, the pump
// relay on a GPIO line, the small ST7789 screen over SPI, and simulated
// versions of all three for a laptop.
package hw

import (
	"context"
	"image"
)

// Sensor reads relative humidity (%) and temperature (°C).
type Sensor interface {
	Read(ctx context.Context) (rh, temp float64, err error)
}

// Switch turns the pump on or off. Setting the same value twice is fine.
type Switch interface {
	Set(on bool) error
}

// The screen is used in landscape.
const (
	ScreenWidth  = 320
	ScreenHeight = 170
)

// Screen shows a ScreenWidth × ScreenHeight frame.
type Screen interface {
	Show(img *image.RGBA) error
	// Close blanks the screen and releases it.
	Close() error
}
