package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Piece is one row of pieces. Dryness values live in observations.
type Piece struct {
	ID        int64
	Number    int64
	GroupID   *int64
	Form      string
	Size      string
	Thickness string
	Label     string
	Notes     string
	PhotoID   *int64
	AddedAt   time.Time
	FlippedAt *time.Time
	EndedAt   *time.Time
	EndKind   string // "" while on the shelf, else trimmed | discarded
	EndNote   string
	Mistake   bool
}

const pieceCols = `id, number, group_id, form, size, thickness, label, notes, photo_id,
	added_at, flipped_at, ended_at, end_kind, end_note, mistake`

func scanPiece(sc interface{ Scan(...any) error }) (Piece, error) {
	var p Piece
	var group, photo, flipped, ended sql.NullInt64
	var endKind sql.NullString
	var added int64
	err := sc.Scan(&p.ID, &p.Number, &group, &p.Form, &p.Size, &p.Thickness, &p.Label, &p.Notes, &photo,
		&added, &flipped, &ended, &endKind, &p.EndNote, &p.Mistake)
	if err != nil {
		return Piece{}, err
	}
	p.GroupID = fromNullInt(group)
	p.PhotoID = fromNullInt(photo)
	p.AddedAt = fromMS(added)
	p.FlippedAt = fromNullMS(flipped)
	p.EndedAt = fromNullMS(ended)
	p.EndKind = endKind.String
	return p, nil
}

func queryPieces(ctx context.Context, q Querier, query string, args ...any) ([]Piece, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Piece
	for rows.Next() {
		p, err := scanPiece(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// InsertPiece inserts p (ignoring p.ID) and returns its new ID.
func InsertPiece(ctx context.Context, q Querier, p Piece) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO pieces
		(number, group_id, form, size, thickness, label, notes, photo_id, added_at, flipped_at, ended_at, end_kind, end_note, mistake)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Number, nullInt(p.GroupID), p.Form, p.Size, p.Thickness, p.Label, p.Notes, nullInt(p.PhotoID),
		ms(p.AddedAt), nullMS(p.FlippedAt), nullMS(p.EndedAt), nullString(p.EndKind), p.EndNote, p.Mistake)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdatePiece writes every column of p.
func UpdatePiece(ctx context.Context, q Querier, p Piece) error {
	_, err := q.ExecContext(ctx, `UPDATE pieces SET
		number = ?, group_id = ?, form = ?, size = ?, thickness = ?, label = ?, notes = ?, photo_id = ?,
		added_at = ?, flipped_at = ?, ended_at = ?, end_kind = ?, end_note = ?, mistake = ?
		WHERE id = ?`,
		p.Number, nullInt(p.GroupID), p.Form, p.Size, p.Thickness, p.Label, p.Notes, nullInt(p.PhotoID),
		ms(p.AddedAt), nullMS(p.FlippedAt), nullMS(p.EndedAt), nullString(p.EndKind), p.EndNote, p.Mistake, p.ID)
	return err
}

// GetPiece returns one piece, including mistakes (they stay reachable by ID).
func GetPiece(ctx context.Context, q Querier, id int64) (Piece, error) {
	p, err := scanPiece(q.QueryRowContext(ctx, `SELECT `+pieceCols+` FROM pieces WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Piece{}, ErrNotFound
	}
	return p, err
}

// OnShelfPieces returns the pieces on the shelf, ordered by number.
func OnShelfPieces(ctx context.Context, q Querier) ([]Piece, error) {
	return queryPieces(ctx, q, `SELECT `+pieceCols+` FROM pieces
		WHERE ended_at IS NULL AND mistake = 0 ORDER BY number`)
}

// HistoryPieces returns every piece not entered by mistake, for training.
func HistoryPieces(ctx context.Context, q Querier) ([]Piece, error) {
	return queryPieces(ctx, q, `SELECT `+pieceCols+` FROM pieces WHERE mistake = 0 ORDER BY id`)
}

// UsedNumbers returns the shelf numbers in use, ascending.
func UsedNumbers(ctx context.Context, q Querier) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT number FROM pieces WHERE ended_at IS NULL AND mistake = 0 ORDER BY number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var n int64
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// PieceCursor is a keyset position in the ended-pieces listing. Pieces on
// the shelf sort first, as if ended at +infinity.
type PieceCursor struct {
	EndedAt int64 `json:"e"` // unix ms; math.MaxInt64 for on-shelf pieces
	ID      int64 `json:"i"`
}

// PiecesPage lists pieces newest ended_at first (pieces on the shelf first
// when includeOnShelf), excluding mistakes. It returns up to limit pieces
// after the cursor, and a cursor for the next page if there may be more.
func PiecesPage(ctx context.Context, q Querier, includeOnShelf bool, after *PieceCursor, limit int) ([]Piece, *PieceCursor, error) {
	const inf = int64(1<<63 - 1)
	var where []string
	var args []any
	where = append(where, "mistake = 0")
	if !includeOnShelf {
		where = append(where, "ended_at IS NOT NULL")
	}
	if after != nil {
		where = append(where, "(coalesce(ended_at, ?) < ? OR (coalesce(ended_at, ?) = ? AND id < ?))")
		args = append(args, inf, after.EndedAt, inf, after.EndedAt, after.ID)
	}
	args = append(args, inf, limit+1)
	pieces, err := queryPieces(ctx, q, `SELECT `+pieceCols+` FROM pieces WHERE `+strings.Join(where, " AND ")+`
		ORDER BY coalesce(ended_at, ?) DESC, id DESC LIMIT ?`, args...)
	if err != nil || len(pieces) <= limit {
		return pieces, nil, err
	}
	pieces = pieces[:limit]
	last := pieces[limit-1]
	next := &PieceCursor{EndedAt: inf, ID: last.ID}
	if last.EndedAt != nil {
		next.EndedAt = ms(*last.EndedAt)
	}
	return pieces, next, nil
}

// GroupSizes returns, per group ID, the number of members not entered by
// mistake.
func GroupSizes(ctx context.Context, q Querier) (map[int64]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT group_id, count(*) FROM pieces
		WHERE group_id IS NOT NULL AND mistake = 0 GROUP BY group_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]int)
	for rows.Next() {
		var g int64
		var n int
		if err := rows.Scan(&g, &n); err != nil {
			return nil, err
		}
		out[g] = n
	}
	return out, rows.Err()
}

// CountPieces returns how many pieces exist, mistakes included.
func CountPieces(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM pieces`).Scan(&n)
	return n, err
}
