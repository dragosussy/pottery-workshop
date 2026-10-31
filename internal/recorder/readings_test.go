package recorder

import (
	"context"
	"errors"
	"testing"
	"time"

	"humidity/internal/store"
)

func TestSeries_BucketsAndGaps(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	var rows []store.Minute
	for i := 0; i < 10; i++ {
		rows = append(rows, store.Minute{Sensor: "room", Minute: t0.Add(time.Duration(i) * time.Minute),
			HumAvg: 80 + float64(i), HumMin: 79 + float64(i), HumMax: 81 + float64(i), TempAvg: 18, VPDAvg: 0.4, N: 12})
	}
	store.InsertMinutes(ctx, st.DB(), rows)

	r, err := Series(ctx, st.DB(), []string{"room", "main"}, t0, t0.Add(15*time.Minute), 5*time.Minute, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.T) != 3 || r.T[0] != t0.Unix() || r.ResolutionSeconds != 300 {
		t.Fatalf("t = %v, res %d", r.T, r.ResolutionSeconds)
	}
	room := r.Series[0]
	if *room.HumidityAvg[0] != 82 || *room.HumidityMin[0] != 79 || *room.HumidityMax[1] != 90 || room.Samples[0] != 60 {
		t.Errorf("room bucket 0/1 = %v %v %v %v", *room.HumidityAvg[0], *room.HumidityMin[0], *room.HumidityMax[1], room.Samples[0])
	}
	if room.HumidityAvg[2] != nil || room.Samples[2] != 0 {
		t.Error("empty bucket should be null with 0 samples")
	}
	if main := r.Series[1]; main.Sensor != "main" || main.HumidityAvg[0] != nil {
		t.Errorf("series with no data = %+v", main)
	}
}

func TestBuckets_DayAlignsToLocalMidnightAcrossDST(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Bucharest")
	from := time.Date(2026, 10, 24, 15, 0, 0, 0, loc)
	to := time.Date(2026, 10, 27, 15, 0, 0, 0, loc)
	b, err := buckets(from, to, 24*time.Hour, loc, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 4 {
		t.Fatalf("buckets = %v", b)
	}
	for _, s := range b {
		if l := s.In(loc); l.Hour() != 0 || l.Minute() != 0 {
			t.Errorf("bucket %v not at local midnight", l)
		}
	}
	if d := b[2].Sub(b[1]); d != 25*time.Hour {
		t.Errorf("the DST day is %v, want 25h", d)
	}
}

func TestPickResolutionAndLimit(t *testing.T) {
	cases := map[time.Duration]time.Duration{
		6 * time.Hour:        time.Minute,     // 360 buckets
		24 * time.Hour:       5 * time.Minute, // 288
		7 * 24 * time.Hour:   time.Hour,       // 168
		90 * 24 * time.Hour:  6 * time.Hour,   // 360
		1000 * time.Hour:     6 * time.Hour,   // 167
		400 * 24 * time.Hour: 24 * time.Hour,  // 400 days: only 1d fits... falls back to 1d
	}
	for span, want := range cases {
		if got := PickResolution(t0, t0.Add(span), time.UTC); got != want {
			t.Errorf("span %v: resolution %v, want %v", span, got, want)
		}
	}
	_, err := Series(context.Background(), openStore(t).DB(), []string{"room"}, t0, t0.Add(48*time.Hour), time.Minute, time.UTC)
	if !errors.Is(err, ErrTooManyPoints) {
		t.Errorf("2880 one-minute buckets: err = %v, want ErrTooManyPoints", err)
	}
}
