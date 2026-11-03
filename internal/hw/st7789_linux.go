//go:build linux

package hw

import (
	"fmt"
	"image"
	"time"
	"unsafe"

	"github.com/warthog618/go-gpiocdev"
	"golang.org/x/sys/unix"
)

// ST7789 is a 1.9" 170 × 320 IPS screen with an ST7789 controller on SPI,
// plus three GPIO lines: data/command, reset and backlight.
type ST7789 struct {
	fd          int // the /dev/spidevB.C file; CS is handled by spidev
	dc, rst, bl *gpiocdev.Line
	buf         []byte // one frame in RGB565
}

// spidev ioctls from <linux/spi/spidev.h>.
const (
	spiIOCWrMode        = 0x40016b01
	spiIOCWrBitsPerWord = 0x40016b03
	spiIOCWrMaxSpeedHz  = 0x40046b04
)

const (
	spiSpeed = 32_000_000
	spiChunk = 4096 // spidev's default buffer size per write
	// The controller has 240 columns of memory; the 170-pixel side of this
	// panel sits in the middle of them.
	st7789Offset = 35
)

// OpenST7789 opens the screen on spidev (e.g. /dev/spidev0.0) with the BCM
// GPIO lines for data/command, reset and backlight, resets and initialises
// it, and leaves it black with the backlight on. flip turns the picture
// upside down, for mounting the other way round.
func OpenST7789(spidev string, dcPin, rstPin, blPin int, flip bool) (*ST7789, error) {
	fd, err := unix.Open(spidev, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s (is SPI enabled and the device mapped into the container?): %w", spidev, err)
	}
	s := &ST7789{fd: fd, buf: make([]byte, ScreenWidth*ScreenHeight*2)}
	mode, bits, speed := uint8(0), uint8(8), uint32(spiSpeed)
	for _, c := range []struct {
		req uintptr
		ptr unsafe.Pointer
	}{{spiIOCWrMode, unsafe.Pointer(&mode)}, {spiIOCWrBitsPerWord, unsafe.Pointer(&bits)}, {spiIOCWrMaxSpeedHz, unsafe.Pointer(&speed)}} {
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), c.req, uintptr(c.ptr)); errno != 0 {
			unix.Close(fd)
			return nil, fmt.Errorf("configure %s: %w", spidev, errno)
		}
	}

	chip, err := findRP1()
	if err != nil {
		s.Close()
		return nil, err
	}
	line := func(pin, initial int) (*gpiocdev.Line, error) {
		l, err := gpiocdev.RequestLine(chip, pin, gpiocdev.WithConsumer("humidity-screen"), gpiocdev.AsOutput(initial))
		if err != nil {
			return nil, fmt.Errorf("screen gpio line %d: %w", pin, err)
		}
		return l, nil
	}
	if s.dc, err = line(dcPin, 0); err == nil {
		if s.rst, err = line(rstPin, 1); err == nil {
			s.bl, err = line(blPin, 0)
		}
	}
	if err != nil {
		s.Close()
		return nil, err
	}

	// Hardware reset, then the usual ST7789 start-up.
	s.rst.SetValue(0)
	time.Sleep(10 * time.Millisecond)
	s.rst.SetValue(1)
	time.Sleep(120 * time.Millisecond)
	madctl := byte(0x60) // landscape: swap rows/columns, mirror x
	if flip {
		madctl = 0xA0
	}
	steps := []struct {
		cmd   byte
		data  []byte
		pause time.Duration
	}{
		{0x01, nil, 150 * time.Millisecond}, // software reset
		{0x11, nil, 120 * time.Millisecond}, // sleep out
		{0x3A, []byte{0x55}, 0},             // 16 bits per pixel
		{0x36, []byte{madctl}, 0},           // orientation
		{0x21, nil, 0},                      // IPS panels need inversion on
		{0x13, nil, 10 * time.Millisecond},  // normal mode
		{0x29, nil, 10 * time.Millisecond},  // display on
	}
	for _, st := range steps {
		if err := s.command(st.cmd, st.data...); err != nil {
			s.Close()
			return nil, err
		}
		time.Sleep(st.pause)
	}
	if err := s.Show(image.NewRGBA(image.Rect(0, 0, ScreenWidth, ScreenHeight))); err != nil {
		s.Close()
		return nil, err
	}
	return s, s.bl.SetValue(1)
}

// command sends a command byte (DC low), then its parameters (DC high).
func (s *ST7789) command(cmd byte, data ...byte) error {
	if err := s.dc.SetValue(0); err != nil {
		return err
	}
	if err := s.write([]byte{cmd}); err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if err := s.dc.SetValue(1); err != nil {
		return err
	}
	return s.write(data)
}

func (s *ST7789) write(p []byte) error {
	for len(p) > 0 {
		n := min(len(p), spiChunk)
		if _, err := unix.Write(s.fd, p[:n]); err != nil {
			return fmt.Errorf("screen spi: %w", err)
		}
		p = p[n:]
	}
	return nil
}

// Show sends a whole frame.
func (s *ST7789) Show(img *image.RGBA) error {
	x1, y0, y1 := ScreenWidth-1, st7789Offset, st7789Offset+ScreenHeight-1
	if err := s.command(0x2A, 0, 0, byte(x1>>8), byte(x1)); err != nil { // columns
		return err
	}
	if err := s.command(0x2B, byte(y0>>8), byte(y0), byte(y1>>8), byte(y1)); err != nil { // rows
		return err
	}
	toRGB565(img, s.buf)
	if err := s.command(0x2C); err != nil { // memory write
		return err
	}
	if err := s.dc.SetValue(1); err != nil {
		return err
	}
	return s.write(s.buf)
}

// Close turns the backlight and panel off and releases everything.
func (s *ST7789) Close() error {
	if s.bl != nil {
		s.bl.SetValue(0)
		s.bl.Close()
	}
	if s.dc != nil && s.rst != nil {
		s.command(0x28) // display off
		s.command(0x10) // sleep in
	}
	if s.dc != nil {
		s.dc.Close()
	}
	if s.rst != nil {
		s.rst.Close()
	}
	return unix.Close(s.fd)
}
