package predict

import "time"

// Observation sources, as stored.
const (
	SourceAdd  = "add"
	SourceUser = "user"
	SourceFlip = "flip"
	SourceEnd  = "end"
)

// Obs is one live (not deleted) observation.
type Obs struct {
	ID      int64
	Dryness float64
	At      time.Time
	Source  string
}

// Piece is what the model needs to know about a piece.
type Piece struct {
	ID        int64
	Number    int64
	GroupID   int64 // 0 when not in a group
	Form      string
	Size      string
	Thickness string
	AddedAt   time.Time
	Ended     bool
	Obs       []Obs // oldest first
	// Unknown is set when a follow-up question about the piece was skipped:
	// its history can't be trusted for training.
	Unknown bool
}

// Interval is a closed stretch of time, e.g. an offline period.
type Interval struct{ From, To time.Time }

func (iv Interval) overlaps(a, b time.Time) bool { return iv.From.Before(b) && iv.To.After(a) }

// Prediction statuses (api-contract §3.3).
const (
	StatusDrying        = "drying"
	StatusProbablyReady = "probably_ready_check"
	StatusReadyNow      = "ready_now"
	StatusMayBePast     = "may_be_past_check"
	StatusPastTrim      = "past_trim"
	StatusEnded         = "ended"
)

// Confidence levels.
const (
	ConfLearning = "learning"
	ConfRough    = "rough"
	ConfDecent   = "decent"
	ConfGood     = "good"
)

// Uncertainty reasons, in the order they are reported.
const (
	ReasonLearning     = "learning"
	ReasonStale        = "stale_observation"
	ReasonDeviceWasOff = "device_was_off"
	ReasonSensorGap    = "sensor_gap"
	ReasonFewChecks    = "few_checks"
)

// Prediction is the live forecast for one piece (api-contract §3.3).
type Prediction struct {
	Status     string
	ETA        *time.Time
	ETALow     *time.Time
	ETAHigh    *time.Time
	ReadyUntil *time.Time
	DrynessNow *float64
	DrynessAt  float64 // central estimate at the evaluation time, unrounded
	Confidence string
	Reasons    []string
	Basis      string // forecast basis: recent | setpoint
	Method     string // prior | learned
	PastPieces int
	Summary    string
	Anchor     *Obs
	// Stale reports that the window is much wider than a fresh check would
	// make it (drives the stale_observation check).
	Stale bool
}
