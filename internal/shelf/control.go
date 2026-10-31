package shelf

import (
	"context"
	"errors"
	"math"
	"time"

	"humidity/internal/predict"
	"humidity/internal/store"
)

const keyTarget = "target_humidity"

// Target humidity limits, in %RH.
const (
	MinTargetPct = 50
	MaxTargetPct = 98
)

// ScopeControl is the change scope for the humidity target.
const ScopeControl = "control"

type storedTarget struct {
	Pct   float64 `json:"pct"`
	SetAt int64   `json:"set_at"` // unix ms
}

// ControlView is the target part of GET /control; the API adds the pump's
// state. Predictions assume the shelf holds the target.
type ControlView struct {
	TargetHumidityPct *float64 `json:"target_humidity_pct"`
	SetAt             *string  `json:"set_at"`
	MinPct            float64  `json:"min_pct"`
	MaxPct            float64  `json:"max_pct"`
}

// SetpointPatch is the PUT /control/setpoint body; null clears the target.
type SetpointPatch struct {
	TargetHumidityPct Opt[float64] `json:"target_humidity_pct"`
}

// PreviewView is the GET /control/preview body: the visit the shelf would
// get with this target.
type PreviewView struct {
	TargetHumidityPct *float64   `json:"target_humidity_pct"`
	ForecastTempC     float64    `json:"forecast_temp_c"`
	NextVisit         *VisitView `json:"next_visit"`
	ReadyNowCount     int        `json:"ready_now_count"`
}

func (s *Service) storedTarget(ctx context.Context, q store.Querier) (*storedTarget, error) {
	var t storedTarget
	err := store.GetSetting(ctx, q, keyTarget, &t)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t.Pct == 0 {
		return nil, nil // cleared
	}
	return &t, nil
}

// Control returns the humidity target.
func (s *Service) Control(ctx context.Context) (ControlView, error) {
	t, err := s.storedTarget(ctx, s.st.DB())
	if err != nil {
		return ControlView{}, err
	}
	out := ControlView{MinPct: MinTargetPct, MaxPct: MaxTargetPct}
	if t != nil {
		out.TargetHumidityPct = &t.Pct
		out.SetAt = tsp(ptr(time.UnixMilli(t.SetAt)))
	}
	return out, nil
}

func validTarget(v *ValidationError, o Opt[float64]) (*float64, bool) {
	pct, ok := o.Value()
	if !ok {
		return nil, true
	}
	if math.IsNaN(pct) || pct < MinTargetPct || pct > MaxTargetPct {
		v.add("target_humidity_pct", CodeOutOfRange, "must be between 50 and 98")
		return nil, false
	}
	pct = round1(pct) + 0
	return &pct, true
}

// SetTarget saves (or, with null, clears) the humidity target and logs it.
func (s *Service) SetTarget(ctx context.Context, patch SetpointPatch) (ControlView, error) {
	v := &ValidationError{}
	if !patch.TargetHumidityPct.Set {
		v.add("target_humidity_pct", CodeRequired, "required (a number, or null to clear)")
	}
	pct, _ := validTarget(v, patch.TargetHumidityPct)
	if err := v.err(); err != nil {
		return ControlView{}, err
	}
	now := s.Now()
	err := s.st.Tx(ctx, func(q store.Querier) error {
		prev, err := s.storedTarget(ctx, q)
		if err != nil {
			return err
		}
		var prevPct *float64
		if prev != nil {
			prevPct = &prev.Pct
		}
		if prevPct == nil && pct == nil || prevPct != nil && pct != nil && *prevPct == *pct {
			return nil
		}
		st := storedTarget{SetAt: now.UnixMilli()}
		if pct != nil {
			st.Pct = *pct
		}
		if err := store.PutSetting(ctx, q, keyTarget, st); err != nil {
			return err
		}
		value := map[string]any{"target_humidity_pct": pct, "previous_pct": prevPct}
		_, err = store.InsertLog(ctx, q, store.LogSetpointSet, value, now)
		return err
	})
	if err != nil {
		return ControlView{}, err
	}
	s.changed([]string{ScopeControl, ScopePredictions, ScopeClimateLog}, nil)
	if s.OnTarget != nil {
		s.OnTarget(pct)
	}
	return s.Control(ctx)
}

// Target returns the saved target percentage, or nil.
func (s *Service) Target(ctx context.Context) (*float64, error) {
	t, err := s.storedTarget(ctx, s.st.DB())
	if err != nil || t == nil {
		return nil, err
	}
	return &t.Pct, nil
}

// ControlEvents adapts the service to control.Events: pump log entries go
// to the climate log, and pump state changes reach live clients.
type ControlEvents struct{ S *Service }

// Log appends a climate-log entry.
func (e ControlEvents) Log(ctx context.Context, kind string, value any) {
	if _, err := store.InsertLog(ctx, e.S.st.DB(), kind, value, e.S.Now()); err != nil {
		return
	}
	e.S.notify.Changed([]string{ScopeControl, ScopeClimateLog}, []int64{})
}

// Changed announces a pump state change; predictions are unaffected.
func (e ControlEvents) Changed() { e.S.notify.Changed([]string{ScopeControl}, []int64{}) }

// Preview plans the shelf as if the target were pct (nil: no target). It
// saves nothing.
func (s *Service) Preview(ctx context.Context, target Opt[float64]) (PreviewView, error) {
	v := &ValidationError{}
	pct, _ := validTarget(v, target)
	if err := v.err(); err != nil {
		return PreviewView{}, err
	}
	pl, err := s.computePlan(ctx, s.Now(), target)
	if err != nil {
		return PreviewView{}, err
	}
	out := PreviewView{
		TargetHumidityPct: pct,
		ForecastTempC:     round1(s.forecastTemp(ctx, pl.out.ComputedAt)),
		NextVisit:         visitView(pl.out.Visit, pl.out.VisitConf),
	}
	for id, pred := range pl.out.Predictions {
		if pred.Status == predict.StatusReadyNow && !pl.history[id].Ended {
			out.ReadyNowCount++
		}
	}
	return out, nil
}

// setpoint returns the target the forecast should assume: the override when
// set, else the stored one.
func (s *Service) setpoint(ctx context.Context, now time.Time, override Opt[float64]) (*predict.Setpoint, error) {
	var pct *float64
	if override.Set {
		if p, ok := override.Value(); ok {
			pct = &p
		}
	} else {
		t, err := s.storedTarget(ctx, s.st.DB())
		if err != nil {
			return nil, err
		}
		if t != nil {
			pct = &t.Pct
		}
	}
	if pct == nil {
		return nil, nil
	}
	return &predict.Setpoint{RH: *pct, TempC: s.forecastTemp(ctx, now)}, nil
}

// forecastTemp is the mean room temperature over the last 24 h, or
// predict.DefaultTempC without data.
func (s *Service) forecastTemp(ctx context.Context, now time.Time) float64 {
	rows, err := store.MinutesBetween(ctx, s.st.DB(), "room", now.Add(-24*time.Hour), now)
	if err != nil {
		return predict.DefaultTempC
	}
	if len(rows) == 0 {
		return predict.DefaultTempC
	}
	sum := 0.0
	for _, m := range rows {
		sum += m.TempAvg
	}
	return sum / float64(len(rows))
}
