package predict

// Follow-up kinds (api-contract §3.9).
const (
	FollowUpMuchSlower = "much_slower"
	FollowUpWentWetter = "went_wetter"
)

// FollowUp decides which question, if any, a new observation raises
// (mid-batch-changes.md §9): went_wetter when it is at least FollowUpStep
// wetter than the previous observation; otherwise much_slower when it is at
// least FollowUpStep below what was predicted for that moment, but never
// during the learning period. prev and predicted may be nil.
func FollowUp(observed float64, prev, predicted *float64, learning bool) string {
	if prev != nil && *prev-observed >= FollowUpStep-1e-9 {
		return FollowUpWentWetter
	}
	if !learning && predicted != nil && *predicted-observed >= FollowUpStep-1e-9 {
		return FollowUpMuchSlower
	}
	return ""
}
