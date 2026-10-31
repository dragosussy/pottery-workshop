// Package predict holds the drying model and the visit planner
// (mid-batch-changes.md §5, §7, §8 and §9). Everything here is a pure
// function of its inputs: pieces, observations, a dose function, the
// climate log, the studio time zone and the current time.
package predict

import "math"

// VPD constants (mid-batch-changes.md §5.4).
const (
	// VPDFloor keeps drying from stopping entirely in saturated air.
	VPDFloor = 0.05 // kPa
	// VPDRef is the vapour-pressure deficit at 20 °C and 65 % RH: one
	// wall-clock hour at this VPD is one effective hour.
	VPDRef = 0.8183 // kPa
	// DefaultVPD stands in when there is no climate history at all: a damp
	// tent at about 18 °C and 80 % RH.
	DefaultVPD = 0.41 // kPa
)

// VPD returns the vapour-pressure deficit in kPa for a temperature in °C and
// a relative humidity in %, floored at VPDFloor.
func VPD(tempC, rh float64) float64 {
	es := 0.6108 * math.Exp(17.27*tempC/(tempC+237.3))
	return max(es*(1-rh/100), VPDFloor)
}

// EffRate converts a VPD to effective hours per wall-clock hour.
func EffRate(vpd float64) float64 { return max(vpd, VPDFloor) / VPDRef }
