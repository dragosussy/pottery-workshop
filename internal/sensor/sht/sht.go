// Package sht reads Sensirion SHT3x (e.g. SHT31) and SHT4x (SHT40/41/45)
// temperature and humidity sensors over I²C.
//
// Both families answer a measurement command with six bytes: temperature and
// humidity, each a big-endian 16-bit word followed by a CRC-8. They differ in
// the command bytes, the conversion time and the humidity formula.
package sht

import (
	"context"
	"errors"
	"fmt"
	"time"

	"humidity/internal/sensor"
)

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

// Sensor is one SHT sensor on an I²C bus.
type Sensor struct {
	name string
	dev  Device
	fam  family
}

// Open connects to the sensor at addr (e.g. 0x44) on bus (e.g. /dev/i2c-1)
// and soft-resets it, so a missing or miswired sensor fails here rather than
// on the first poll.
func Open(name string, v Variant, bus string, addr uint16) (*Sensor, error) {
	dev, err := openDevice(bus, addr)
	if err != nil {
		return nil, err
	}
	s, err := New(name, v, dev)
	if err != nil {
		dev.Close()
		return nil, err
	}
	return s, nil
}

// New wraps an open device and soft-resets the sensor. It takes ownership of
// dev only on success.
func New(name string, v Variant, dev Device) (*Sensor, error) {
	fam, ok := families[v]
	if !ok {
		return nil, fmt.Errorf("unknown SHT variant %d", v)
	}

	s := &Sensor{name: name, dev: dev, fam: fam}
	if err := s.write(fam.reset); err != nil {
		return nil, fmt.Errorf("soft reset (is the sensor connected at this address?): %w", err)
	}
	time.Sleep(fam.resetWait)
	return s, nil
}

func (s *Sensor) Name() string { return s.name }

func (s *Sensor) Close() error { return s.dev.Close() }

// Read retries a few times: a disturbed bus transaction or a checksum error
// is usually gone on the next attempt.
func (s *Sensor) Read(ctx context.Context) (sensor.Reading, error) {
	var lastErr error
	for range attempts {
		r, err := s.readOnce(ctx)
		if err == nil {
			return r, nil
		}
		if ctx.Err() != nil {
			return sensor.Reading{}, ctx.Err()
		}
		lastErr = err
	}
	return sensor.Reading{}, fmt.Errorf("after %d attempts: %w", attempts, lastErr)
}

func (s *Sensor) readOnce(ctx context.Context) (sensor.Reading, error) {
	if err := s.write(s.fam.measure); err != nil {
		return sensor.Reading{}, fmt.Errorf("start measurement: %w", err)
	}
	if err := sleepCtx(ctx, s.fam.wait); err != nil {
		return sensor.Reading{}, err
	}

	// One 6-byte read is one bus transaction; a short read can't be resumed.
	var buf [6]byte
	n, err := s.dev.Read(buf[:])
	if err != nil {
		return sensor.Reading{}, fmt.Errorf("read result: %w", err)
	}
	if n != len(buf) {
		return sensor.Reading{}, fmt.Errorf("read result: got %d of %d bytes", n, len(buf))
	}

	temp, hum, err := decode(s.fam, buf)
	if err != nil {
		return sensor.Reading{}, err
	}
	return sensor.Reading{
		Sensor:      s.name,
		Temperature: temp,
		Humidity:    hum,
		At:          time.Now(),
	}, nil
}

func (s *Sensor) write(cmd []byte) error {
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

// sleepCtx sleeps for d, returning early with ctx's error if it is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
