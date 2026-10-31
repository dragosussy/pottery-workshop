package predict

import "time"

// Check reasons (api-contract §3.7), in the order they are listed.
const (
	CheckProbablyReady    = "probably_ready"
	CheckMayBePast        = "may_be_past"
	CheckNotToday         = "not_checked_today"
	CheckSinceRestart     = "not_checked_since_restart"
	CheckStaleObservation = "stale_observation"
)

// Check is one piece the potter should look at, and why.
type Check struct {
	PieceID int64
	Reasons []string
}

// CheckInput is one on-shelf piece with its prediction.
type CheckInput struct {
	Piece Piece
	Pred  Prediction
}

// Checks builds the "check these" list, in the given (shelf number) order.
// lastOffline is the most recent offline period, or nil.
func Checks(now time.Time, learning bool, lastOffline *Interval, pieces []CheckInput) []Check {
	out := []Check{}
	for _, in := range pieces {
		var last time.Time
		if n := len(in.Piece.Obs); n > 0 {
			last = in.Piece.Obs[n-1].At
		} else {
			last = in.Piece.AddedAt
		}
		var reasons []string
		add := func(ok bool, r string) {
			if ok {
				reasons = append(reasons, r)
			}
		}
		add(in.Pred.Status == StatusProbablyReady, CheckProbablyReady)
		add(in.Pred.Status == StatusMayBePast, CheckMayBePast)
		add(learning && now.Sub(last) > 24*time.Hour, CheckNotToday)
		add(lastOffline != nil && in.Piece.AddedAt.Before(lastOffline.To) && last.Before(lastOffline.To), CheckSinceRestart)
		add(!learning && in.Pred.Stale, CheckStaleObservation)
		if len(reasons) > 0 {
			out = append(out, Check{PieceID: in.Piece.ID, Reasons: reasons})
		}
	}
	return out
}
