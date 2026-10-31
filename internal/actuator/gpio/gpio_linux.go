//go:build linux

package gpio

import (
	"fmt"
	"sync"

	"github.com/warthog618/go-gpiocdev"
)

// Output is one GPIO line requested as an output, initially off.
type Output struct {
	name string
	mu   sync.Mutex
	line *gpiocdev.Line
}

// Open requests the line as an output driven to off (inactive) from the
// first moment, so opening never pulses the pump.
func Open(name string, cfg Config) (*Output, error) {
	chip := cfg.Chip
	if chip == "" {
		var err error
		if chip, err = findRP1(); err != nil {
			return nil, err
		}
	}
	opts := []gpiocdev.LineReqOption{gpiocdev.WithConsumer("humidity-" + name), gpiocdev.AsOutput(0)}
	if cfg.ActiveLow {
		opts = append(opts, gpiocdev.AsActiveLow)
	}
	l, err := gpiocdev.RequestLine(chip, cfg.Line, opts...)
	if err != nil {
		return nil, fmt.Errorf("gpio %s line %d: %w", chip, cfg.Line, err)
	}
	return &Output{name: name, line: l}, nil
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
		if label == rp1Label {
			return name, nil
		}
	}
	return "", fmt.Errorf("gpio: no %s chip found; set PUMP_GPIO_CHIP (and map /dev/gpiochipN into the container)", rp1Label)
}

func (o *Output) Name() string { return o.name }

// Set drives the line active (on) or inactive (off).
func (o *Output) Set(on bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.line == nil {
		return fmt.Errorf("gpio %s: closed", o.name)
	}
	v := 0
	if on {
		v = 1
	}
	return o.line.SetValue(v)
}

// Close switches the output off, then releases the line.
func (o *Output) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.line == nil {
		return nil
	}
	err := o.line.SetValue(0)
	if cerr := o.line.Close(); err == nil {
		err = cerr
	}
	o.line = nil
	return err
}
