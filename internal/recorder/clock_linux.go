//go:build linux

package recorder

import "golang.org/x/sys/unix"

// ClockSynced reports whether the kernel considers the system clock
// synchronised (STA_UNSYNC cleared, as set by systemd-timesyncd or chrony).
// After days switched off, the Pi's clock is wrong until it syncs. If the
// call itself fails the clock is assumed good, so recording never stalls on
// a kernel quirk.
func ClockSynced() bool {
	var tx unix.Timex
	if _, err := unix.Adjtimex(&tx); err != nil {
		return true
	}
	return tx.Status&unix.STA_UNSYNC == 0
}
