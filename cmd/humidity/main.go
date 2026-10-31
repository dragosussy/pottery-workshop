// Command humidity monitors and controls (with a pump) the humidity of the
// pottery drying space, records its climate, tracks the pieces drying on the
// shelf, and serves the API and the web app.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "time/tzdata" // studio time zones work in a minimal container

	"golang.org/x/sync/errgroup"

	"humidity/internal/api"
	"humidity/internal/config"
	"humidity/internal/control"
	"humidity/internal/demo"
	"humidity/internal/monitor"
	"humidity/internal/photos"
	"humidity/internal/recorder"
	"humidity/internal/shelf"
	"humidity/internal/store"
	"humidity/web"
)

// Set at build time with -ldflags "-X main.version=… -X main.commit=… -X main.builtAt=…".
var (
	version = "dev"
	commit  = "unknown"
	builtAt = ""
)

func main() {
	level := new(slog.LevelVar)
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	if err := run(log, level); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, level *slog.LevelVar) error {
	// docker stop and the Pi's power button send SIGTERM; handle it so the
	// recorder flushes the open minute and the pump switches off.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startedAt := time.Now()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	level.Set(cfg.LogLevel)

	// Store: a backup before migrations, then migrate.
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer st.Close()
	backups := &store.Backups{Store: st, Dir: cfg.BackupDir, Keep: cfg.BackupKeep, Log: log, Now: time.Now}
	if err := backups.AtStartup(ctx); err != nil {
		return fmt.Errorf("startup backup: %w", err)
	}
	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	sensors, err := openSensors(cfg)
	if err != nil {
		return err
	}
	defer closeSensors(sensors)
	names := make([]string, len(cfg.Sensors))
	for i, s := range cfg.Sensors {
		names[i] = s.Name
	}

	mon := monitor.New(sensors, cfg.PollInterval, log)
	hub := api.NewHub()
	dose := recorder.NewDoseIndex(time.Now)
	svc := shelf.New(st, dose, hub, mon, names, cfg.StudioTZ)
	ph, err := photos.New(st, cfg.PhotoDir, log)
	if err != nil {
		return fmt.Errorf("photos: %w", err)
	}

	if cfg.DemoSeed != config.DemoOff {
		if err := demo.Seed(ctx, demo.Deps{Store: st, Shelf: svc, Photos: ph, Dose: dose, Sensors: names, Mode: cfg.DemoSeed, Log: log}); err != nil {
			return fmt.Errorf("demo seed: %w", err)
		}
	}
	if err := dose.Load(ctx, st.DB()); err != nil {
		return fmt.Errorf("dose index: %w", err)
	}
	offsets, err := svc.Offsets(ctx)
	if err != nil {
		return fmt.Errorf("offsets: %w", err)
	}
	mon.SetOffsets(offsets)

	synced := recorder.ClockSynced
	if !cfg.ClockCheck {
		synced = func() bool { return true }
	}
	rec := recorder.New(st, dose, hub, synced, log)
	recCycles, cancelRec := mon.Subscribe(16)
	defer cancelRec()
	streamCycles, cancelStream := mon.Subscribe(4)
	defer cancelStream()

	// The pump: off from the moment it is opened, and on shutdown.
	sw, err := openPump(cfg.Pump, sensors)
	if err != nil {
		return err
	}
	var ctl *control.Controller
	if sw != nil {
		defer sw.Close()
		ctl = control.New(sw, controlConfig(cfg.Pump), shelf.ControlEvents{S: svc}, log)
		target, err := svc.Target(ctx)
		if err != nil {
			return fmt.Errorf("target: %w", err)
		}
		ctl.SetTarget(target)
		svc.OnTarget = ctl.SetTarget
	}
	var ctlCycles <-chan monitor.Cycle
	if ctl != nil {
		var cancelCtl func()
		ctlCycles, cancelCtl = mon.Subscribe(4)
		defer cancelCtl()
	}

	srv, handler := api.New(api.Deps{
		Store: st, Monitor: mon, Shelf: svc, Control: ctl, Photos: ph, Hub: hub,
		ClockSynced: rec.ClockSynced, Web: web.Dist(), TempDir: filepath.Dir(cfg.DBPath),
		Version: version, Commit: commit, BuiltAt: builtAt, StartedAt: startedAt, Log: log,
	})
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	httpSrv.RegisterOnShutdown(hub.Close)

	log.Info("started",
		"version", version,
		"driver", cfg.Driver,
		"pump", cfg.Pump.Driver,
		"sensors", len(sensors),
		"interval", cfg.PollInterval,
		"addr", cfg.HTTPAddr,
		"db", cfg.DBPath,
		"frontend_embedded", web.Embedded,
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return mon.Run(gctx) })
	g.Go(func() error { return rec.Run(gctx, recCycles) })
	if ctl != nil {
		g.Go(func() error { return ctl.Run(gctx, ctlCycles) })
	}
	g.Go(func() error { return srv.RunStream(gctx, streamCycles) })
	g.Go(func() error { return backups.RunDaily(gctx) })
	g.Go(func() error { return ph.RunCleanup(gctx) })
	g.Go(func() error {
		if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(sctx)
	})
	return g.Wait()
}
