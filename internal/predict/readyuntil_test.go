package predict

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// ready_until is anchored on eta, so it is never earlier than eta, in the
// learning period and after it, whatever the piece and the climate.
func TestLive_ReadyUntilNeverBeforeETA(t *testing.T) {
	models := map[string]*Model{
		"learning":   NewModel(nil, constDose{rate: 0.5}),
		"calibrated": NewModel(history(8, 24), constDose{rate: 0.5}),
	}
	sizes := []string{"small", "medium", "large", "xlarge"}
	walls := []string{"thin", "medium", "thick"}
	for name, m := range models {
		t.Run(name, func(t *testing.T) {
			checked := 0
			for i := 0; i < 600; i++ {
				rate := 0.15 + float64(i%7)*0.12
				var filled []Interval
				if i%5 == 0 {
					filled = []Interval{{h(2), h(20)}}
				}
				d := constDose{rate: rate, filled: filled}
				p := Piece{ID: 1, Number: 1, Form: "mug", Size: sizes[i%4], Thickness: walls[(i/4)%3], AddedAt: h(0)}
				p.Obs = []Obs{obs(h(0), float64(i%25)/10, SourceAdd)}
				if i%3 == 0 { // a later check, sometimes slower than expected
					p.Obs = append(p.Obs, obs(h(float64(1+i%17)), p.Obs[0].Dryness+float64(i%9)/10, SourceUser))
				}
				now := h(float64(1 + i%60))
				pr := m.Live(p, NewClimate(d, now), LiveContext{Offline: filled})
				if pr.ETA == nil || pr.ReadyUntil == nil {
					continue
				}
				checked++
				if pr.ReadyUntil.Before(*pr.ETA) {
					t.Fatalf("case %d (%s): ready_until %v before eta %v", i, pr.Status, pr.ReadyUntil, pr.ETA)
				}
			}
			if checked < 300 {
				t.Errorf("only %d cases had both eta and ready_until", checked)
			}
		})
	}
}

// A group with the same eta lands in "ready", not "maybe", and the visit
// window opens at the first slot from its eta and closes by its
// best-before (regression: ready_until used to fall before eta
// during the learning period, so the visit came too early).
func TestPlan_SameETAGroupIsReady(t *testing.T) {
	for _, calibrated := range []bool{false, true} {
		t.Run(fmt.Sprintf("calibrated=%v", calibrated), func(t *testing.T) {
			d := constDose{rate: 0.5}
			now := h(30)
			var pieces []Piece
			if calibrated {
				pieces = history(6, 24)
			}
			for id := int64(1); id <= 4; id++ {
				p := mug(id, obs(h(10), 1.0, SourceAdd), obs(h(28), 2.0, SourceUser))
				p.GroupID = 1
				pieces = append(pieces, p)
			}
			out := Plan(Input{Now: now, Pieces: pieces, Dose: d, Loc: time.UTC})
			eta := *out.Predictions[1].ETA
			v := out.Visit
			if v == nil {
				t.Fatal("no visit")
			}
			if !slices.Equal(v.Ready, []int64{1, 2, 3, 4}) || len(v.Maybe) != 0 {
				t.Errorf("ready %v maybe %v, want all four ready", v.Ready, v.Maybe)
			}
			until := *out.Predictions[1].ReadyUntil
			if v.Start.Before(eta) || v.Start.Sub(eta) >= SlotStep || v.End.After(until) {
				t.Errorf("visit %v–%v, want from the first slot at or after eta %v, ending by %v", v.Start, v.End, eta, until)
			}
			if !v.Deadline.After(eta) {
				t.Errorf("deadline %v not after eta %v", v.Deadline, eta)
			}
		})
	}
}
