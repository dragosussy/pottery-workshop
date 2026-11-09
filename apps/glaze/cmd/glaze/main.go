// Command glaze runs the glaze notebook. It is configured through the
// environment; see docs/apps/glaze/design.md.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"atelier/apps/glaze/internal/app"
	"atelier/apps/glaze/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dataDir := env("DATA_DIR", "/data")
	db, err := app.OpenDB(dataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	a := &app.App{DB: db, DataDir: dataDir, Web: web.Dist()}

	var wg sync.WaitGroup
	wg.Go(func() { a.SnapshotLoop(ctx) })

	srv := &http.Server{Addr: env("ADDR", ":8080"), Handler: a.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Printf("listening on %s (data in %s)", srv.Addr, dataDir)
	err = srv.ListenAndServe()
	stop() // if the server failed, stop the snapshots too
	wg.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		log.Print("stopped")
		return nil
	}
	return err
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
