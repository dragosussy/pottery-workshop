// Package gpio drives an on/off output (a relay or MOSFET board switching
// the pump) through the Linux GPIO character device.
package gpio

import "errors"

// Config says where the output is wired.
type Config struct {
	// Chip is a chip name (gpiochip0) or "" to find the Raspberry Pi 5
	// header chip (label pinctrl-rp1) automatically.
	Chip string
	// Line is the line offset on the chip. On the Pi 5 header chip this is
	// the BCM GPIO number (GPIO17 is physical pin 11).
	Line int
	// ActiveLow inverts the output: many relay boards switch on when their
	// input is pulled low.
	ActiveLow bool
}

// ErrUnsupported is returned on systems without GPIO character devices.
var ErrUnsupported = errors.New("gpio: only supported on Linux")

// rp1Label is the label of the Raspberry Pi 5 header GPIO chip.
const rp1Label = "pinctrl-rp1"
