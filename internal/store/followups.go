package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// FollowUp records that a question was asked about a piece. Each piece is
// asked at most once.
type FollowUp struct {
	PieceID       int64
	ObservationID int64
	Kind          string // much_slower | went_wetter
	Answer        *string
	AskedAt       time.Time
}

// InsertFollowUp records a question as asked. It reports false if the piece
// was already asked one.
func InsertFollowUp(ctx context.Context, q Querier, f FollowUp) (bool, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO follow_ups (piece_id, observation_id, kind, answer, asked_at)
		VALUES (?, ?, ?, NULL, ?) ON CONFLICT (piece_id) DO NOTHING`,
		f.PieceID, f.ObservationID, f.Kind, ms(f.AskedAt))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// FollowUpAsked reports whether the piece has been asked a question.
func FollowUpAsked(ctx context.Context, q Querier, pieceID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM follow_ups WHERE piece_id = ?`, pieceID).Scan(&n)
	return n > 0, err
}

// FollowUpForObservation returns the question asked about an observation.
func FollowUpForObservation(ctx context.Context, q Querier, obsID int64) (FollowUp, error) {
	var f FollowUp
	var answer sql.NullString
	var asked int64
	err := q.QueryRowContext(ctx, `SELECT piece_id, observation_id, kind, answer, asked_at FROM follow_ups
		WHERE observation_id = ?`, obsID).Scan(&f.PieceID, &f.ObservationID, &f.Kind, &answer, &asked)
	if errors.Is(err, sql.ErrNoRows) {
		return FollowUp{}, ErrNotFound
	}
	if answer.Valid {
		f.Answer = &answer.String
	}
	f.AskedAt = fromMS(asked)
	return f, err
}

// AnswerFollowUp stores the answer to the question asked about a piece.
func AnswerFollowUp(ctx context.Context, q Querier, pieceID int64, answer string) error {
	_, err := q.ExecContext(ctx, `UPDATE follow_ups SET answer = ? WHERE piece_id = ?`, answer, pieceID)
	return err
}

// UnansweredFollowUps returns the pieces whose question was skipped. Their
// history is unknown, so training leaves them out.
func UnansweredFollowUps(ctx context.Context, q Querier) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT piece_id FROM follow_ups WHERE answer IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// Snapshot is what was predicted for a piece when an observation was made.
type Snapshot struct {
	ObservationID int64
	PieceID       int64
	At            time.Time
	ETA           *time.Time
	ETALow        *time.Time
	ETAHigh       *time.Time
	ReadyUntil    *time.Time
	DrynessPred   *float64
	Method        string
}

// InsertSnapshot stores a prediction snapshot, replacing any earlier one for
// the same observation.
func InsertSnapshot(ctx context.Context, q Querier, s Snapshot) error {
	var pred any
	if s.DrynessPred != nil {
		pred = *s.DrynessPred
	}
	_, err := q.ExecContext(ctx, `INSERT OR REPLACE INTO prediction_snapshots
		(observation_id, piece_id, at, eta, eta_low, eta_high, ready_until, dryness_pred, method)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ObservationID, s.PieceID, ms(s.At), nullMS(s.ETA), nullMS(s.ETALow), nullMS(s.ETAHigh),
		nullMS(s.ReadyUntil), pred, s.Method)
	return err
}

// CountSnapshots returns how many prediction snapshots exist.
func CountSnapshots(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM prediction_snapshots`).Scan(&n)
	return n, err
}
