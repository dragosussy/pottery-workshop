package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Observation sources.
const (
	SourceAdd  = "add"
	SourceUser = "user"
	SourceFlip = "flip"
	SourceEnd  = "end"
)

// Observation is one row of observations.
type Observation struct {
	ID         int64
	PieceID    int64
	Dryness    float64
	ObservedAt time.Time
	Source     string
	Note       string
	CreatedAt  time.Time
	DeletedAt  *time.Time
}

const obsCols = `id, piece_id, dryness, observed_at, source, note, created_at, deleted_at`

func scanObs(sc interface{ Scan(...any) error }) (Observation, error) {
	var o Observation
	var observed, created int64
	var deleted sql.NullInt64
	if err := sc.Scan(&o.ID, &o.PieceID, &o.Dryness, &observed, &o.Source, &o.Note, &created, &deleted); err != nil {
		return Observation{}, err
	}
	o.ObservedAt = fromMS(observed)
	o.CreatedAt = fromMS(created)
	o.DeletedAt = fromNullMS(deleted)
	return o, nil
}

func queryObs(ctx context.Context, q Querier, query string, args ...any) ([]Observation, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Observation
	for rows.Next() {
		o, err := scanObs(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// InsertObservation inserts o (ignoring o.ID) and returns its new ID.
func InsertObservation(ctx context.Context, q Querier, o Observation) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO observations
		(piece_id, dryness, observed_at, source, note, created_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		o.PieceID, o.Dryness, ms(o.ObservedAt), o.Source, o.Note, ms(o.CreatedAt), nullMS(o.DeletedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateObservation writes the mutable columns of o.
func UpdateObservation(ctx context.Context, q Querier, o Observation) error {
	_, err := q.ExecContext(ctx, `UPDATE observations SET dryness = ?, observed_at = ?, note = ?, deleted_at = ? WHERE id = ?`,
		o.Dryness, ms(o.ObservedAt), o.Note, nullMS(o.DeletedAt), o.ID)
	return err
}

// GetObservation returns one observation, deleted or not.
func GetObservation(ctx context.Context, q Querier, id int64) (Observation, error) {
	o, err := scanObs(q.QueryRowContext(ctx, `SELECT `+obsCols+` FROM observations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Observation{}, ErrNotFound
	}
	return o, err
}

// PieceObservations returns the live (not deleted) observations of the given
// pieces, oldest first, keyed by piece ID.
func PieceObservations(ctx context.Context, q Querier, pieceIDs []int64) (map[int64][]Observation, error) {
	out := make(map[int64][]Observation, len(pieceIDs))
	if len(pieceIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(pieceIDs))
	for i, id := range pieceIDs {
		args[i] = id
	}
	obs, err := queryObs(ctx, q, `SELECT `+obsCols+` FROM observations
		WHERE deleted_at IS NULL AND piece_id IN (?`+strings.Repeat(",?", len(pieceIDs)-1)+`)
		ORDER BY observed_at, id`, args...)
	if err != nil {
		return nil, err
	}
	for _, o := range obs {
		out[o.PieceID] = append(out[o.PieceID], o)
	}
	return out, nil
}

// HistoryObservations returns every live observation of pieces not entered
// by mistake, oldest first, keyed by piece ID.
func HistoryObservations(ctx context.Context, q Querier) (map[int64][]Observation, error) {
	obs, err := queryObs(ctx, q, `SELECT `+obsCols+` FROM observations
		WHERE deleted_at IS NULL AND piece_id IN (SELECT id FROM pieces WHERE mistake = 0)
		ORDER BY observed_at, id`)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]Observation)
	for _, o := range obs {
		out[o.PieceID] = append(out[o.PieceID], o)
	}
	return out, nil
}

// ObsCursor is a keyset position in the recent-observations listing.
type ObsCursor struct {
	ObservedAt int64 `json:"t"`
	ID         int64 `json:"i"`
}

// ObservationsPage lists live observations of pieces not entered by
// mistake, newest first.
func ObservationsPage(ctx context.Context, q Querier, after *ObsCursor, limit int) ([]Observation, *ObsCursor, error) {
	where := `deleted_at IS NULL AND piece_id IN (SELECT id FROM pieces WHERE mistake = 0)`
	var args []any
	if after != nil {
		where += ` AND (observed_at < ? OR (observed_at = ? AND id < ?))`
		args = append(args, after.ObservedAt, after.ObservedAt, after.ID)
	}
	args = append(args, limit+1)
	obs, err := queryObs(ctx, q, `SELECT `+obsCols+` FROM observations WHERE `+where+`
		ORDER BY observed_at DESC, id DESC LIMIT ?`, args...)
	if err != nil || len(obs) <= limit {
		return obs, nil, err
	}
	obs = obs[:limit]
	last := obs[limit-1]
	return obs, &ObsCursor{ObservedAt: ms(last.ObservedAt), ID: last.ID}, nil
}
