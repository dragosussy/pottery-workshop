// Package actuator defines the shared vocabulary for things the controller
// switches on and off (the pump). Concrete drivers live in sub-packages
// (gpio) and in the simulator (sensor/sim).
package actuator

// Switch is an on/off output.
type Switch interface {
	Name() string
	// Set switches the output. It must be safe to call repeatedly with the
	// same value.
	Set(on bool) error
	// Close switches the output off and releases it.
	Close() error
}
