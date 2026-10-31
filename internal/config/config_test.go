package config

import (
	"slices"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Driver != "sht4x" {
		t.Errorf("Driver = %q, want sht4x", cfg.Driver)
	}
	if cfg.I2CBus != "/dev/i2c-1" {
		t.Errorf("I2CBus = %q, want /dev/i2c-1", cfg.I2CBus)
	}
	if cfg.PollInterval != 5*time.Second {
		t.Errorf("PollInterval = %v, want 5s", cfg.PollInterval)
	}
	if want := []SensorSpec{{Name: "main", Address: 0x44}}; !slices.Equal(cfg.Sensors, want) {
		t.Errorf("Sensors = %v, want %v", cfg.Sensors, want)
	}
	if cfg.HTTPAddr != ":8080" || cfg.DBPath != "/data/humidity.db" || cfg.PhotoDir != "/data/photos" ||
		cfg.BackupDir != "/data/backups" || cfg.BackupKeep != 14 || cfg.StudioTZ != "UTC" {
		t.Errorf("storage/http defaults = %+v", cfg)
	}
	if cfg.DemoSeed != DemoOff {
		t.Errorf("DemoSeed = %q, want off", cfg.DemoSeed)
	}
	if !cfg.ClockCheck {
		t.Error("ClockCheck = false, want on by default")
	}
}

func TestLoad_DataDirsFollowDBPath(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DB_PATH": "dev/dev.db"}))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.PhotoDir != "dev/photos" || cfg.BackupDir != "dev/backups" {
		t.Errorf("PhotoDir, BackupDir = %q, %q, want dev/photos, dev/backups", cfg.PhotoDir, cfg.BackupDir)
	}
}

func TestLoad_HTTPAddrFromPort(t *testing.T) {
	cfg, err := Load(env(map[string]string{"PORT": "9000"}))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.HTTPAddr != ":9000" {
		t.Errorf("HTTPAddr = %q, want :9000", cfg.HTTPAddr)
	}
}

func TestLoad_DemoSeed(t *testing.T) {
	tests := []struct{ in, want string }{
		{"true", DemoLearning},
		{"learning", DemoLearning},
		{"calibrated", DemoCalibrated},
		{"false", DemoOff},
	}
	for _, tc := range tests {
		cfg, err := Load(env(map[string]string{"SENSOR_DRIVER": "sim", "DEMO_SEED": tc.in}))
		if err != nil {
			t.Fatalf("DEMO_SEED=%s: %v", tc.in, err)
		}
		if cfg.DemoSeed != tc.want {
			t.Errorf("DEMO_SEED=%s: got %q, want %q", tc.in, cfg.DemoSeed, tc.want)
		}
	}
}

func TestLoad_Overrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"SENSOR_DRIVER": "sim",
		"I2C_BUS":       "/dev/i2c-3",
		"POLL_INTERVAL": "1500ms",
		"SENSORS":       "shelf:0x44, top:69",
	}))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Driver != "sim" {
		t.Errorf("Driver = %q, want sim", cfg.Driver)
	}
	if cfg.I2CBus != "/dev/i2c-3" {
		t.Errorf("I2CBus = %q, want /dev/i2c-3", cfg.I2CBus)
	}
	if cfg.PollInterval != 1500*time.Millisecond {
		t.Errorf("PollInterval = %v, want 1.5s", cfg.PollInterval)
	}
	want := []SensorSpec{{Name: "shelf", Address: 0x44}, {Name: "top", Address: 0x45}}
	if !slices.Equal(cfg.Sensors, want) {
		t.Errorf("Sensors = %v, want %v", cfg.Sensors, want)
	}
}

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"interval not a duration", map[string]string{"POLL_INTERVAL": "abc"}},
		{"interval zero", map[string]string{"POLL_INTERVAL": "0s"}},
		{"interval negative", map[string]string{"POLL_INTERVAL": "-1s"}},
		{"sensor missing address", map[string]string{"SENSORS": "main"}},
		{"sensor missing name", map[string]string{"SENSORS": ":0x44"}},
		{"sensor address not a number", map[string]string{"SENSORS": "main:x"}},
		{"sensor address negative", map[string]string{"SENSORS": "main:-1"}},
		{"sensor address reserved low", map[string]string{"SENSORS": "main:0x03"}},
		{"sensor address beyond 7 bits", map[string]string{"SENSORS": "main:0x80"}},
		{"duplicate sensor names", map[string]string{"SENSORS": "a:0x44,a:0x45"}},
		{"duplicate addresses", map[string]string{"SENSORS": "a:0x44,b:68"}},
		{"trailing comma", map[string]string{"SENSORS": "a:0x44,"}},
		{"reserved sensor name", map[string]string{"SENSORS": "room:0x44"}},
		{"sensor name with comma-unsafe chars", map[string]string{"SENSORS": "a b:0x44"}},
		{"backup keep zero", map[string]string{"BACKUP_KEEP": "0"}},
		{"unknown time zone", map[string]string{"STUDIO_TZ": "Mars/Base"}},
		{"demo seed with real driver", map[string]string{"DEMO_SEED": "true"}},
		{"demo seed unknown value", map[string]string{"SENSOR_DRIVER": "sim", "DEMO_SEED": "maybe"}},
		{"bad clock check", map[string]string{"CLOCK_CHECK": "maybe"}},
		{"bad log level", map[string]string{"LOG_LEVEL": "chatty"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(env(tc.env)); err == nil {
				t.Error("Load() succeeded, want error")
			}
		})
	}
}
