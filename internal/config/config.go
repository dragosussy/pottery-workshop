// Package config loads runtime settings from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SensorSpec names one sensor and says where it is connected.
type SensorSpec struct {
	Name    string // permanent key for stored history; don't rename
	Address uint16 // 7-bit I²C address, e.g. 0x44
}

// Demo seed modes. DemoOff leaves the database alone.
const (
	DemoOff        = ""
	DemoLearning   = "learning"   // fewer than 5 training samples: learning period
	DemoCalibrated = "calibrated" // enough history to leave the learning period
)

// Config holds all runtime settings.
type Config struct {
	Driver       string // sht3x | sht4x | sim
	I2CBus       string // e.g. /dev/i2c-1 (the Pi header)
	Sensors      []SensorSpec
	PollInterval time.Duration

	HTTPAddr   string // e.g. :8080
	DBPath     string
	PhotoDir   string
	BackupDir  string
	BackupKeep int
	StudioTZ   string // initial value of the studio_tz setting
	DemoSeed   string // DemoOff | DemoLearning | DemoCalibrated
	ClockCheck bool   // wait for the kernel to report the clock synced before recording
	LogLevel   slog.Level

	Pump PumpConfig
}

// PumpConfig is the humidifier pump and its controller.
type PumpConfig struct {
	Driver    string // none | gpio | sim
	Chip      string // "" = find the Pi 5 header chip
	Line      int    // BCM GPIO number on the Pi 5 header
	ActiveLow bool

	Pulse      time.Duration
	Rest       time.Duration
	Hysteresis float64
	MaxRH      float64
}

// Load reads configuration using getenv (pass os.Getenv in production).
//
//	SENSOR_DRIVER  sht4x (default) | sht3x | sim
//	SENSORS        comma-separated name:address list, default "main:0x44",
//	               e.g. "shelf:0x44,top:0x45"
//	I2C_BUS        default /dev/i2c-1
//	POLL_INTERVAL  Go duration, default 5s
//	HTTP_ADDR      default ":$PORT", or ":8080" without PORT
//	DB_PATH        default /data/humidity.db
//	PHOTO_DIR      default photos/ next to DB_PATH (/data/photos)
//	BACKUP_DIR     default backups/ next to DB_PATH (/data/backups)
//	BACKUP_KEEP    number of backups kept, default 14
//	STUDIO_TZ      IANA zone, default UTC
//	DEMO_SEED      false (default) | true (= learning) | learning | calibrated;
//	               only with SENSOR_DRIVER=sim
//	CLOCK_CHECK    kernel (default): hold readings until the kernel reports
//	               the clock synced; off: trust the clock (for hosts whose
//	               time sync doesn't clear STA_UNSYNC)
//	LOG_LEVEL      debug | info (default) | warn | error
//
// The pump (see internal/control):
//
//	PUMP_DRIVER         none (default) | gpio | sim (only with SENSOR_DRIVER=sim)
//	PUMP_GPIO_LINE      BCM GPIO number of the relay input, e.g. 17 (pin 11); required for gpio
//	PUMP_ACTIVE_LOW     true | false; required for gpio (many relay boards are
//	                    active-low: their pump runs when the pin is low)
//	PUMP_GPIO_CHIP      default: the Pi 5 header chip (label pinctrl-rp1)
//	PUMP_PULSE          one pump run, default 10s
//	PUMP_REST           pause after a run, default 3m
//	PUMP_HYSTERESIS     pump when this many % below the target, default 2
//	PUMP_MAX_RH         never pump at or above this %, default 95
func Load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	cfg := Config{
		Driver:   get("SENSOR_DRIVER", "sht4x"),
		I2CBus:   get("I2C_BUS", "/dev/i2c-1"),
		HTTPAddr: get("HTTP_ADDR", ":"+get("PORT", "8080")),
		DBPath:   get("DB_PATH", "/data/humidity.db"),
		StudioTZ: get("STUDIO_TZ", "UTC"),
	}
	// Photos and backups live next to the database unless placed elsewhere,
	// so pointing DB_PATH at a dev directory is enough.
	dataDir := filepath.Dir(cfg.DBPath)
	cfg.PhotoDir = get("PHOTO_DIR", filepath.Join(dataDir, "photos"))
	cfg.BackupDir = get("BACKUP_DIR", filepath.Join(dataDir, "backups"))

	var err error

	cfg.PollInterval, err = time.ParseDuration(get("POLL_INTERVAL", "5s"))
	if err != nil {
		return Config{}, fmt.Errorf("POLL_INTERVAL: %w", err)
	}
	if cfg.PollInterval <= 0 {
		return Config{}, fmt.Errorf("POLL_INTERVAL: must be positive, got %s", cfg.PollInterval)
	}

	cfg.Sensors, err = parseSensors(get("SENSORS", "main:0x44"))
	if err != nil {
		return Config{}, fmt.Errorf("SENSORS: %w", err)
	}

	cfg.BackupKeep, err = strconv.Atoi(get("BACKUP_KEEP", "14"))
	if err != nil || cfg.BackupKeep < 1 {
		return Config{}, fmt.Errorf("BACKUP_KEEP: want a positive integer, got %q", getenv("BACKUP_KEEP"))
	}

	if _, err := time.LoadLocation(cfg.StudioTZ); err != nil {
		return Config{}, fmt.Errorf("STUDIO_TZ: %w", err)
	}

	switch v := strings.ToLower(get("DEMO_SEED", "false")); v {
	case "false", "0", "no", "off":
		cfg.DemoSeed = DemoOff
	case "true", "1", "yes", "on", DemoLearning:
		cfg.DemoSeed = DemoLearning
	case DemoCalibrated:
		cfg.DemoSeed = DemoCalibrated
	default:
		return Config{}, fmt.Errorf("DEMO_SEED: want true, false, learning or calibrated, got %q", v)
	}
	if cfg.DemoSeed != DemoOff && cfg.Driver != "sim" {
		return Config{}, fmt.Errorf("DEMO_SEED: only allowed with SENSOR_DRIVER=sim")
	}

	switch v := get("CLOCK_CHECK", "kernel"); v {
	case "kernel":
		cfg.ClockCheck = true
	case "off":
	default:
		return Config{}, fmt.Errorf("CLOCK_CHECK: want kernel or off, got %q", v)
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
	}

	cfg.Pump, err = loadPump(get, cfg.Driver)
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func loadPump(get func(key, def string) string, sensorDriver string) (PumpConfig, error) {
	p := PumpConfig{Driver: get("PUMP_DRIVER", "none"), Chip: get("PUMP_GPIO_CHIP", "")}
	switch p.Driver {
	case "none":
	case "sim":
		if sensorDriver != "sim" {
			return p, fmt.Errorf("PUMP_DRIVER: sim only with SENSOR_DRIVER=sim")
		}
	case "gpio":
		line, err := strconv.Atoi(get("PUMP_GPIO_LINE", ""))
		if err != nil || line < 0 || line > 53 {
			return p, fmt.Errorf("PUMP_GPIO_LINE: want the BCM GPIO number of the relay input (e.g. 17), got %q", get("PUMP_GPIO_LINE", ""))
		}
		p.Line = line
		// No default: guessing the polarity wrong runs the pump while the
		// app thinks it is off.
		switch v := strings.ToLower(get("PUMP_ACTIVE_LOW", "")); v {
		case "true", "1", "yes":
			p.ActiveLow = true
		case "false", "0", "no":
		default:
			return p, fmt.Errorf("PUMP_ACTIVE_LOW: set true or false to match the relay board, got %q", v)
		}
	default:
		return p, fmt.Errorf("PUMP_DRIVER: want none, gpio or sim, got %q", p.Driver)
	}

	durations := []struct {
		key, def string
		dst      *time.Duration
	}{
		{"PUMP_PULSE", "10s", &p.Pulse},
		{"PUMP_REST", "3m", &p.Rest},
	}
	for _, d := range durations {
		v, err := time.ParseDuration(get(d.key, d.def))
		if err != nil || v <= 0 {
			return p, fmt.Errorf("%s: want a positive duration, got %q", d.key, get(d.key, d.def))
		}
		*d.dst = v
	}
	if p.Pulse > 2*time.Minute {
		return p, fmt.Errorf("PUMP_PULSE: at most 2m, got %s", p.Pulse)
	}
	var err error
	if p.Hysteresis, err = strconv.ParseFloat(get("PUMP_HYSTERESIS", "2"), 64); err != nil || p.Hysteresis < 0.5 || p.Hysteresis > 10 {
		return p, fmt.Errorf("PUMP_HYSTERESIS: want 0.5-10, got %q", get("PUMP_HYSTERESIS", "2"))
	}
	if p.MaxRH, err = strconv.ParseFloat(get("PUMP_MAX_RH", "95"), 64); err != nil || p.MaxRH < 60 || p.MaxRH > 99 {
		return p, fmt.Errorf("PUMP_MAX_RH: want 60-99, got %q", get("PUMP_MAX_RH", "95"))
	}
	return p, nil
}

// parseSensors parses "shelf:0x44,top:0x45". Addresses may be hex (0x44) or
// decimal (68).
func parseSensors(s string) ([]SensorSpec, error) {
	var specs []SensorSpec
	seenName := make(map[string]bool)
	seenAddr := make(map[uint16]bool)

	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)

		name, addrStr, ok := strings.Cut(part, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid entry %q, want name:address", part)
		}
		// Names are used as keys in the history and in ?sensors= lists.
		if name == "room" || !validName(name) {
			return nil, fmt.Errorf("invalid sensor name %q: use letters, digits, - or _, and not \"room\"", name)
		}

		// 0x00–0x07 and 0x78–0x7F are reserved by the I²C spec.
		addr, err := strconv.ParseUint(strings.TrimSpace(addrStr), 0, 16)
		if err != nil || addr < 0x08 || addr > 0x77 {
			return nil, fmt.Errorf("invalid I²C address in %q, want 0x08-0x77", part)
		}

		if seenName[name] {
			return nil, fmt.Errorf("duplicate sensor name %q", name)
		}
		if seenAddr[uint16(addr)] {
			return nil, fmt.Errorf("duplicate I²C address %#x", addr)
		}
		seenName[name] = true
		seenAddr[uint16(addr)] = true

		specs = append(specs, SensorSpec{Name: name, Address: uint16(addr)})
	}
	return specs, nil
}

func validName(s string) bool {
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return len(s) <= 32
}
