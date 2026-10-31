package predict

import (
	"math"
	"testing"
	"time"
)

func TestPieceSample_Interpolation(t *testing.T) {
	d := constDose{rate: 0.5}
	// 1.0 at h0, 2.6 at h20, 3.4 at h30: crosses 3.0 halfway, at h25.
	p := mug(1, obs(h(0), 1.0, SourceAdd), obs(h(20), 2.6, SourceUser), obs(h(30), 3.4, SourceUser))
	s, ok := PieceSample(p, d)
	if !ok {
		t.Fatal("no sample")
	}
	if !s.ReachedAt.Equal(h(25)) {
		t.Errorf("reached at %v, want %v", s.ReachedAt, h(25))
	}
	// 2 units over 12.5 eff-h.
	if math.Abs(s.Rate-2/12.5) > 1e-9 {
		t.Errorf("rate = %v, want %v", s.Rate, 2/12.5)
	}
	// Gap 5 eff-h over 12.5: weight 1 − 0.5·0.4.
	if math.Abs(s.Weight-0.8) > 1e-9 {
		t.Errorf("weight = %v, want 0.8", s.Weight)
	}
}

func TestPieceSample_Inclusion(t *testing.T) {
	d := constDose{rate: 0.5}
	normal := func() Piece {
		return mug(1, obs(h(0), 1.0, SourceAdd), obs(h(20), 2.8, SourceUser), obs(h(26), 3.1, SourceEnd))
	}
	tests := []struct {
		name  string
		piece func() Piece
		dose  Doser
		want  bool
	}{
		{"normal, reached by trim", normal, d, true},
		{"discarded after reaching 3.0", func() Piece {
			p := mug(1, obs(h(0), 1.0, SourceAdd), obs(h(20), 2.8, SourceUser), obs(h(26), 3.1, SourceUser))
			return p
		}, d, true},
		{"trimmed early (covered at 2.4)", func() Piece {
			return mug(1, obs(h(0), 1.0, SourceAdd), obs(h(20), 2.4, SourceEnd))
		}, d, false},
		{"added in the band", func() Piece {
			return mug(1, obs(h(0), 2.8, SourceAdd), obs(h(5), 3.2, SourceUser))
		}, d, false},
		{"add observation deleted", func() Piece {
			return mug(1, obs(h(20), 2.8, SourceUser), obs(h(26), 3.1, SourceEnd))
		}, d, false},
		{"inconsistent observation", func() Piece {
			return mug(1, obs(h(0), 1.0, SourceAdd), obs(h(10), 2.2, SourceUser), obs(h(12), 1.6, SourceUser), obs(h(26), 3.1, SourceEnd))
		}, d, false},
		{"skipped follow-up", func() Piece { p := normal(); p.Unknown = true; return p }, d, false},
		{"spans an offline stretch", normal, constDose{rate: 0.5, filled: []Interval{{h(5), h(10)}}}, false},
		{"crossing poorly bracketed", func() Piece {
			return mug(1, obs(h(0), 1.0, SourceAdd), obs(h(2), 1.2, SourceUser), obs(h(30), 3.4, SourceEnd))
		}, d, false},
	}
	for _, tc := range tests {
		if _, ok := PieceSample(tc.piece(), tc.dose); ok != tc.want {
			t.Errorf("%s: sample = %v, want %v", tc.name, ok, tc.want)
		}
	}
}

func TestPieceSample_MedianWithin30Minutes(t *testing.T) {
	d := constDose{rate: 0.5}
	// Two people check 10 minutes apart; one says 3.4, the other 2.6. The
	// raw pair looks 0.8 wetter, but training sees their median, 3.0.
	p := mug(1, obs(h(0), 1.0, SourceAdd), obs(h(10), 2.0, SourceUser),
		obs(h(24), 3.4, SourceUser), obs(h(24).Add(10*time.Minute), 2.6, SourceUser), obs(h(30), 3.5, SourceEnd))
	s, ok := PieceSample(p, d)
	if !ok {
		t.Fatal("no sample")
	}
	if d := s.ReachedAt.Sub(h(24)); d < 0 || d > 10*time.Minute {
		t.Errorf("reached at %v, want the median check around h24", s.ReachedAt)
	}
}

func TestModel_GroupsCountOnceAndOutliersDropped(t *testing.T) {
	d := constDose{rate: 0.5}
	var pieces []Piece
	// A group of 4 identical mugs counts as one sample.
	for i := 0; i < 4; i++ {
		p := mug(int64(10+i), obs(h(0), 1.0, SourceAdd), obs(h(20), 2.8, SourceUser), obs(h(26), 3.1, SourceEnd))
		p.GroupID = 10
		pieces = append(pieces, p)
	}
	m := NewModel(pieces, d)
	if m.Samples != 1 {
		t.Errorf("samples = %d, want 1 for one group", m.Samples)
	}
	// Five ordinary pieces and one that was covered but never marked
	// (crawled to 3.0 over 20 days): the outlier is rejected.
	pieces = history(5, 24)
	slow := mug(99, obs(h(-1000), 1.0, SourceAdd), obs(h(-530), 2.8, SourceUser), obs(h(-500), 3.1, SourceEnd))
	pieces = append(pieces, slow)
	m = NewModel(pieces, d)
	if m.Samples != 5 {
		t.Errorf("samples = %d, want 5 after rejecting the outlier", m.Samples)
	}
}
