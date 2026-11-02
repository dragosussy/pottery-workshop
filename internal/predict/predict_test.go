package predict

import (
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func h(n float64) time.Time { return t0.Add(time.Duration(n * float64(time.Hour))) }

// piece builds a medium/medium piece (prior 1/12 per hour) from
// (hours after t0, dryness) pairs.
func piece(id int64, checks ...float64) Piece {
	p := Piece{ID: id, Size: "medium", Thickness: "medium"}
	for i := 0; i < len(checks); i += 2 {
		p.Checks = append(p.Checks, Check{At: h(checks[i]), Dryness: checks[i+1]})
	}
	return p
}

func trimmed(p Piece) Piece {
	p.Ended, p.Trimmed = true, true
	return p
}

// ref is a shelf held at the reference humidity, with no readings: rates
// are plain dryness per hour.
var ref = Climate{Ahead: RefRH}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPrior(t *testing.T) {
	for _, tc := range []struct {
		size, thickness string
		want            float64
	}{
		{"small", "thin", 1 / 4.5},
		{"medium", "medium", 1.0 / 12},
		{"large", "thick", 1 / (24 * 1.4)},
		{"xlarge", "thick", 1 / (24 * 1.9)},
	} {
		if got := Prior(tc.size, tc.thickness); !near(got, tc.want) {
			t.Errorf("Prior(%s, %s) = %v, want %v", tc.size, tc.thickness, got, tc.want)
		}
	}
}

func TestStudioFactor(t *testing.T) {
	// Twice the prior: 1.0 → 3.0 in 12 h at a prior of 1/12 per hour.
	fast := trimmed(piece(1, 0, 1.0, 12, 3.0))
	discarded := piece(2, 0, 1.0, 12, 3.0)
	discarded.Ended = true

	for _, tc := range []struct {
		name   string
		pieces []Piece
		k      float64
		n      int
	}{
		{"no samples", nil, 1, 0},
		{"one sample, shrunk towards 1", []Piece{fast}, math.Sqrt2, 1},
		{"two samples", []Piece{fast, trimmed(piece(3, 0, 1.0, 12, 3.0))}, math.Pow(2, 2.0/3), 2},
		{"on the shelf is not a sample", []Piece{piece(4, 0, 1.0, 12, 3.0)}, 1, 0},
		{"discarded is not a sample", []Piece{discarded}, 1, 0},
		{"first check already 2.8", []Piece{trimmed(piece(5, 0, 2.8, 12, 3.0))}, 1, 0},
		{"last check below 2.8", []Piece{trimmed(piece(6, 0, 1.0, 12, 2.7))}, 1, 0},
		{"last check exactly 2.8", []Piece{trimmed(piece(7, 0, 1.6, 12, 2.8))}, math.Sqrt(1.2), 1},
		{"under an hour apart", []Piece{trimmed(piece(8, 0, 1.0, 0.9, 3.0))}, 1, 0},
		{"single check", []Piece{trimmed(piece(9, 0, 3.0))}, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, n := StudioFactor(tc.pieces, ref)
			if !near(k, tc.k) || n != tc.n {
				t.Errorf("StudioFactor() = %v, %d; want %v, %d", k, n, tc.k, tc.n)
			}
		})
	}
}

func TestRate(t *testing.T) {
	base := 1.0 / 12
	for _, tc := range []struct {
		name string
		p    Piece
		k    float64
		want float64
	}{
		{"one check", piece(1, 0, 1.0), 1, base},
		{"k scales the prior", piece(1, 0, 1.0), 2, 2 * base},
		{"no movement", piece(1, 0, 1.0, 24, 1.0), 1, base},
		{"checks going down", piece(1, 0, 2.0, 24, 1.5), 1, base},
		{"under an hour apart", piece(1, 0, 1.0, 0.5, 2.0), 1, base},
		// 12 h watched: w = 1, own = 2·base → base · 2^(1/3)
		{"blend after 12 h", piece(1, 0, 1.0, 12, 3.0), 1, base * math.Cbrt(2)},
		// 96 h watched: w is capped at 6, own = 2·base → base · 2^(6/8)
		{"weight capped at 6", piece(1, 0, 1.0, 96, 2.6), 0.1, 0.1 * base * math.Pow(2, 0.75)},
		// own = 100·base is clamped to 10·base
		{"own slope clamped", piece(1, 0, 1.0, 12, 2.0), 0.01, 0.01 * base * math.Cbrt(10)},
		{"middle checks don't matter", piece(1, 0, 1.0, 6, 0.5, 12, 3.0), 1, base * math.Cbrt(2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Rate(tc.p, tc.k, ref); !near(got, tc.want) {
				t.Errorf("Rate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRun(t *testing.T) {
	at := func(hours float64) *time.Time { x := h(hours); return &x }
	ended := piece(9, 0, 1.0)
	ended.Ended = true

	// One check each, k = 1: every piece dries at 1/12 per hour.
	for _, tc := range []struct {
		name string
		p    Piece
		now  float64 // hours after t0
		want Estimate
	}{
		{"drying", piece(1, 0, 1.0), 12, Estimate{Dryness: 2.0, Status: "drying", ReadyAt: at(24), ReadyUntil: at(31.2)}},
		{"ready at 2.8", piece(1, 0, 2.8), 0, Estimate{Dryness: 2.8, Status: "ready", ReadyUntil: at(9.6)}},
		{"ready", piece(1, 0, 2.5), 6, Estimate{Dryness: 3.0, Status: "ready", ReadyUntil: at(13.2)}},
		{"ready at 3.6", piece(1, 0, 3.6), 0, Estimate{Dryness: 3.6, Status: "ready", ReadyUntil: at(0)}},
		{"past", piece(1, 0, 3.0), 12, Estimate{Dryness: 4.0, Status: "past"}},
		{"capped at 5", piece(1, 0, 4.5), 24, Estimate{Dryness: 5, Status: "past"}},
		{"counts from the last check", piece(1, 0, 1.0, 1, 1.0), 13, Estimate{Dryness: 2.0, Status: "drying", ReadyAt: at(25), ReadyUntil: at(32.2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			est, n := Run([]Piece{tc.p, ended}, h(tc.now), ref)
			if n != 0 {
				t.Errorf("n = %d, want 0", n)
			}
			if _, ok := est[9]; ok || len(est) != 1 {
				t.Errorf("ended piece got an estimate: %v", est)
			}
			got := est[1]
			if !near(got.Dryness, tc.want.Dryness) || got.Status != tc.want.Status ||
				!sameTime(got.ReadyAt, tc.want.ReadyAt) || !sameTime(got.ReadyUntil, tc.want.ReadyUntil) {
				t.Errorf("Run() = %+v %v %v, want %+v %v %v", got, got.ReadyAt, got.ReadyUntil, tc.want, tc.want.ReadyAt, tc.want.ReadyUntil)
			}
		})
	}
}

func TestRunUsesLearnedFactor(t *testing.T) {
	// Five samples drying twice as fast as the prior: k = 2^(5/6).
	var pieces []Piece
	for i := range 5 {
		pieces = append(pieces, trimmed(piece(int64(10+i), 0, 1.0, 12, 3.0)))
	}
	pieces = append(pieces, piece(1, 0, 1.0))
	est, n := Run(pieces, t0, ref)
	if n != 5 {
		t.Fatalf("n = %d, want 5", n)
	}
	rate := math.Pow(2, 5.0/6) / 12
	want := h(2 / rate).Truncate(time.Second)
	if !est[1].ReadyAt.Equal(want) {
		t.Errorf("ReadyAt = %v, want %v", est[1].ReadyAt, want)
	}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func TestSpeed(t *testing.T) {
	for _, tc := range []struct{ rh, want float64 }{
		{80, 1}, {90, 0.5}, {60, 2}, {100, 0.05}, {120, 0.05},
	} {
		if got := Speed(tc.rh); !near(got, tc.want) {
			t.Errorf("Speed(%v) = %v, want %v", tc.rh, got, tc.want)
		}
	}
}

func TestRunFollowsTheTarget(t *testing.T) {
	at := func(hours float64) *time.Time { x := h(hours); return &x }
	p := piece(1, 0, 1.0)
	// At 80 % the piece needs 24 h to reach 3.0. At 90 % the air is half as
	// dry, so from now on it dries at half the speed.
	for _, tc := range []struct {
		ahead float64
		want  *time.Time
	}{
		{80, at(24)},
		{90, at(12 + 24)}, // 12 h done at 80 % (dryness 2.0), then 1.0 at 1/24 per hour
		{60, at(12 + 6)},  // twice as fast
	} {
		c := Climate{Ahead: tc.ahead, MeanRH: func(from, to time.Time) (float64, bool) { return 80, true }}
		est, _ := Run([]Piece{p}, h(12), c)
		if got := est[1]; !near(got.Dryness, 2.0) || !sameTime(got.ReadyAt, tc.want) {
			t.Errorf("ahead %v %%: dryness %v, ready %v; want 2.0, %v", tc.ahead, got.Dryness, got.ReadyAt, tc.want)
		}
	}
}

func TestRecordedHumidityIsNormalised(t *testing.T) {
	// Five samples that dried at the prior's pace while the shelf was at 90 %:
	// at the reference humidity that's twice the prior.
	var pieces []Piece
	for i := range 5 {
		pieces = append(pieces, trimmed(piece(int64(10+i), 0, 1.0, 24, 3.0)))
	}
	at90 := Climate{Ahead: 90, MeanRH: func(from, to time.Time) (float64, bool) { return 90, true }}
	k, n := StudioFactor(pieces, at90)
	if want := math.Pow(2, 5.0/6); n != 5 || !near(k, want) {
		t.Errorf("k = %v (n %d), want %v", k, n, want)
	}
	// Without readings the expected humidity stands in.
	if k2, _ := StudioFactor(pieces, Climate{Ahead: 90}); !near(k2, k) {
		t.Errorf("k without readings = %v, want %v", k2, k)
	}
}
