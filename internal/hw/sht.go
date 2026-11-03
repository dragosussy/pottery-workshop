package hw

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sensirion SHT3x (e.g. SHT31) and SHT4x (SHT40/41/45) over I²C.
//
// Both families answer a measurement command with six bytes: temperature and
// humidity, each a big-endian 16-bit word followed by a CRC-8. They differ in
// the command bytes, the conversion time and the humidity formula.

// Variant selects the sensor family.
type Variant int

const (
	SHT3x Variant = iota + 1 // SHT30/31/35
	SHT4x                    // SHT40/41/45
)

const attempts = 3

// family holds what differs between SHT3x and SHT4x (from the datasheets).
type family struct {
	measure   []byte        // single-shot, highest repeatability
	wait      time.Duration // conversion time, with margin over the datasheet max
	reset     []byte        // soft reset
	resetWait time.Duration
	humidity  func(raw uint16) float64
}

var families = map[Variant]family{
	SHT3x: {
		measure:   []byte{0x24, 0x00}, // high repeatability, no clock stretching; max 15.5 ms
		wait:      20 * time.Millisecond,
		reset:     []byte{0x30, 0xA2},
		resetWait: 2 * time.Millisecond,
		humidity:  func(raw uint16) float64 { return 100 * float64(raw) / 65535 },
	},
	SHT4x: {
		measure:   []byte{0xFD}, // high precision; max 8.3 ms
		wait:      10 * time.Millisecond,
		reset:     []byte{0x94},
		resetWait: 2 * time.Millisecond,
		humidity:  func(raw uint16) float64 { return -6 + 125*float64(raw)/65535 },
	},
}

// Device is an I²C device already bound to the sensor's address: every Write
// and Read is one bus transaction.
type Device interface {
	Write(p []byte) (int, error)
	Read(p []byte) (int, error)
	Close() error
}

// SHT is one SHT sensor on an I²C bus.
type SHT struct {
	dev Device
	fam family
}

// OpenSHT connects to the sensor at addr (e.g. 0x44) on bus (e.g.
// /dev/i2c-1). It fails only if the bus can't be opened: a missing sensor
// shows up as read errors, and reads start working once it's connected.
func OpenSHT(v Variant, bus string, addr uint16) (*SHT, error) {
	dev, err := openDevice(bus, addr)
	if err != nil {
		return nil, err
	}
	s, err := NewSHT(v, dev)
	if err != nil {
		dev.Close()
		return nil, err
	}
	return s, nil
}

// NewSHT wraps an open device.
func NewSHT(v Variant, dev Device) (*SHT, error) {
	fam, ok := families[v]
	if !ok {
		return nil, fmt.Errorf("unknown SHT variant %d", v)
	}
	return &SHT{dev: dev, fam: fam}, nil
}

// Reset soft-resets the sensor; an error usually means it isn't connected
// at this address.
func (s *SHT) Reset() error {
	if err := s.write(s.fam.reset); err != nil {
		return fmt.Errorf("soft reset (is the sensor connected at this address?): %w", err)
	}
	time.Sleep(s.fam.resetWait)
	return nil
}

// Read retries a few times: a disturbed bus transaction or a checksum error
// is usually gone on the next attempt.
func (s *SHT) Read(ctx context.Context) (rh, temp float64, err error) {
	for range attempts {
		rh, temp, err = s.readOnce(ctx)
		if err == nil {
			return rh, temp, nil
		}
		if ctx.Err() != nil {
			return 0, 0, ctx.Err()
		}
	}
	return 0, 0, fmt.Errorf("after %d attempts: %w", attempts, err)
}

func (s *SHT) readOnce(ctx context.Context) (rh, temp float64, err error) {
	if err := s.write(s.fam.measure); err != nil {
		return 0, 0, fmt.Errorf("start measurement: %w", err)
	}
	select {
	case <-time.After(s.fam.wait):
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	}

	// One 6-byte read is one bus transaction; a short read can't be resumed.
	var buf [6]byte
	n, err := s.dev.Read(buf[:])
	if err != nil {
		return 0, 0, fmt.Errorf("read result: %w", err)
	}
	if n != len(buf) {
		return 0, 0, fmt.Errorf("read result: got %d of %d bytes", n, len(buf))
	}
	temp, rh, err = decode(s.fam, buf)
	return rh, temp, err
}

func (s *SHT) write(cmd []byte) error {
	n, err := s.dev.Write(cmd)
	if err != nil {
		return err
	}
	if n != len(cmd) {
		return fmt.Errorf("wrote %d of %d bytes", n, len(cmd))
	}
	return nil
}

// decode checks both checksums and converts the raw words. A floating or
// shorted bus reads as all 0xFF or 0x00, which fails the checksum.
func decode(fam family, b [6]byte) (temp, hum float64, err error) {
	if crc8(b[0:2]) != b[2] {
		return 0, 0, errors.New("temperature checksum mismatch")
	}
	if crc8(b[3:5]) != b[5] {
		return 0, 0, errors.New("humidity checksum mismatch")
	}

	rawT := uint16(b[0])<<8 | uint16(b[1])
	rawH := uint16(b[3])<<8 | uint16(b[4])

	temp = -45 + 175*float64(rawT)/65535
	hum = min(max(fam.humidity(rawH), 0), 100) // SHT4x's formula overshoots 0–100 by design
	return temp, hum, nil
}

// crc8 is the Sensirion checksum: polynomial 0x31 (x⁸+x⁵+x⁴+1), init 0xFF.
func crc8(data []byte) byte {
	crc := byte(0xFF)
	for _, b := range data {
		crc ^= b
		for range 8 {
			if crc&0x80 != 0 {
				crc = crc<<1 ^ 0x31
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
