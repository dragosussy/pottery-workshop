package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Photo is one row of photos. The files live in the photo directory.
type Photo struct {
	ID        int64
	Width     int
	Height    int
	Bytes     int64
	CreatedAt time.Time
}

// InsertPhoto inserts p (ignoring p.ID) and returns its new ID.
func InsertPhoto(ctx context.Context, q Querier, p Photo) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO photos (width, height, bytes, created_at) VALUES (?, ?, ?, ?)`,
		p.Width, p.Height, p.Bytes, ms(p.CreatedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetPhoto returns one photo row.
func GetPhoto(ctx context.Context, q Querier, id int64) (Photo, error) {
	var p Photo
	var created int64
	err := q.QueryRowContext(ctx, `SELECT id, width, height, bytes, created_at FROM photos WHERE id = ?`, id).
		Scan(&p.ID, &p.Width, &p.Height, &p.Bytes, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Photo{}, ErrNotFound
	}
	p.CreatedAt = fromMS(created)
	return p, err
}

// DeletePhoto removes a photo row.
func DeletePhoto(ctx context.Context, q Querier, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM photos WHERE id = ?`, id)
	return err
}

// OrphanPhotos returns photos created before the cutoff that no piece
// refers to.
func OrphanPhotos(ctx context.Context, q Querier, before time.Time) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM photos WHERE created_at < ?
		AND id NOT IN (SELECT photo_id FROM pieces WHERE photo_id IS NOT NULL)`, ms(before))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
