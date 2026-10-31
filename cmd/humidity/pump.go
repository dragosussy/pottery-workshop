package main

import (
	"fmt"

	"humidity/internal/actuator"
	"humidity/internal/actuator/gpio"
	"humidity/internal/config"
	"humidity/internal/control"
	"humidity/internal/sensor"
	"humidity/internal/sensor/sim"
)

// openPump opens the configured pump switch, or returns nil for none. The
// simulated pump is attached to the simulated sensors, so pumping raises
// their humidity.
func openPump(cfg config.PumpConfig, sensors []sensor.Sensor) (actuator.Switch, error) {
	switch cfg.Driver {
	case "none":
		return nil, nil
	case "sim":
		p := sim.NewPump("pump")
		for _, s := range sensors {
			if ss, ok := s.(*sim.Sensor); ok {
				ss.Attach(p)
			}
		}
		return p, nil
	case "gpio":
		o, err := gpio.Open("pump", gpio.Config{Chip: cfg.Chip, Line: cfg.Line, ActiveLow: cfg.ActiveLow})
		if err != nil {
			return nil, fmt.Errorf("pump: %w", err)
		}
		return o, nil
	}
	return nil, fmt.Errorf("pump: unknown driver %q", cfg.Driver)
}

func controlConfig(cfg config.PumpConfig) control.Config {
	return control.Config{
		Pulse: cfg.Pulse, Rest: cfg.Rest, Hysteresis: cfg.Hysteresis,
		MaxRH: cfg.MaxRH,
	}
}
