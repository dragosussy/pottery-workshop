package predict

import (
	"math"
	"slices"
	"testing"
	"time"
)

// constDose is a climate with a constant effective rate, with optional
// stretches counted as filled.
type constDose struct {
	rate   float64 // effective hours per hour
	filled []Interval
}

func (c constDose) Dose(from, to time.Time) (float64, float64) {
	if !to.After(from) {
		return 0, 0
	}
	eff := to.Sub(from).Hours() * c.rate
	f := 0.0
	for _, iv := range c.filled {
		a, b := maxTime(from, iv.From), minTime(to, iv.To)
		if b.After(a) {
			f += b.Sub(a).Hours() * c.rate
		}
	}
	return eff, f
}

var day0 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func h(n float64) time.Time { return day0.Add(time.Duration(n * float64(time.Hour))) }

func TestClimate_TimeAtAndBetween(t *testing.T) {
	now := h(100)
	c := NewClimate(constDose{rate: 0.5}, now)
	// Measured: 10 h from h(80) gives 5 eff-h.
	if got := c.TimeAt(h(80), 5); !got.Equal(h(90)) {
		t.Errorf("TimeAt measured = %v, want %v", got, h(90))
	}
	// Crosses into the forecast: 10 eff-h from h(90) → 5 measured to now, then 10 h at 0.5.
	if got := c.TimeAt(h(90), 10); !got.Equal(h(110)) {
		t.Errorf("TimeAt forecast = %v, want %v", got, h(110))
	}
	if eff, _ := c.Between(h(90), h(110)); math.Abs(eff-10) > 1e-9 {
		t.Errorf("Between = %v, want 10", eff)
	}
	// The forecast uses the last 6 h, then the last 24 h.
	type step struct {
		at   time.Time
		rate float64
	}
	d := stepDose{{h(0), 0.2}, {h(94), 1.0}} // last 6 h at 1.0, earlier at 0.2
	c = NewClimate(d, now)
	want6, want24 := 1.0, (6*1.0+18*0.2)/24
	if math.Abs(c.rate6-want6) > 1e-9 || math.Abs(c.rate24-want24) > 1e-9 {
		t.Errorf("forecast rates = %v, %v; want %v, %v", c.rate6, c.rate24, want6, want24)
	}
	if eff, _ := c.Between(now, h(112)); math.Abs(eff-(6*want6+6*want24)) > 1e-9 {
		t.Errorf("12 h forecast = %v", eff)
	}
}

// stepDose has a piecewise-constant rate starting at each step.
type stepDose []struct {
	at   time.Time
	rate float64
}

func (s stepDose) Dose(from, to time.Time) (float64, float64) {
	eff := 0.0
	for i, st := range s {
		end := time.Unix(1<<40, 0)
		if i+1 < len(s) {
			end = s[i+1].at
		}
		a, b := maxTime(from, st.at), minTime(to, end)
		if b.After(a) {
			eff += b.Sub(a).Hours() * st.rate
		}
	}
	return eff, 0
}

func obs(at time.Time, d float64, src string) Obs { return Obs{Dryness: d, At: at, Source: src} }

// medium mug, medium wall: prior 6 eff-h per unit.
func mug(id int64, o ...Obs) Piece {
	return Piece{ID: id, Number: id, Form: "mug", Size: "medium", Thickness: "medium", AddedAt: o[0].At, Obs: o}
}

func TestLive_LearningPriorWalk(t *testing.T) {
	now := h(10)
	c := NewClimate(constDose{rate: 0.5}, now)
	m := NewModel(nil, constDose{rate: 0.5})
	if !m.Learning || m.Samples != 0 {
		t.Fatalf("empty history: learning=%v samples=%d", m.Learning, m.Samples)
	}
	p := mug(1, obs(h(0), 1.0, SourceAdd))
	pr := m.Live(p, c, LiveContext{})

	// rate = 1/6 per eff-h, 0.5 eff-h/h → 1/12 per hour. 10 h → +0.83.
	if pr.DrynessNow == nil || *pr.DrynessNow != 1.8 {
		t.Errorf("dryness_now = %v, want 1.8", pr.DrynessNow)
	}
	if pr.Status != StatusDrying || pr.Confidence != ConfLearning || pr.Method != "prior" {
		t.Errorf("status/conf/method = %s/%s/%s", pr.Status, pr.Confidence, pr.Method)
	}
	// eta: 2 units → 12 eff-h → 24 h after the anchor.
	if !pr.ETA.Equal(h(24)) {
		t.Errorf("eta = %v, want %v", pr.ETA, h(24))
	}
	if !pr.ETALow.Before(*pr.ETA) || !pr.ETAHigh.After(*pr.ETA) {
		t.Errorf("window [%v, %v] doesn't contain eta %v", pr.ETALow, pr.ETAHigh, pr.ETA)
	}
	// ready_until: eta plus 0.6 units (3.0 → 3.6) at the fast rate
	// (×e^{1.28·0.7}): 0.6·6 eff-h / fast, at 0.5 eff-h per hour.
	fast := math.Exp(z80 * SigmaLearning)
	wantRU := h(24 + 0.6*12/fast)
	if d := pr.ReadyUntil.Sub(wantRU); d < -time.Second || d > time.Second {
		t.Errorf("ready_until = %v, want %v", pr.ReadyUntil, wantRU)
	}
	if !slices.Equal(pr.Reasons, []string{ReasonLearning}) {
		t.Errorf("reasons = %v", pr.Reasons)
	}
}

// history builds n finished pieces that each dried from 1.0 to 3.2 in the
// given hours at constant dose, so their rate is known.
func history(n int, hoursToTarget float64) []Piece {
	var out []Piece
	for i := 0; i < n; i++ {
		start := h(float64(-500 + i*40))
		cross := start.Add(time.Duration(hoursToTarget * float64(time.Hour)))
		p := mug(int64(100+i),
			obs(start, 1.0, SourceAdd),
			obs(cross.Add(-time.Duration(hoursToTarget*0.1*float64(time.Hour))), 2.8, SourceUser),
			obs(cross.Add(time.Duration(hoursToTarget*0.1*float64(time.Hour))), 3.2, SourceEnd),
		)
		p.Ended = true
		out = append(out, p)
	}
	return out
}

func TestLive_CalibratedLearnsFasterStudio(t *testing.T) {
	d := constDose{rate: 0.5}
	// Prior says 2 units take 24 h here; the studio does it in 12 h.
	hist := history(8, 12)
	m := NewModel(hist, d)
	if m.Learning || m.Samples != 8 {
		t.Fatalf("learning=%v samples=%d, want calibrated with 8", m.Learning, m.Samples)
	}
	if m.AStudio <= 0.3 {
		t.Errorf("a_studio = %.3f, want clearly positive (faster than prior)", m.AStudio)
	}

	now := h(2)
	c := NewClimate(d, now)
	p := mug(1, obs(h(0), 1.0, SourceAdd))
	pr := m.Live(p, c, LiveContext{})
	if pr.Confidence == ConfLearning || pr.Method != "learned" {
		t.Errorf("confidence/method = %s/%s", pr.Confidence, pr.Method)
	}
	// Faster than the prior's 24 h, but shrinkage keeps it above 12 h.
	if !pr.ETA.Before(h(20)) || !pr.ETA.After(h(12)) {
		t.Errorf("eta = %v, want between h12 and h20", pr.ETA)
	}
	// Narrower window than during learning.
	learn := NewModel(nil, d).Live(p, c, LiveContext{})
	if pr.ETAHigh.Sub(*pr.ETALow) >= learn.ETAHigh.Sub(*learn.ETALow) {
		t.Errorf("calibrated window %v not narrower than learning %v",
			pr.ETAHigh.Sub(*pr.ETALow), learn.ETAHigh.Sub(*learn.ETALow))
	}
}

func TestLive_Statuses(t *testing.T) {
	d := constDose{rate: 0.5}
	m := NewModel(history(8, 24), d)
	tests := []struct {
		name   string
		now    float64
		obs    []Obs
		status string
		etaNil bool
		ruNil  bool
	}{
		{"drying", 1, []Obs{obs(h(0), 1.0, SourceAdd)}, StatusDrying, false, false},
		{"probably ready", 23, []Obs{obs(h(0), 1.0, SourceAdd)}, StatusProbablyReady, false, false},
		{"ready now", 1, []Obs{obs(h(0), 1.0, SourceAdd), obs(h(0.5), 3.0, SourceUser)}, StatusReadyNow, false, false},
		{"ready expires into may be past", 15, []Obs{obs(h(0), 1.0, SourceAdd), obs(h(0.5), 3.0, SourceUser)}, StatusMayBePast, true, false},
		{"predicted past the band", 60, []Obs{obs(h(0), 1.0, SourceAdd)}, StatusMayBePast, true, false},
		{"observed past the band", 1, []Obs{obs(h(0), 1.0, SourceAdd), obs(h(0.5), 3.8, SourceUser)}, StatusPastTrim, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClimate(d, h(tc.now))
			pr := m.Live(mug(1, tc.obs...), c, LiveContext{})
			if pr.Status != tc.status {
				t.Fatalf("status = %s (dryness %.2f), want %s", pr.Status, pr.DrynessAt, tc.status)
			}
			if (pr.ETA == nil) != tc.etaNil || (pr.ReadyUntil == nil) != tc.ruNil {
				t.Errorf("eta=%v ready_until=%v", pr.ETA, pr.ReadyUntil)
			}
			if tc.status == StatusReadyNow && !pr.ETA.Equal(tc.obs[len(tc.obs)-1].At) {
				t.Errorf("ready_now eta = %v, want the observation time", pr.ETA)
			}
			if tc.status == StatusMayBePast && !pr.ReadyUntil.Before(h(tc.now)) {
				t.Errorf("may_be_past ready_until %v not in the past", pr.ReadyUntil)
			}
		})
	}
}

func TestLive_DeviceWasOffAndStale(t *testing.T) {
	off := Interval{From: h(5), To: h(50)}
	d := constDose{rate: 0.3, filled: []Interval{off}}
	m := NewModel(history(8, 40), d)
	p := mug(1, obs(h(0), 0.5, SourceAdd))
	c := NewClimate(d, h(52))
	pr := m.Live(p, c, LiveContext{Offline: []Interval{off}})

	if !slices.Contains(pr.Reasons, ReasonDeviceWasOff) || slices.Contains(pr.Reasons, ReasonSensorGap) {
		t.Errorf("reasons = %v, want device_was_off without sensor_gap", pr.Reasons)
	}
	if !slices.Contains(pr.Reasons, ReasonStale) || !pr.Stale {
		t.Errorf("reasons = %v, want stale_observation after 52 h", pr.Reasons)
	}
	if !slices.Contains(pr.Reasons, ReasonFewChecks) {
		t.Errorf("reasons = %v, want few_checks", pr.Reasons)
	}
	if pr.Confidence != ConfRough {
		t.Errorf("confidence = %s, want rough", pr.Confidence)
	}
	// The same piece with the dose measured is narrower.
	clean := m.Live(p, NewClimate(constDose{rate: 0.3}, h(52)), LiveContext{})
	if pr.ETAHigh.Sub(*pr.ETALow) <= clean.ETAHigh.Sub(*clean.ETALow) {
		t.Error("filled dose did not widen the window")
	}
}

func TestLive_PieceFactorAbsorbsSlowObservation(t *testing.T) {
	d := constDose{rate: 0.5}
	m := NewModel(history(8, 24), d)
	c := NewClimate(d, h(24))
	expected := m.Live(mug(1, obs(h(0), 1.0, SourceAdd)), c, LiveContext{})
	// After 20 h the model expects ~2.7; the potter says 1.6.
	slow := m.Live(mug(1, obs(h(0), 1.0, SourceAdd), obs(h(20), 1.6, SourceUser)), c, LiveContext{})
	if !slow.ETA.After(expected.ETA.Add(24 * time.Hour)) {
		t.Errorf("slow piece eta %v, expected-piece eta %v: the piece factor didn't slow it", slow.ETA, expected.ETA)
	}
}

func TestFollowUp(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	tests := []struct {
		name      string
		obs       float64
		prev      *float64
		predicted *float64
		learning  bool
		want      string
	}{
		{"none", 2.0, f(1.8), f(2.1), false, ""},
		{"went wetter", 1.0, f(2.5), f(2.6), false, FollowUpWentWetter},
		{"went wetter beats much slower", 1.0, f(2.5), f(3.0), false, FollowUpWentWetter},
		{"wetter by 1.4 only", 1.1, f(2.5), nil, false, ""},
		{"much slower", 1.5, f(1.4), f(3.0), false, FollowUpMuchSlower},
		{"much slower is off while learning", 1.5, f(1.4), f(3.0), true, ""},
		{"went wetter even while learning", 1.0, f(2.5), nil, true, FollowUpWentWetter},
		{"first observation", 1.0, nil, nil, false, ""},
	}
	for _, tc := range tests {
		if got := FollowUp(tc.obs, tc.prev, tc.predicted, tc.learning); got != tc.want {
			t.Errorf("%s: FollowUp = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestInconsistent(t *testing.T) {
	if Inconsistent(2.5, 2.0) {
		t.Error("0.5 wetter is judgment noise, not inconsistent")
	}
	if !Inconsistent(2.5, 1.9) {
		t.Error("0.6 wetter should be inconsistent")
	}
}
