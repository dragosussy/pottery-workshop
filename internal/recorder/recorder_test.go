package recorder

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"humidity/internal/monitor"
	"humidity/internal/predict"
	"humidity/internal/sensor"
	"humidity/internal/store"
)

type fakeNotifier struct {
	mu     sync.Mutex
	scopes [][]string
}

func (f *fakeNotifier) Changed(scopes []string, _ []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scopes = append(f.scopes, scopes)
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}

func newRecorder(t *testing.T, st *store.Store, synced func() bool) (*Recorder, *fakeNotifier) {
	n := &fakeNotifier{}
	idx := NewDoseIndex(time.Now)
	r := New(st, idx, n, synced, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return r, n
}

func cycle(at time.Time, sensors ...monitor.SensorStatus) monitor.Cycle {
	for i := range sensors {
		sensors[i].Reading.At = at
	}
	return monitor.Cycle{At: at, Sensors: sensors}
}

func ok(name string, h, t float64) monitor.SensorStatus {
	return monitor.SensorStatus{Sensor: name, OK: true, Reading: sensor.Reading{Sensor: name, Humidity: h, Temperature: t}}
}

func failing(name string, since time.Time) monitor.SensorStatus {
	return monitor.SensorStatus{Sensor: name, LastError: "boom", FailingSince: since}
}

var t0 = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func TestRecorder_AggregatesMinutesAndRoomMean(t *testing.T) {
	st := openStore(t)
	r, _ := newRecorder(t, st, func() bool { return true })
	ctx := context.Background()

	r.handle(ctx, cycle(t0.Add(5*time.Second), ok("a", 80, 18), ok("b", 84, 20)))
	r.handle(ctx, cycle(t0.Add(35*time.Second), ok("a", 82, 18), failing("b", t0.Add(30*time.Second))))
	// Nothing written while the minute is open.
	if last, _ := store.LastMinute(ctx, st.DB()); !last.IsZero() {
		t.Fatalf("minute written before it closed: %v", last)
	}
	r.handle(ctx, cycle(t0.Add(65*time.Second), ok("a", 70, 18)))

	room, err := store.MinutesBetween(ctx, st.DB(), "room", t0, t0.Add(time.Minute))
	if err != nil || len(room) != 1 {
		t.Fatalf("room minutes = %v, %v", room, err)
	}
	// Cycle 1 room = (80+84)/2 = 82, cycle 2 room = 82 (only a) → avg 82.
	if m := room[0]; m.N != 2 || m.HumAvg != 82 || m.HumMin != 82 || m.HumMax != 82 || m.TempAvg != 18.5 {
		t.Errorf("room minute = %+v", m)
	}
	a, _ := store.MinutesBetween(ctx, st.DB(), "a", t0, t0.Add(time.Minute))
	if m := a[0]; m.N != 2 || m.HumAvg != 81 || m.HumMin != 80 || m.HumMax != 82 {
		t.Errorf("sensor a minute = %+v", m)
	}
	wantVPD := (predict.VPD(18, 80) + predict.VPD(18, 82)) / 2
	if math.Abs(a[0].VPDAvg-wantVPD) > 1e-9 {
		t.Errorf("vpd = %v, want the mean of per-reading VPDs %v", a[0].VPDAvg, wantVPD)
	}
	b, _ := store.MinutesBetween(ctx, st.DB(), "b", t0, t0.Add(time.Minute))
	if len(b) != 1 || b[0].N != 1 {
		t.Errorf("sensor b minute = %+v, want only its good reading", b)
	}

	// Shutdown flushes the open (partial) minute.
	r.flush(ctx)
	if last, _ := store.LastMinute(ctx, st.DB()); !last.Equal(t0.Add(time.Minute)) {
		t.Errorf("last minute after flush = %v", last)
	}
}

func TestRecorder_ClockGuard(t *testing.T) {
	st := openStore(t)
	synced := false
	r, _ := newRecorder(t, st, func() bool { return synced })
	ctx := context.Background()

	r.handle(ctx, cycle(t0, ok("a", 80, 18)))
	r.handle(ctx, cycle(t0.Add(time.Minute), ok("a", 80, 18)))
	r.flush(ctx)
	if last, _ := store.LastMinute(ctx, st.DB()); !last.IsZero() {
		t.Fatalf("wrote %v while the clock was unsynced", last)
	}
	if r.ClockSynced() {
		t.Error("ClockSynced() = true while unsynced")
	}

	synced = true
	r.handle(ctx, cycle(t0.Add(2*time.Minute), ok("a", 80, 18)))
	r.handle(ctx, cycle(t0.Add(3*time.Minute), ok("a", 80, 18)))
	// The clock steps back: those readings are dropped, never written.
	r.handle(ctx, cycle(t0.Add(-time.Hour), ok("a", 10, 18)))
	r.handle(ctx, cycle(t0.Add(2*time.Minute), ok("a", 10, 18)))
	r.flush(ctx)

	rows, _ := store.MinutesBetween(ctx, st.DB(), "a", t0.Add(-2*time.Hour), t0.Add(time.Hour))
	if len(rows) != 2 || rows[0].HumAvg != 80 || rows[1].HumAvg != 80 {
		t.Errorf("rows = %+v, want minutes 2 and 3 at 80%% only", rows)
	}
}

func TestRecorder_OfflineDetection(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	lastOn := t0.Add(-9 * 24 * time.Hour)
	if err := store.InsertMinutes(ctx, st.DB(), []store.Minute{{Sensor: "room", Minute: lastOn, HumAvg: 80, VPDAvg: 0.4, N: 12}}); err != nil {
		t.Fatal(err)
	}

	r, n := newRecorder(t, st, func() bool { return true })
	r.handle(ctx, cycle(t0.Add(30*time.Second), ok("a", 80, 18)))

	entries, _ := store.LogEntries(ctx, st.DB())
	if len(entries) != 1 || entries[0].Kind != store.LogOffline {
		t.Fatalf("log = %+v, want one offline entry", entries)
	}
	var v struct{ From, To string }
	json.Unmarshal(entries[0].Value, &v)
	if v.From != "2026-10-21T12:01:00Z" || v.To != "2026-10-30T12:00:30Z" {
		t.Errorf("offline = %+v", v)
	}
	if len(n.scopes) != 1 {
		t.Errorf("notifications = %v, want one", n.scopes)
	}

	// A short restart is not an offline period.
	st2 := openStore(t)
	store.InsertMinutes(ctx, st2.DB(), []store.Minute{{Sensor: "room", Minute: t0.Add(-30 * time.Minute), VPDAvg: 0.4, N: 1}})
	r2, _ := newRecorder(t, st2, func() bool { return true })
	r2.handle(ctx, cycle(t0, ok("a", 80, 18)))
	if entries, _ := store.LogEntries(ctx, st2.DB()); len(entries) != 0 {
		t.Errorf("log after a 30-minute gap = %+v, want none", entries)
	}
}

func TestRecorder_SensorGap(t *testing.T) {
	st := openStore(t)
	r, _ := newRecorder(t, st, func() bool { return true })
	ctx := context.Background()

	r.handle(ctx, cycle(t0, failing("a", t0), ok("b", 80, 18)))
	r.handle(ctx, cycle(t0.Add(30*time.Minute), ok("a", 80, 18), ok("b", 80, 18))) // 30 min: not logged
	r.handle(ctx, cycle(t0.Add(40*time.Minute), failing("a", t0.Add(40*time.Minute)), ok("b", 80, 18)))
	r.handle(ctx, cycle(t0.Add(3*time.Hour), ok("a", 80, 18), ok("b", 80, 18)))

	entries, _ := store.LogEntries(ctx, st.DB())
	if len(entries) != 1 || entries[0].Kind != store.LogSensorGap {
		t.Fatalf("log = %+v, want one sensor_gap", entries)
	}
	var v struct{ Sensor, From, To string }
	json.Unmarshal(entries[0].Value, &v)
	if v.Sensor != "a" || v.From != "2026-10-30T12:40:00Z" || v.To != "2026-10-30T15:00:00Z" {
		t.Errorf("gap = %+v", v)
	}
}

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %.6f, want %.6f", name, got, want)
	}
}

func TestDoseIndex(t *testing.T) {
	now := t0.Add(48 * time.Hour)
	d := NewDoseIndex(func() time.Time { return now })
	r1 := predict.EffRate(0.4)
	r2 := predict.EffRate(0.8)

	// 24 h at 0.4 kPa, then a 30-minute gap, then 1 h at 0.8, then a 10 h
	// gap (device off), then 1 h at 0.8.
	m := t0
	for i := 0; i < 24*60; i++ {
		d.Append(m, 0.4)
		m = m.Add(time.Minute)
	}
	gap1 := m
	m = m.Add(30 * time.Minute)
	for i := 0; i < 60; i++ {
		d.Append(m, 0.8)
		m = m.Add(time.Minute)
	}
	off := m
	m = m.Add(10 * time.Hour)
	back := m
	for i := 0; i < 60; i++ {
		d.Append(m, 0.8)
		m = m.Add(time.Minute)
	}
	end := m

	eff, filled := d.Dose(t0, t0.Add(24*time.Hour))
	approx(t, "first day", eff, 24*r1)
	approx(t, "first day filled", filled, 0)

	// A short gap carries the last VPD forward and isn't "filled".
	eff, filled = d.Dose(gap1, gap1.Add(30*time.Minute))
	approx(t, "short gap", eff, 0.5*r1)
	approx(t, "short gap filled", filled, 0)

	// A long gap is filled with the mean VPD of the 24 h before it, whose
	// recorded minutes are 22.5 h at 0.4 and 1 h at 0.8.
	mean := (0.4*float64(23*60-30) + 0.8*60) / float64(24*60-30)
	eff, filled = d.Dose(off, back)
	approx(t, "long gap", eff, 10*predict.EffRate(mean))
	approx(t, "long gap filled", filled, eff)

	// Partial minutes interpolate.
	eff, _ = d.Dose(t0.Add(30*time.Second), t0.Add(90*time.Second))
	approx(t, "partial minutes", eff, r1/60)

	// Additivity across the whole range.
	a, _ := d.Dose(t0, off)
	b, _ := d.Dose(off, end)
	c, _ := d.Dose(t0, end)
	approx(t, "additive", a+b, c)

	// The open tail: under an hour since the last minute carries forward;
	// longer is filled.
	now = end.Add(30 * time.Minute)
	eff, filled = d.Dose(end, now)
	approx(t, "short tail", eff, 0.5*r2)
	approx(t, "short tail filled", filled, 0)
	now = end.Add(3 * time.Hour)
	_, filled = d.Dose(end, now)
	if filled <= 0 {
		t.Error("a 3 h tail should count as filled")
	}

	// Before the first minute: filled with the first day's mean.
	eff, filled = d.Dose(t0.Add(-2*time.Hour), t0)
	approx(t, "before first", eff, 2*r1)
	approx(t, "before first filled", filled, eff)

	if eff, _ := d.Dose(end, t0); eff != 0 {
		t.Errorf("reversed range = %v, want 0", eff)
	}
}

func TestDoseIndex_LoadMatchesAppend(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	var rows []store.Minute
	live := NewDoseIndex(time.Now)
	for i := 0; i < 300; i++ {
		m := t0.Add(time.Duration(i) * time.Minute)
		if i > 100 && i < 250 {
			continue // a 2.5 h gap
		}
		v := 0.3 + float64(i%7)/20
		rows = append(rows, store.Minute{Sensor: "room", Minute: m, VPDAvg: v, N: 1})
		live.Append(m, v)
	}
	store.InsertMinutes(ctx, st.DB(), rows)
	loaded := NewDoseIndex(time.Now)
	if err := loaded.Load(ctx, st.DB()); err != nil {
		t.Fatal(err)
	}
	a, af := live.Dose(t0, t0.Add(5*time.Hour))
	b, bf := loaded.Dose(t0, t0.Add(5*time.Hour))
	approx(t, "loaded dose", b, a)
	approx(t, "loaded filled", bf, af)
}
