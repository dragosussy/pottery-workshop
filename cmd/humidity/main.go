// Command humidity runs the drying shelf: it reads the humidity sensors,
// runs the misting pump, keeps the list of pieces and serves the web app.
// It is configured through the environment; see docs/plan.md §7.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"humidity/internal/app"
	"humidity/internal/hw"
	"humidity/web"
)

const i2cBus = "/dev/i2c-1"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// docker stop and the Pi's power button send SIGTERM: catch it so the
	// deferred pump-off below runs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dataDir := env("DATA_DIR", "/data")
	pulse, err := time.ParseDuration(env("PUMP_PULSE", "10s"))
	if err != nil {
		return fmt.Errorf("PUMP_PULSE: %w", err)
	}
	rest, err := time.ParseDuration(env("PUMP_REST", "3m"))
	if err != nil {
		return fmt.Errorf("PUMP_REST: %w", err)
	}
	maxRH, err := strconv.Atoi(env("PUMP_MAX_RH", "95"))
	if err != nil {
		return fmt.Errorf("PUMP_MAX_RH: %w", err)
	}

	// The pump first: a sim sensor reacts to the sim pump.
	var sw hw.Switch
	var simPump *hw.SimPump
	switch kind := env("PUMP", "none"); kind {
	case "none":
	case "sim":
		simPump = &hw.SimPump{}
		sw = simPump
	case "gpio":
		pin, err := strconv.Atoi(os.Getenv("PUMP_PIN"))
		if err != nil {
			return fmt.Errorf("PUMP=gpio needs PUMP_PIN, the BCM GPIO number: %w", err)
		}
		// No default: a wrong guess runs the pump while the app shows it off.
		activeLow, err := strconv.ParseBool(os.Getenv("PUMP_ACTIVE_LOW"))
		if err != nil {
			return fmt.Errorf("PUMP=gpio needs PUMP_ACTIVE_LOW=true or false: %w", err)
		}
		gpio, err := hw.OpenGPIOPump(pin, activeLow)
		if err != nil {
			return err
		}
		sw = gpio
	default:
		return fmt.Errorf("PUMP must be none, gpio or sim, not %q", kind)
	}
	if sw != nil {
		defer sw.Set(false) // again, whatever happened to the pump loop
	}

	var addrs []string
	var sensors []hw.Sensor
	kind := env("SENSOR", "sht4x")
	for _, s := range strings.Split(env("SENSOR_ADDRS", "0x44"), ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(s), 0, 7)
		if err != nil {
			return fmt.Errorf("SENSOR_ADDRS: %q is not an I²C address like 0x44", s)
		}
		addr := fmt.Sprintf("%#x", n)
		switch kind {
		case "sim":
			sensors = append(sensors, hw.NewSimSensor(simPump))
		case "sht3x", "sht4x":
			v := hw.SHT4x
			if kind == "sht3x" {
				v = hw.SHT3x
			}
			sht, err := hw.OpenSHT(v, i2cBus, uint16(n))
			if err != nil {
				return fmt.Errorf("sensor %s: %w", addr, err)
			}
			// Carry on without it: the app shows the read errors, and
			// readings start once the sensor is connected.
			if err := sht.Reset(); err != nil {
				log.Printf("sensor %s: %v", addr, err)
			}
			sensors = append(sensors, sht)
		default:
			return fmt.Errorf("SENSOR must be sht4x, sht3x or sim, not %q", kind)
		}
		addrs = append(addrs, addr)
	}

	db, err := app.OpenDB(dataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	live, err := app.NewLive(db, addrs)
	if err != nil {
		return err
	}
	a := &app.App{DB: db, DataDir: dataDir, MaxRH: maxRH, Live: live, Web: web.Dist()}

	var wg sync.WaitGroup
	if sw != nil {
		a.Pump = app.NewPump(sw, live, pulse, rest, maxRH)
		wg.Go(func() { a.Pump.Loop(ctx) })
	}
	wg.Go(func() { a.SensorLoop(ctx, sensors) })

	screen, err := openScreen(dataDir)
	if err != nil {
		return err
	}
	if screen != nil {
		wg.Go(func() { a.DisplayLoop(ctx, screen) })
	}

	srv := &http.Server{Addr: env("ADDR", ":8080"), Handler: a.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Printf("listening on %s (sensors %s %v, pump %s, screen %s, data in %s)",
		srv.Addr, kind, addrs, env("PUMP", "none"), env("SCREEN", "none"), dataDir)
	err = srv.ListenAndServe()
	stop() // if the server failed, stop the loops too
	wg.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		log.Print("stopped")
		return nil
	}
	return err
}

// openScreen opens the small screen next to the Pi, or returns nil for none.
func openScreen(dataDir string) (hw.Screen, error) {
	switch kind := env("SCREEN", "none"); kind {
	case "none":
		return nil, nil
	case "sim":
		return hw.PNGScreen{Path: filepath.Join(dataDir, "screen.png")}, nil
	case "st7789":
		pin := func(key, def string) (int, error) {
			n, err := strconv.Atoi(env(key, def))
			if err != nil {
				return 0, fmt.Errorf("%s: want a BCM GPIO number: %w", key, err)
			}
			return n, nil
		}
		dc, err := pin("SCREEN_DC", "25")
		if err != nil {
			return nil, err
		}
		rst, err := pin("SCREEN_RST", "27")
		if err != nil {
			return nil, err
		}
		bl, err := pin("SCREEN_BL", "18")
		if err != nil {
			return nil, err
		}
		flip, err := strconv.ParseBool(env("SCREEN_FLIP", "false"))
		if err != nil {
			return nil, fmt.Errorf("SCREEN_FLIP: %w", err)
		}
		return hw.OpenST7789("/dev/spidev0.0", dc, rst, bl, flip)
	default:
		return nil, fmt.Errorf("SCREEN must be none, st7789 or sim, not %q", kind)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
