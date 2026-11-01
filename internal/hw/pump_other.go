//go:build !linux

package hw

import "errors"

// GPIOPump is unavailable off Linux; use PUMP=sim on a laptop.
type GPIOPump struct{}

func OpenGPIOPump(pin int, activeLow bool) (*GPIOPump, error) {
	return nil, errors.New("gpio pump requires Linux")
}

func (p *GPIOPump) Set(on bool) error { return errors.New("gpio pump requires Linux") }
