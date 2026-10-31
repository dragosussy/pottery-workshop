package predict

import (
	"testing"
	"time"
)

func TestClimate_WithSetpoint(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	start := now.Add(-10 * time.Hour)
	recent := NewClimate(constDose{rate: 0.5}, now)
	if recent.Basis() != "recent" {
		t.Fatalf("basis = %s", recent.Basis())
	}

	damp := NewClimate(constDose{rate: 0.5}, now).WithSetpoint(&Setpoint{RH: 90, TempC: 18})
	dry := NewClimate(constDose{rate: 0.5}, now).WithSetpoint(&Setpoint{RH: 60, TempC: 18})
	if damp.Basis() != "setpoint" {
		t.Fatalf("basis = %s", damp.Basis())
	}
	// The measured stretch is the same whatever the target.
	if a, _ := damp.Between(start, now); a != 5 {
		t.Errorf("measured dose = %v, want 5", a)
	}
	// Afterwards the rate is the target's.
	want := EffRate(VPD(18, 90)) * 24
	if got, _ := damp.Between(now, now.Add(24*time.Hour)); got < want-1e-9 || got > want+1e-9 {
		t.Errorf("forecast dose = %v, want %v", got, want)
	}
	// A damper target means later.
	if !damp.TimeAt(start, 20).After(dry.TimeAt(start, 20)) {
		t.Error("a higher humidity target should push the time out")
	}
	if NewClimate(constDose{rate: 0.5}, now).WithSetpoint(nil).Basis() != "recent" {
		t.Error("nil setpoint should keep the recent forecast")
	}
}
