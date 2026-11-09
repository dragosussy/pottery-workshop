package hw

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"
)

// fakeDevice records writes and answers reads from a script.
type fakeDevice struct {
	writes   [][]byte
	reads    [][]byte // returned in order; an empty entry means a read error
	writeErr error
	closed   bool
}

func (d *fakeDevice) Write(p []byte) (int, error) {
	if d.writeErr != nil {
		return 0, d.writeErr
	}
	d.writes = append(d.writes, append([]byte(nil), p...))
	return len(p), nil
}

func (d *fakeDevice) Read(p []byte) (int, error) {
	if len(d.reads) == 0 {
		return 0, errors.New("no scripted read")
	}
	r := d.reads[0]
	d.reads = d.reads[1:]
	if len(r) == 0 {
		return 0, errors.New("remote I/O error")
	}
	return copy(p, r), nil
}

func (d *fakeDevice) Close() error {
	d.closed = true
	return nil
}

// frame builds a 6-byte response with valid checksums.
func frame(rawT, rawH uint16) []byte {
	t := []byte{byte(rawT >> 8), byte(rawT)}
	h := []byte{byte(rawH >> 8), byte(rawH)}
	return []byte{t[0], t[1], crc8(t), h[0], h[1], crc8(h)}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestCRC8_DatasheetExample(t *testing.T) {
	if got := crc8([]byte{0xBE, 0xEF}); got != 0x92 {
		t.Errorf("crc8(0xBEEF) = %#x, want 0x92", got)
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name       string
		variant    Variant
		rawT, rawH uint16
		temp, hum  float64
	}{
		{"sht3x mid-range", SHT3x, 0x6666, 0x8000, 25.0, 50.0},
		{"sht4x mid-range", SHT4x, 0x6666, 0x8000, 25.0, 56.5},
		{"sht4x clamps low", SHT4x, 0x6666, 0x0000, 25.0, 0},
		{"sht4x clamps high", SHT4x, 0x6666, 0xFFFF, 25.0, 100},
		{"min temperature", SHT3x, 0x0000, 0x8000, -45.0, 50.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf [6]byte
			copy(buf[:], frame(tc.rawT, tc.rawH))

			temp, hum, err := decode(families[tc.variant], buf)
			if err != nil {
				t.Fatalf("decode() error: %v", err)
			}
			if !near(temp, tc.temp) || !near(hum, tc.hum) {
				t.Errorf("decode() = %.2f°C %.2f%%, want %.2f°C %.2f%%", temp, hum, tc.temp, tc.hum)
			}
		})
	}
}

func TestDecode_ChecksumErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		buf  [6]byte
	}{
		{"floating bus", [6]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
		{"shorted bus", [6]byte{}},
		{"bad humidity crc", [6]byte{0xBE, 0xEF, 0x92, 0x80, 0x00, 0x00}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := decode(families[SHT3x], tc.buf); err == nil {
				t.Error("decode() succeeded, want checksum error")
			}
		})
	}
}

func TestReset(t *testing.T) {
	for _, tc := range []struct {
		variant Variant
		reset   []byte
	}{
		{SHT3x, []byte{0x30, 0xA2}},
		{SHT4x, []byte{0x94}},
	} {
		dev := &fakeDevice{}
		s, err := NewSHT(tc.variant, dev)
		if err != nil {
			t.Fatalf("NewSHT() error: %v", err)
		}
		if err := s.Reset(); err != nil {
			t.Fatalf("Reset() error: %v", err)
		}
		if len(dev.writes) != 1 || !bytes.Equal(dev.writes[0], tc.reset) {
			t.Errorf("variant %d: writes = %x, want one reset %x", tc.variant, dev.writes, tc.reset)
		}
	}
}

func TestReset_NoSensor(t *testing.T) {
	dev := &fakeDevice{writeErr: errors.New("remote I/O error")}
	s, err := NewSHT(SHT4x, dev)
	if err != nil {
		t.Fatalf("NewSHT() error: %v", err)
	}
	if err := s.Reset(); err == nil {
		t.Error("Reset() succeeded without a responding sensor")
	}
}

func TestRead_RetriesThenSucceeds(t *testing.T) {
	bad := frame(0x6666, 0x8000)
	bad[5] ^= 0xFF // corrupt the humidity checksum

	dev := &fakeDevice{reads: [][]byte{nil, bad, frame(0x6666, 0x8000)}}
	s, err := NewSHT(SHT4x, dev)
	if err != nil {
		t.Fatalf("NewSHT() error: %v", err)
	}

	rh, temp, err := s.Read(context.Background())
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if !near(temp, 25.0) || !near(rh, 56.5) {
		t.Errorf("Read() = %.2f%% %.2f°C", rh, temp)
	}
	// three measurement commands
	if len(dev.writes) != 3 || !bytes.Equal(dev.writes[2], []byte{0xFD}) {
		t.Errorf("writes = %x, want 3× FD", dev.writes)
	}
}

func TestRead_GivesUp(t *testing.T) {
	dev := &fakeDevice{reads: [][]byte{nil, nil, nil}}
	s, err := NewSHT(SHT3x, dev)
	if err != nil {
		t.Fatalf("NewSHT() error: %v", err)
	}
	if _, _, err := s.Read(context.Background()); err == nil {
		t.Error("Read() succeeded, want error after all attempts failed")
	}
}

func TestRead_ShortRead(t *testing.T) {
	short := frame(0x6666, 0x8000)[:4]
	dev := &fakeDevice{reads: [][]byte{short, short, short}}
	s, err := NewSHT(SHT3x, dev)
	if err != nil {
		t.Fatalf("NewSHT() error: %v", err)
	}
	if _, _, err := s.Read(context.Background()); err == nil {
		t.Error("Read() succeeded on short reads")
	}
}

func TestRead_Cancelled(t *testing.T) {
	dev := &fakeDevice{}
	s, err := NewSHT(SHT3x, dev)
	if err != nil {
		t.Fatalf("NewSHT() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Read() error = %v, want context.Canceled", err)
	}
}
