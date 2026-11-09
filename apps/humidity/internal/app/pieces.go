package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"atelier/apps/humidity/internal/predict"
)

var (
	forms       = []string{"bowl", "mug", "plate", "vase", "other"}
	sizes       = []string{"small", "medium", "large", "xlarge"}
	thicknesses = []string{"thin", "medium", "thick"}
	states      = []string{"in", "flipped", "trimmed", "discarded"}
)

type Piece struct {
	ID        int64      `json:"id"`
	Number    int        `json:"number"`
	Form      string     `json:"form"`
	Size      string     `json:"size"`
	Thickness string     `json:"thickness"`
	Label     string     `json:"label"`
	Notes     string     `json:"notes"`
	Photo     bool       `json:"photo"`
	State     string     `json:"state"`
	AddedAt   time.Time  `json:"added_at"`
	FlippedAt *time.Time `json:"flipped_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Dryness   float64    `json:"dryness"` // estimated now on the shelf; the last check once ended
	LastCheck LastCheck  `json:"last_check"`
	Estimate  *Estimate  `json:"estimate"` // nil once ended
	Checks    []Check    `json:"checks,omitempty"`
}

type LastCheck struct {
	Dryness float64   `json:"dryness"`
	At      time.Time `json:"at"`
}

type Check struct {
	ID      int64     `json:"id"`
	Dryness float64   `json:"dryness"`
	At      time.Time `json:"at"`
}

type Estimate struct {
	Status     string     `json:"status"` // drying | ready | past
	ReadyAt    *time.Time `json:"ready_at"`
	ReadyUntil *time.Time `json:"ready_until"`
}

// loadPieces loads every piece, ended ones included, with its checks
// (oldest first) and estimate. n is the number of samples behind the
// studio factor.
func (a *App) loadPieces(ctx context.Context) (pieces []*Piece, n int, err error) {
	// One read transaction, so pieces and checks are a consistent snapshot.
	tx, err := a.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id, number, form, size, thickness, label, notes,
		added_at, flipped_at, ended_at, coalesce(end_kind, '') FROM pieces ORDER BY id`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	byID := make(map[int64]*Piece)
	for rows.Next() {
		var p Piece
		var added int64
		var flipped, ended sql.NullInt64
		var endKind string
		err := rows.Scan(&p.ID, &p.Number, &p.Form, &p.Size, &p.Thickness, &p.Label, &p.Notes,
			&added, &flipped, &ended, &endKind)
		if err != nil {
			return nil, 0, err
		}
		p.AddedAt = unixTime(added)
		p.FlippedAt = unixTimePtr(flipped)
		p.EndedAt = unixTimePtr(ended)
		p.State = pieceState(flipped.Valid, endKind)
		pieces = append(pieces, &p)
		byID[p.ID] = &p
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	rows, err = tx.QueryContext(ctx, `SELECT id, piece_id, dryness, at FROM checks ORDER BY at, id`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Check
		var pieceID, at int64
		if err := rows.Scan(&c.ID, &pieceID, &c.Dryness, &at); err != nil {
			return nil, 0, err
		}
		c.At = unixTime(at)
		byID[pieceID].Checks = append(byID[pieceID].Checks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	in := make([]predict.Piece, len(pieces))
	for i, p := range pieces {
		in[i] = predict.Piece{ID: p.ID, Size: p.Size, Thickness: p.Thickness,
			Ended: p.EndedAt != nil, Trimmed: p.State == "trimmed"}
		for _, c := range p.Checks {
			in[i].Checks = append(in[i].Checks, predict.Check{Dryness: c.Dryness, At: c.At})
		}
		last := p.Checks[len(p.Checks)-1]
		p.LastCheck = LastCheck{Dryness: last.Dryness, At: last.At}
		p.Dryness = last.Dryness
		_, err := os.Stat(a.photoPath(p.ID))
		p.Photo = err == nil
	}
	now := time.Now()
	estimates, n := predict.Run(in, now, a.climate(ctx, now))
	for _, p := range pieces {
		if e, ok := estimates[p.ID]; ok {
			p.Dryness = roundDryness(e.Dryness)
			p.Estimate = &Estimate{Status: e.Status, ReadyAt: e.ReadyAt, ReadyUntil: e.ReadyUntil}
		}
	}
	return pieces, n, nil
}

// piece returns one piece with its checks.
func (a *App) piece(ctx context.Context, id int64) (*Piece, error) {
	pieces, _, err := a.loadPieces(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pieces {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, errNoPiece
}

type newPieces struct {
	Form      string   `json:"form"`
	Size      string   `json:"size"`
	Thickness string   `json:"thickness"`
	Dryness   *float64 `json:"dryness"`
	Label     string   `json:"label"`
	Notes     string   `json:"notes"`
	Quantity  *int     `json:"quantity"`
}

// addPieces creates in.Quantity identical pieces, each with its starting
// dryness as its first check and the lowest free shelf number. The handler
// has validated in and filled in the defaults.
func (a *App) addPieces(ctx context.Context, in newPieces) ([]int64, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	var ids []int64
	for range *in.Quantity {
		number, err := freeNumber(tx, 0)
		if err != nil {
			return nil, err
		}
		res, err := tx.Exec(`INSERT INTO pieces (number, form, size, thickness, label, notes, added_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, number, in.Form, in.Size, in.Thickness, in.Label, in.Notes, now)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO checks (piece_id, dryness, at) VALUES (?, ?, ?)`, id, *in.Dryness, now); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, tx.Commit()
}

// freeNumber returns want if no piece on the shelf uses it, else the lowest
// number that none uses.
func freeNumber(tx *sql.Tx, want int) (int, error) {
	rows, err := tx.Query(`SELECT number FROM pieces WHERE ended_at IS NULL`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	used := make(map[int]bool)
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
		used[n] = true
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if want > 0 && !used[want] {
		return want, nil
	}
	n := 1
	for used[n] {
		n++
	}
	return n, nil
}

// piecePatch is a validated PATCH body.
type piecePatch struct {
	edits   map[string]string // column → new value, for label, notes, form, size, thickness
	state   string            // "" for no change
	dryness *float64          // with state trimmed or discarded
}

// patchPiece applies the edits and the state change in one transaction.
func (a *App) patchPiece(ctx context.Context, id int64, p piecePatch) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var number int
	var flipped, ended sql.NullInt64
	var endKind string
	err = tx.QueryRow(`SELECT number, flipped_at, ended_at, coalesce(end_kind, '') FROM pieces WHERE id = ?`, id).
		Scan(&number, &flipped, &ended, &endKind)
	if err == sql.ErrNoRows {
		return errNoPiece
	}
	if err != nil {
		return err
	}

	for col, v := range p.edits { // col is one of five known names
		if _, err := tx.Exec(`UPDATE pieces SET `+col+` = ? WHERE id = ?`, v, id); err != nil {
			return err
		}
	}

	if p.state != "" {
		from := pieceState(flipped.Valid, endKind)
		onShelf := from == "in" || from == "flipped"
		now := time.Now().Unix()
		switch {
		case p.state == "flipped" && from == "in":
			_, err = tx.Exec(`UPDATE pieces SET flipped_at = ? WHERE id = ?`, now, id)
		case p.state == "in" && from == "flipped":
			_, err = tx.Exec(`UPDATE pieces SET flipped_at = NULL WHERE id = ?`, id)
		case p.state == "in" && !onShelf:
			err = putBack(tx, id, number, ended.Int64)
		case (p.state == "trimmed" || p.state == "discarded") && onShelf:
			_, err = tx.Exec(`UPDATE pieces SET ended_at = ?, end_kind = ? WHERE id = ?`, now, p.state, id)
			d := p.dryness
			if d == nil && p.state == "trimmed" {
				target := predict.Target // never the model's own estimate
				d = &target
			}
			if err == nil && d != nil {
				_, err = tx.Exec(`INSERT INTO checks (piece_id, dryness, at) VALUES (?, ?, ?)`, id, *d, now)
			}
		default:
			return errorf(409, "a piece can't go from %s to %s", from, p.state)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// putBack returns an ended piece to the shelf: it drops the check written
// when it ended (never the last one) and keeps its number if that's free.
func putBack(tx *sql.Tx, id int64, number int, endedAt int64) error {
	number, err := freeNumber(tx, number)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE pieces SET ended_at = NULL, end_kind = NULL, number = ? WHERE id = ?`, number, id); err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM checks WHERE id = (
			SELECT id FROM checks WHERE piece_id = ? AND at = ? ORDER BY id DESC LIMIT 1)
		AND (SELECT count(*) FROM checks WHERE piece_id = ?) > 1`, id, endedAt, id)
	return err
}

func (a *App) deletePiece(ctx context.Context, id int64) error {
	res, err := a.DB.ExecContext(ctx, `DELETE FROM pieces WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNoPiece
	}
	if err := os.Remove(a.photoPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (a *App) addCheck(ctx context.Context, id int64, dryness float64) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ended sql.NullInt64
	err = tx.QueryRow(`SELECT ended_at FROM pieces WHERE id = ?`, id).Scan(&ended)
	if err == sql.ErrNoRows {
		return errNoPiece
	}
	if err != nil {
		return err
	}
	if ended.Valid {
		return errorf(409, "the piece has ended; put it back first")
	}
	if _, err := tx.Exec(`INSERT INTO checks (piece_id, dryness, at) VALUES (?, ?, ?)`, id, dryness, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteCheck deletes a check, unless it is its piece's only one, and
// returns the piece's ID.
func (a *App) deleteCheck(ctx context.Context, checkID int64) (int64, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var pieceID int64
	var count int
	err = tx.QueryRow(`SELECT piece_id, (SELECT count(*) FROM checks c WHERE c.piece_id = checks.piece_id)
		FROM checks WHERE id = ?`, checkID).Scan(&pieceID, &count)
	if err == sql.ErrNoRows {
		return 0, errorf(404, "no such check")
	}
	if err != nil {
		return 0, err
	}
	if count == 1 {
		return 0, errorf(409, "a piece keeps at least one check")
	}
	if _, err := tx.Exec(`DELETE FROM checks WHERE id = ?`, checkID); err != nil {
		return 0, err
	}
	return pieceID, tx.Commit()
}

func (a *App) photoPath(id int64) string {
	return filepath.Join(a.DataDir, "photos", strconv.FormatInt(id, 10)+".jpg")
}

func pieceState(flipped bool, endKind string) string {
	switch {
	case endKind != "":
		return endKind
	case flipped:
		return "flipped"
	default:
		return "in"
	}
}

func unixTime(s int64) time.Time { return time.Unix(s, 0).UTC() }

func unixTimePtr(s sql.NullInt64) *time.Time {
	if !s.Valid {
		return nil
	}
	t := unixTime(s.Int64)
	return &t
}

func roundDryness(d float64) float64 { return math.Round(d*10) / 10 }

// apiError is an error with the HTTP status to answer it with.
type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string { return e.msg }

func errorf(code int, format string, args ...any) error {
	return &apiError{code, fmt.Sprintf(format, args...)}
}

var errNoPiece = errorf(404, "no such piece")

// climate is what the model needs about the shelf's humidity: the recorded
// mean for any stretch, and what to expect from now on (the target, else the
// latest reading, else the last day's mean).
func (a *App) climate(ctx context.Context, now time.Time) predict.Climate {
	meanRH := func(from, to time.Time) (float64, bool) {
		var rh sql.NullFloat64
		err := a.DB.QueryRowContext(ctx, `SELECT avg(rh) FROM readings WHERE minute >= ? AND minute < ?`,
			from.Unix(), to.Unix()).Scan(&rh)
		return rh.Float64, err == nil && rh.Valid
	}
	ahead := float64(predict.RefRH)
	if t := a.Live.Target(); t > 0 {
		ahead = float64(t)
	} else if room, ok := a.Live.Room(now); ok {
		ahead = room.RH
	} else if rh, ok := meanRH(now.Add(-24*time.Hour), now); ok {
		ahead = rh
	}
	return predict.Climate{MeanRH: meanRH, Ahead: ahead}
}
