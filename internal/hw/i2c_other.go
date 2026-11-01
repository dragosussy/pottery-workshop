//go:build !linux

package hw

import "errors"

// This stub keeps the project compiling on macOS/Windows, where you can still
// develop against the simulated sensor.

var errUnsupported = errors.New("sht driver requires Linux (I²C character device)")

func openDevice(bus string, addr uint16) (Device, error) {
	return nil, errUnsupported
}
