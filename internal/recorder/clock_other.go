//go:build !linux

package recorder

// ClockSynced always reports true off Linux, where development machines
// keep their own clock in sync.
func ClockSynced() bool { return true }
