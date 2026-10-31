package demo

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"humidity/internal/config"
	"humidity/internal/photos"
	"humidity/internal/recorder"
	"humidity/internal/shelf"
	"humidity/internal/store"
)

type nop struct{}

func (nop) Changed([]string, []int64)     {}
func (nop) SetOffsets(map[string]float64) {}

// Every state the web app can show must be reachable from the seed.
func TestSeed_ReachesEveryState(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Bucharest")
	y, m, d := time.Now().In(loc).Date()
	for _, mode := range []string{config.DemoLearning, config.DemoCalibrated} {
		// At several times of day, so no state depends on the hour.
		for _, hh := range []int{0, 7, 12, 18, 23} {
			now := time.Date(y, m, d, hh, 30, 0, 0, loc)
			t.Run(fmt.Sprintf("%s/%02d:30", mode, hh), func(t *testing.T) { seedAndCheck(t, mode, now) })
		}
	}
}

func seedAndCheck(t *testing.T, mode string, now time.Time) {
	clock := func() time.Time { return now }
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.Migrate(ctx)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	dose := recorder.NewDoseIndex(clock)
	svc := shelf.New(st, dose, nop{}, nop{}, []string{"main"}, "Europe/Bucharest")
	svc.Now = clock
	ph, _ := photos.New(st, filepath.Join(dir, "photos"), log)
	ph.Now = clock
	d := Deps{Store: st, Shelf: svc, Photos: ph, Dose: dose, Sensors: []string{"main"}, Mode: mode, Log: log, Now: clock}
	if err := Seed(ctx, d); err != nil {
		t.Fatal(err)
	}

	sh, err := svc.Shelf(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	group := false
	for _, p := range sh.Items {
		seen[p.Prediction.Status] = true
		group = group || p.GroupSize == 4
	}
	for _, want := range []string{"drying", "probably_ready_check", "ready_now", "may_be_past_check", "past_trim"} {
		if !seen[want] {
			t.Errorf("no piece is %s (have %v)", want, seen)
		}
	}
	if !group || sh.NextVisit == nil || len(sh.CheckThese) == 0 {
		t.Errorf("group=%v next_visit=%v checks=%d", group, sh.NextVisit, len(sh.CheckThese))
	}
	var restart bool
	for _, c := range sh.CheckThese {
		restart = restart || slices.Contains(c.Reasons, "not_checked_since_restart")
	}
	if !restart {
		t.Error("no piece left on the shelf across the offline period")
	}

	status, _ := svc.Status(ctx)
	wantState := map[string]string{config.DemoLearning: "learning", config.DemoCalibrated: "calibrated"}[mode]
	if status.Learning.State != wantState || status.LastOffline == nil {
		t.Errorf("status = %+v", status)
	}
	ended, _ := svc.ListPieces(ctx, false, "", 50)
	if len(ended.Items) < 3 {
		t.Errorf("only %d finished pieces", len(ended.Items))
	}

	// A second run leaves the database alone.
	if err := Seed(ctx, d); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.CountPieces(ctx, st.DB()); n != len(sh.Items)+len(ended.Items)+1 { // +1 mistake
		t.Errorf("pieces after a second seed = %d", n)
	}
}
