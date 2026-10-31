package recorder_test

import (
	"math"
	"testing"
	"time"

	"humidity/internal/predict"
	"humidity/internal/recorder"
)

// BenchmarkPlan predicts a busy shelf against a year of minutes and 500
// finished pieces: the worst case the Pi should see.
func BenchmarkPlan(b *testing.B) {
	start := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	now := start.Add(365 * 24 * time.Hour)
	idx := recorder.NewDoseIndex(func() time.Time { return now })
	for m := start; m.Before(now); m = m.Add(time.Minute) {
		if m.YearDay()%10 == 0 { // the device is off one day in ten
			continue
		}
		h := float64(m.Unix()) / 3600
		idx.Append(m, 0.4+0.1*math.Sin(h/24*2*math.Pi))
	}
	var pieces []predict.Piece
	for i := 0; i < 520; i++ {
		added := start.Add(time.Duration(i) * 16 * time.Hour)
		p := predict.Piece{ID: int64(i + 1), Number: int64(i%30 + 1), Form: "mug", Size: "medium", Thickness: "medium", AddedAt: added,
			Obs: []predict.Obs{{Dryness: 1, At: added, Source: "add"}, {Dryness: 2.6, At: added.Add(20 * time.Hour), Source: "user"},
				{Dryness: 3.1, At: added.Add(26 * time.Hour), Source: "end"}}}
		p.Ended = i < 500
		if !p.Ended {
			p.Obs = p.Obs[:1]
		}
		pieces = append(pieces, p)
	}
	loc, _ := time.LoadLocation("Europe/Bucharest")
	b.ResetTimer()
	for b.Loop() {
		predict.Plan(predict.Input{Now: now, Pieces: pieces, Dose: idx, Loc: loc})
	}
}
