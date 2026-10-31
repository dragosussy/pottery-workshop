package predict

import "time"

// SlotStep is the spacing of candidate visit times.
const SlotStep = 30 * time.Minute

// Horizon is how far ahead visits are planned.
const Horizon = 7 * 24 * time.Hour

// Slots returns the candidate visit times in [now, now+Horizon): now
// itself, then every half hour on the local :00 and :30 marks of loc. The
// potter can go to the studio at any time, so the grid runs around the
// clock. Steps are in absolute time, and DST shifts are whole hours (or half
// hours), so the grid stays on local :00 and :30 across a change.
func Slots(now time.Time, loc *time.Location) []time.Time {
	end := now.Add(Horizon)
	out := []time.Time{now}
	t := now.Truncate(time.Minute).Add(time.Minute)
	for t.In(loc).Minute()%30 != 0 {
		t = t.Add(time.Minute)
	}
	for ; t.Before(end); t = t.Add(SlotStep) {
		out = append(out, t)
	}
	return out
}
