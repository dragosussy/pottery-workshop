package predict

import (
	"slices"
	"testing"
	"time"
)

func TestChecks(t *testing.T) {
	now := h(100)
	off := &Interval{From: h(40), To: h(90)}
	in := func(id int64, added, last float64, status string, stale bool) CheckInput {
		p := mug(id, obs(h(added), 1.0, SourceAdd))
		if last != added {
			p.Obs = append(p.Obs, obs(h(last), 2.0, SourceUser))
		}
		return CheckInput{Piece: p, Pred: Prediction{Status: status, Stale: stale}}
	}
	pieces := []CheckInput{
		in(1, 95, 95, StatusDrying, false),        // fresh: nothing
		in(2, 30, 35, StatusProbablyReady, false), // on the shelf across the restart
		in(3, 30, 92, StatusMayBePast, true),      // checked since the restart
		in(4, 60, 60, StatusDrying, false),        // added while off (backdated), not checked
	}
	learning := Checks(now, true, off, pieces)
	want := []Check{
		{2, []string{CheckProbablyReady, CheckNotToday, CheckSinceRestart}},
		{3, []string{CheckMayBePast}},
		{4, []string{CheckNotToday, CheckSinceRestart}},
	}
	if !checksEqual(learning, want) {
		t.Errorf("learning checks = %v, want %v", learning, want)
	}
	calibrated := Checks(now, false, nil, pieces)
	want = []Check{
		{2, []string{CheckProbablyReady}},
		{3, []string{CheckMayBePast, CheckStaleObservation}},
	}
	if !checksEqual(calibrated, want) {
		t.Errorf("calibrated checks = %v, want %v", calibrated, want)
	}
}

func checksEqual(a, b []Check) bool {
	return slices.EqualFunc(a, b, func(x, y Check) bool { return x.PieceID == y.PieceID && slices.Equal(x.Reasons, y.Reasons) })
}

func TestPlan_EndToEnd(t *testing.T) {
	d := constDose{rate: 0.5}
	now := h(30)
	pieces := history(6, 24)
	ready := mug(1, obs(h(0), 1.0, SourceAdd), obs(h(29), 3.0, SourceUser))
	drying := mug(2, obs(h(20), 1.0, SourceAdd))
	past := mug(3, obs(h(0), 1.0, SourceAdd), obs(h(29), 3.9, SourceUser))
	ready.Number, drying.Number, past.Number = 1, 2, 3
	pieces = append(pieces, ready, drying, past)

	out := Plan(Input{Now: now, Pieces: pieces, Dose: d, Loc: time.UTC})
	// The ready piece already reached the target, so it is a sample too.
	if out.Learning.Learning || out.Learning.Samples != 7 {
		t.Errorf("learning = %+v", out.Learning)
	}
	if out.Predictions[1].Status != StatusReadyNow || out.Predictions[2].Status != StatusDrying || out.Predictions[3].Status != StatusPastTrim {
		t.Errorf("statuses = %s %s %s", out.Predictions[1].Status, out.Predictions[2].Status, out.Predictions[3].Status)
	}
	if out.Predictions[100].Status != StatusEnded {
		t.Errorf("history piece status = %s", out.Predictions[100].Status)
	}
	// Later, piece 2 is maybe-ready too (1.5 beats 1 now), before piece 1's
	// best-before.
	v := out.Visit
	if v == nil || !v.RecommendedAt.After(now) || v.Score != 1.5 || !slices.Equal(v.Ready, []int64{1}) || !slices.Equal(v.Maybe, []int64{2}) ||
		v.RecommendedAt.After(*out.Predictions[1].ReadyUntil) {
		t.Errorf("visit = %+v", out.Visit)
	}
	if out.VisitConf == "" || out.VisitConf == ConfLearning {
		t.Errorf("visit confidence = %q", out.VisitConf)
	}
}
