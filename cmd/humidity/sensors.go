package main

import (
	"fmt"

	"humidity/internal/config"
	"humidity/internal/sensor"
	"humidity/internal/sensor/sht"
	"humidity/internal/sensor/sim"
)

// openSensors builds every configured sensor. If one fails to open, the
// ones already opened are closed again.
func openSensors(cfg config.Config) ([]sensor.Sensor, error) {
	var opened []sensor.Sensor

	for _, spec := range cfg.Sensors {
		s, err := openSensor(cfg, spec)
		if err != nil {
			closeSensors(opened)
			return nil, fmt.Errorf("sensor %q: %w", spec.Name, err)
		}
		opened = append(opened, s)
	}
	return opened, nil
}

func openSensor(cfg config.Config, spec config.SensorSpec) (sensor.Sensor, error) {
	switch cfg.Driver {
	case "sht3x":
		return openSHT(sht.SHT3x, cfg, spec)
	case "sht4x":
		return openSHT(sht.SHT4x, cfg, spec)
	case "sim":
		return sim.New(spec.Name), nil
	default:
		return nil, fmt.Errorf("unknown SENSOR_DRIVER %q (want sht3x, sht4x or sim)", cfg.Driver)
	}
}

func openSHT(v sht.Variant, cfg config.Config, spec config.SensorSpec) (sensor.Sensor, error) {
	s, err := sht.Open(spec.Name, v, cfg.I2CBus, spec.Address)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func closeSensors(sensors []sensor.Sensor) {
	for _, s := range sensors {
		s.Close()
	}
}
