//go:build linux

package sht

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// i2cSlave is I2C_SLAVE from <linux/i2c-dev.h>: it sets the address used by
// plain read and write calls on the file descriptor.
const i2cSlave = 0x0703

// i2cDevice is a /dev/i2c-N descriptor bound to one address. It uses raw
// syscalls because i2c-dev doesn't support the poller behind os.File.
type i2cDevice struct{ fd int }

func openDevice(bus string, addr uint16) (Device, error) {
	fd, err := unix.Open(bus, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s (is I²C enabled and the device mapped into the container?): %w", bus, err)
	}
	if err := unix.IoctlSetInt(fd, i2cSlave, int(addr)); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("select address %#x on %s: %w", addr, bus, err)
	}
	return &i2cDevice{fd: fd}, nil
}

func (d *i2cDevice) Read(p []byte) (int, error)  { return unix.Read(d.fd, p) }
func (d *i2cDevice) Write(p []byte) (int, error) { return unix.Write(d.fd, p) }
func (d *i2cDevice) Close() error                { return unix.Close(d.fd) }
