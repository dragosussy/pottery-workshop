package predict

import (
	"slices"
	"testing"
	"time"
)

// A Wednesday to plan from (UTC).
var wed = time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

func at(day int, hh, mm int) time.Time {
	return wed.Add(time.Duration(day)*24*time.Hour + time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute)
}

func cand(id int64, low, eta, until time.Time) Candidate {
	return Candidate{ID: id, ETALow: low, ETA: eta, ReadyUntil: until}
}

func TestPlanVisit(t *testing.T) {
	type want struct {
		visit       bool
		recommended time.Time
		start, end  time.Time
		deadline    time.Time
		score       float64
		ready       []int64
		maybe       []int64
		notYet      []int64
	}
	tests := []struct {
		name  string
		now   time.Time
		cands []Candidate
		want  want
	}{
		{
			name: "empty shelf",
			now:  at(0, 10, 0),
		},
		{
			name: "everything ready now",
			now:  at(0, 10, 10),
			cands: []Candidate{
				cand(1, at(0, 9, 0), at(0, 9, 0), at(0, 15, 0)),
				cand(2, at(0, 8, 0), at(0, 8, 0), at(0, 13, 0)),
			},
			want: want{
				visit: true, recommended: at(0, 10, 10), start: at(0, 10, 10), end: at(0, 13, 0),
				deadline: at(0, 13, 0), score: 2, ready: []int64{1, 2}, maybe: []int64{}, notYet: []int64{},
			},
		},
		{
			name: "at night: ready now, and another maybe ready before the first one's deadline",
			now:  at(0, 22, 0),
			cands: []Candidate{
				cand(1, at(0, 20, 0), at(0, 20, 0), at(1, 1, 15)),
				cand(2, at(1, 0, 0), at(1, 2, 0), at(1, 9, 0)),
			},
			want: want{
				visit: true, recommended: at(1, 0, 0), start: at(1, 0, 0), end: at(1, 1, 0),
				deadline: at(1, 1, 15), score: 1.5, ready: []int64{1}, maybe: []int64{2}, notYet: []int64{},
			},
		},
		{
			name: "a piece whose best-before has passed is not planned",
			now:  at(0, 10, 0),
			cands: []Candidate{
				cand(1, at(0, 5, 0), at(0, 5, 0), at(0, 9, 0)),
				cand(2, at(0, 11, 0), at(0, 12, 0), at(0, 18, 0)),
			},
			want: want{
				visit: true, recommended: at(0, 12, 0), start: at(0, 12, 0), end: at(0, 18, 0),
				deadline: at(0, 18, 0), score: 1, ready: []int64{2}, maybe: []int64{}, notYet: []int64{},
			},
		},
		{
			name: "two clusters, the deadline forces the earlier one",
			now:  at(0, 8, 0),
			cands: []Candidate{
				cand(1, at(1, 8, 0), at(1, 9, 0), at(1, 12, 0)),
				cand(2, at(1, 8, 0), at(1, 9, 0), at(1, 12, 0)),
				cand(3, at(2, 9, 0), at(2, 10, 0), at(2, 18, 0)),
				cand(4, at(2, 9, 0), at(2, 10, 0), at(2, 18, 0)),
				cand(5, at(2, 9, 0), at(2, 10, 0), at(2, 18, 0)),
			},
			want: want{
				visit: true, recommended: at(1, 9, 0), start: at(1, 9, 0), end: at(1, 12, 0),
				deadline: at(1, 12, 0), score: 2, ready: []int64{1, 2}, maybe: []int64{}, notYet: []int64{3, 4, 5},
			},
		},
		{
			name: "ready beats maybe",
			now:  at(0, 8, 0),
			cands: []Candidate{
				cand(1, at(0, 9, 0), at(0, 12, 0), at(0, 18, 0)),
				cand(2, at(0, 10, 0), at(0, 14, 0), at(0, 20, 0)),
			},
			want: want{
				visit: true, recommended: at(0, 14, 0), start: at(0, 14, 0), end: at(0, 18, 0),
				deadline: at(0, 18, 0), score: 2, ready: []int64{1, 2}, maybe: []int64{}, notYet: []int64{},
			},
		},
		{
			name: "tie: the earliest wins, and the window runs through the night",
			now:  at(0, 8, 0),
			cands: []Candidate{
				cand(1, at(0, 19, 0), at(0, 19, 0), at(1, 12, 0)),
			},
			want: want{
				visit: true, recommended: at(0, 19, 0), start: at(0, 19, 0), end: at(1, 12, 0),
				deadline: at(1, 12, 0), score: 1, ready: []int64{1}, maybe: []int64{}, notYet: []int64{},
			},
		},
		{
			name: "only maybe before the deadline",
			now:  at(0, 8, 0),
			cands: []Candidate{
				cand(1, at(0, 22, 0), at(1, 2, 0), at(1, 9, 0)),
				cand(2, at(0, 8, 30), at(0, 9, 0), at(0, 8, 45)),
			},
			want: want{
				visit: true, recommended: at(0, 8, 30), start: at(0, 8, 30), end: at(0, 8, 30),
				deadline: at(0, 8, 45), score: 0.5, ready: []int64{}, maybe: []int64{2}, notYet: []int64{1},
			},
		},
		{
			name: "a window between two grid points scores nothing",
			now:  at(0, 9, 0),
			cands: []Candidate{
				cand(1, at(0, 10, 5), at(0, 10, 10), at(0, 10, 20)),
			},
		},
		{
			name: "due beyond the horizon: nothing scores",
			now:  at(0, 8, 0),
			cands: []Candidate{
				cand(1, at(9, 0, 0), at(10, 0, 0), at(11, 0, 0)),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := PlanVisit(Slots(tc.now, time.UTC), tc.cands)
			w := tc.want
			if (v != nil) != w.visit {
				t.Fatalf("visit = %+v, want visit=%v", v, w.visit)
			}
			if v == nil {
				return
			}
			if !v.RecommendedAt.Equal(w.recommended) || !v.Start.Equal(w.start) || !v.End.Equal(w.end) || !v.Deadline.Equal(w.deadline) {
				t.Errorf("visit at %v [%v, %v] deadline %v; want %v [%v, %v] deadline %v",
					v.RecommendedAt, v.Start, v.End, v.Deadline, w.recommended, w.start, w.end, w.deadline)
			}
			if v.Score != w.score {
				t.Errorf("score = %v, want %v", v.Score, w.score)
			}
			if !slices.Equal(v.Ready, w.ready) || !slices.Equal(v.Maybe, w.maybe) || !slices.Equal(v.NotYet, w.notYet) {
				t.Errorf("ready/maybe/not yet = %v/%v/%v, want %v/%v/%v", v.Ready, v.Maybe, v.NotYet, w.ready, w.maybe, w.notYet)
			}
		})
	}
}

// The planner never recommends a time before now or after the deadline,
// and the deadline is never a best-before that has already passed,
// whatever the shelf looks like.
func TestPlanVisit_NeverAfterDeadline(t *testing.T) {
	loc := mustLoc(t, "Europe/Bucharest")
	now := time.Date(2026, 10, 23, 16, 7, 0, 0, loc) // across the DST change
	slots := Slots(now, loc)
	for i := 0; i < 300; i++ {
		var cands []Candidate
		for j := 0; j < 1+i%7; j++ {
			low := now.Add(time.Duration((i*37+j*53)%150-20) * time.Hour)
			eta := low.Add(time.Duration((i+j)%9) * time.Hour)
			until := eta.Add(time.Duration(1+(i*j)%30) * time.Hour)
			cands = append(cands, cand(int64(j+1), low, eta, until))
		}
		v := PlanVisit(slots, cands)
		if v == nil {
			continue
		}
		if v.RecommendedAt.Before(now) || v.RecommendedAt.After(v.Deadline) || v.End.After(v.Deadline) {
			t.Fatalf("case %d: visit %v–%v, now %v, deadline %v", i, v.Start, v.End, now, v.Deadline)
		}
		if v.Deadline.Before(now) {
			t.Fatalf("case %d: deadline %v already passed", i, v.Deadline)
		}
		if l := v.RecommendedAt.In(loc); !v.RecommendedAt.Equal(now) && l.Minute()%30 != 0 {
			t.Fatalf("case %d: recommended %v off the grid", i, l)
		}
	}
}
