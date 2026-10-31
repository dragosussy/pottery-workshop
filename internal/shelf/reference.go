package shelf

// Enum keys and labels (api-contract §3.8). Display order is the order here.

// Option is one enum value with its label.
type Option struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Stage is a landmark on the dryness scale.
type Stage struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value int    `json:"value"`
}

// DrynessScale describes the slider.
type DrynessScale struct {
	Min      float64    `json:"min"`
	Max      float64    `json:"max"`
	Step     float64    `json:"step"`
	Target   float64    `json:"target"`
	TrimBand [2]float64 `json:"trim_band"`
}

// Reference is the GET /reference body.
type Reference struct {
	Dryness            DrynessScale `json:"dryness"`
	Stages             []Stage      `json:"stages"`
	Forms              []Option     `json:"forms"`
	Sizes              []Option     `json:"sizes"`
	Thicknesses        []Option     `json:"thicknesses"`
	PieceStates        []Option     `json:"piece_states"`
	PredictionStatuses []Option     `json:"prediction_statuses"`
	UncertaintyReasons []Option     `json:"uncertainty_reasons"`
	CheckReasons       []Option     `json:"check_reasons"`
}

// Piece states.
const (
	StateIn        = "in"
	StateFlipped   = "flipped"
	StateTrimmed   = "trimmed"
	StateDiscarded = "discarded"
)

var (
	stages = []Stage{
		{"very_wet", "Very wet", 0},
		{"wet", "Wet", 1},
		{"soft_leather_hard", "Soft leather hard", 2},
		{"leather_hard", "Leather hard", 3},
		{"hard_leather_hard", "Hard leather hard", 4},
		{"bone_dry", "Bone dry", 5},
	}
	forms       = []Option{{"bowl", "Bowl"}, {"mug", "Mug"}, {"plate", "Plate"}, {"vase", "Vase"}, {"other", "Other"}}
	sizes       = []Option{{"small", "S"}, {"medium", "M"}, {"large", "L"}, {"xlarge", "XL"}}
	thicknesses = []Option{{"thin", "Thin"}, {"medium", "Medium"}, {"thick", "Thick"}}
	pieceStates = []Option{{StateIn, "On shelf"}, {StateFlipped, "Flipped"}, {StateTrimmed, "Trimmed"}, {StateDiscarded, "Discarded"}}
	statuses    = []Option{
		{"drying", "Drying"},
		{"probably_ready_check", "Probably ready, check it"},
		{"ready_now", "Ready now"},
		{"may_be_past_check", "May be past leather hard, check it"},
		{"past_trim", "Past leather hard"},
		{"ended", "Off the shelf"},
	}
	uncertaintyReasons = []Option{
		{"learning", "still learning your shelf"},
		{"stale_observation", "not checked for a while"},
		{"device_was_off", "the controller was off"},
		{"sensor_gap", "the sensor was down"},
		{"few_checks", "only checked when added"},
	}
	checkReasons = []Option{
		{"probably_ready", "Probably ready"},
		{"may_be_past", "May be past leather hard"},
		{"not_checked_today", "Not checked in the last day"},
		{"not_checked_since_restart", "Not checked since the controller came back on"},
		{"stale_observation", "Not checked for a while"},
	}
)

// ReferenceData returns the static reference body.
func ReferenceData() Reference {
	return Reference{
		Dryness:            DrynessScale{Min: 0, Max: 5, Step: 0.1, Target: 3.0, TrimBand: [2]float64{2.8, 3.6}},
		Stages:             stages,
		Forms:              forms,
		Sizes:              sizes,
		Thicknesses:        thicknesses,
		PieceStates:        pieceStates,
		PredictionStatuses: statuses,
		UncertaintyReasons: uncertaintyReasons,
		CheckReasons:       checkReasons,
	}
}

func validKey(opts []Option, key string) bool {
	for _, o := range opts {
		if o.Key == key {
			return true
		}
	}
	return false
}

// nearestStage returns the stage key nearest to a dryness.
func nearestStage(d float64) string {
	i := int(round1(d) + 0.5)
	return stages[min(max(i, 0), len(stages)-1)].Key
}
