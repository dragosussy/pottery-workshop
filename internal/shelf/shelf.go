package shelf

import (
	"context"
	"encoding/json"
	"time"

	"humidity/internal/predict"
	"humidity/internal/store"
)

// ShelfView is the GET /shelf body (api-contract §3.7).
type ShelfView struct {
	ComputedAt string      `json:"computed_at"`
	Items      []PieceView `json:"items"`
	NextVisit  *VisitView  `json:"next_visit"`
	CheckThese []CheckView `json:"check_these"`
}

// VisitView is the recommended visit.
type VisitView struct {
	RecommendedAt  string  `json:"recommended_at"`
	Start          string  `json:"start"`
	End            string  `json:"end"`
	Deadline       string  `json:"deadline"`
	ReadyPieceIDs  []int64 `json:"ready_piece_ids"`
	MaybePieceIDs  []int64 `json:"maybe_piece_ids"`
	NotYetPieceIDs []int64 `json:"not_yet_piece_ids"`
	Score          float64 `json:"score"`
	Confidence     string  `json:"confidence"`
}

// CheckView is one "check these" item.
type CheckView struct {
	PieceID int64    `json:"piece_id"`
	Reasons []string `json:"reasons"`
}

func visitView(v *predict.Visit, conf string) *VisitView {
	if v == nil {
		return nil
	}
	return &VisitView{
		RecommendedAt: ts(v.RecommendedAt), Start: ts(v.Start), End: ts(v.End), Deadline: ts(v.Deadline),
		ReadyPieceIDs: v.Ready, MaybePieceIDs: v.Maybe, NotYetPieceIDs: v.NotYet,
		Score: v.Score, Confidence: conf,
	}
}

func checkViews(cs []predict.Check) []CheckView {
	out := make([]CheckView, len(cs))
	for i, c := range cs {
		out[i] = CheckView{PieceID: c.PieceID, Reasons: c.Reasons}
	}
	return out
}

// Shelf returns the pieces on the shelf with predictions, the visit
// recommendation and what to check.
func (s *Service) Shelf(ctx context.Context) (ShelfView, error) {
	pl, err := s.plan(ctx)
	if err != nil {
		return ShelfView{}, err
	}
	items, err := s.OnShelf(ctx)
	if err != nil {
		return ShelfView{}, err
	}
	if items == nil {
		items = []PieceView{}
	}
	return ShelfView{
		ComputedAt: ts(pl.out.ComputedAt),
		Items:      items,
		NextVisit:  visitView(pl.out.Visit, pl.out.VisitConf),
		CheckThese: checkViews(pl.out.Checks),
	}, nil
}

// StatusParts is the shelf-derived part of GET /status (api-contract §3.2).
type StatusParts struct {
	Shelf       ShelfSummary `json:"shelf"`
	Trim        TrimSummary  `json:"trim"`
	Learning    LearningView `json:"learning"`
	LastOffline *OfflineView `json:"last_offline"`
}

// ShelfSummary counts what is on the shelf.
type ShelfSummary struct {
	OnShelf       int            `json:"on_shelf"`
	OldestAddedAt *string        `json:"oldest_added_at"`
	ByStage       map[string]int `json:"by_stage"`
	LastCheckAt   *string        `json:"last_check_at"`
}

// TrimSummary summarises /shelf for the trim card.
type TrimSummary struct {
	ReadyNowCount int        `json:"ready_now_count"`
	ReadyNowUntil *string    `json:"ready_now_until"`
	CheckCount    int        `json:"check_count"`
	LateCount     int        `json:"late_count"`
	NextVisit     *VisitView `json:"next_visit"`
}

// LearningView is the learning-period state: learning | calibrated.
type LearningView struct {
	State   string `json:"state"`
	Samples int    `json:"samples"`
	Needed  int    `json:"needed"`
}

// OfflineView is the last time the device was off, for the banner.
type OfflineView struct {
	From          string `json:"from"`
	To            string `json:"to"`
	PiecesToCheck int    `json:"pieces_to_check"`
}

// lastOfflineWindow is how long after coming back the banner shows.
const lastOfflineWindow = 7 * 24 * time.Hour

// Status returns the shelf part of the home screen.
func (s *Service) Status(ctx context.Context) (StatusParts, error) {
	pl, err := s.plan(ctx)
	if err != nil {
		return StatusParts{}, err
	}
	now := pl.out.ComputedAt
	out := StatusParts{
		Shelf: ShelfSummary{ByStage: map[string]int{}},
		Trim: TrimSummary{
			CheckCount: len(pl.out.Checks),
			NextVisit:  visitView(pl.out.Visit, pl.out.VisitConf),
		},
		Learning: LearningView{State: "calibrated", Samples: pl.out.Learning.Samples, Needed: pl.out.Learning.Needed},
	}
	if pl.out.Learning.Learning {
		out.Learning.State = "learning"
	}
	for _, st := range stages {
		out.Shelf.ByStage[st.Key] = 0
	}

	var oldest, lastCheck, readyUntil *time.Time
	var offline *predict.Interval
	if n := len(pl.offline); n > 0 && now.Sub(pl.offline[n-1].To) <= lastOfflineWindow {
		offline = &pl.offline[n-1]
		out.LastOffline = &OfflineView{From: ts(offline.From), To: ts(offline.To)}
	}
	for _, p := range pl.history {
		if p.Ended {
			continue
		}
		out.Shelf.OnShelf++
		if oldest == nil || p.AddedAt.Before(*oldest) {
			oldest = ptr(p.AddedAt)
		}
		var last predict.Obs
		if n := len(p.Obs); n > 0 {
			last = p.Obs[n-1]
			out.Shelf.ByStage[nearestStage(last.Dryness)]++
		}
		for _, o := range p.Obs {
			if (o.Source == predict.SourceUser || o.Source == predict.SourceFlip) && (lastCheck == nil || o.At.After(*lastCheck)) {
				lastCheck = ptr(o.At)
			}
		}
		if offline != nil && (len(p.Obs) == 0 || last.At.Before(offline.To)) {
			out.LastOffline.PiecesToCheck++
		}
		pred := pl.out.Predictions[p.ID]
		switch pred.Status {
		case predict.StatusReadyNow:
			out.Trim.ReadyNowCount++
			if readyUntil == nil || pred.ReadyUntil.Before(*readyUntil) {
				readyUntil = pred.ReadyUntil
			}
		case predict.StatusMayBePast, predict.StatusPastTrim:
			out.Trim.LateCount++
		}
	}
	out.Shelf.OldestAddedAt = tsp(oldest)
	out.Shelf.LastCheckAt = tsp(lastCheck)
	out.Trim.ReadyNowUntil = tsp(readyUntil)
	return out, nil
}

// LogEntryView is one climate-log entry (api-contract §3.4).
type LogEntryView struct {
	ID    int64           `json:"id"`
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
	At    string          `json:"at"`
}

// ClimateLog returns the entries relevant to [from, to], oldest first:
// periods (offline, sensor_gap) that overlap it, and events inside it.
func (s *Service) ClimateLog(ctx context.Context, from, to time.Time) ([]LogEntryView, error) {
	entries, err := store.LogEntries(ctx, s.st.DB())
	if err != nil {
		return nil, err
	}
	out := []LogEntryView{}
	for _, e := range entries {
		start, end := e.At, e.At
		if e.Kind == store.LogOffline || e.Kind == store.LogSensorGap {
			var v struct{ From, To string }
			if json.Unmarshal(e.Value, &v) == nil {
				if t, err := time.Parse(time.RFC3339, v.From); err == nil {
					start = t
				}
				if t, err := time.Parse(time.RFC3339, v.To); err == nil {
					end = t
				}
			}
		}
		if end.Before(from) || start.After(to) {
			continue
		}
		out = append(out, LogEntryView{ID: e.ID, Kind: e.Kind, Value: e.Value, At: ts(e.At)})
	}
	return out, nil
}
