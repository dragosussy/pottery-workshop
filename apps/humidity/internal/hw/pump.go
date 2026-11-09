//go:build linux

package hw

import (
	"fmt"

	"github.com/warthog618/go-gpiocdev"
)

// GPIOPump is the pump relay on one GPIO line of the Raspberry Pi 5 header.
type GPIOPump struct {
	line *gpiocdev.Line
}

// OpenGPIOPump requests BCM line pin as an output driven off from the first
// moment, so opening never pulses the pump. Many relay boards switch on when
// their input is low: activeLow inverts the line for them.
func OpenGPIOPump(pin int, activeLow bool) (*GPIOPump, error) {
	chip, err := findRP1()
	if err != nil {
		return nil, err
	}
	opts := []gpiocdev.LineReqOption{gpiocdev.WithConsumer("humidity-pump"), gpiocdev.AsOutput(0)}
	if activeLow {
		opts = append(opts, gpiocdev.AsActiveLow)
	}
	l, err := gpiocdev.RequestLine(chip, pin, opts...)
	if err != nil {
		return nil, fmt.Errorf("gpio %s line %d: %w", chip, pin, err)
	}
	return &GPIOPump{line: l}, nil
}

// findRP1 returns the name of the Pi 5 header chip. Its number changed
// between kernels (gpiochip4, later gpiochip0), its label didn't.
func findRP1() (string, error) {
	for _, name := range gpiocdev.Chips() {
		c, err := gpiocdev.NewChip(name)
		if err != nil {
			continue
		}
		label := c.Label
		c.Close()
		if label == "pinctrl-rp1" {
			return name, nil
		}
	}
	return "", fmt.Errorf("gpio: no pinctrl-rp1 chip found (is /dev/gpiochip0 mapped into the container?)")
}

func (p *GPIOPump) Set(on bool) error {
	v := 0
	if on {
		v = 1
	}
	return p.line.SetValue(v)
}
