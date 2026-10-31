// Package store owns the SQLite database: opening it, migrating the schema,
// taking backups, and the SQL for each table group (one file per group).
//
// Times are stored as INTEGER unix milliseconds UTC. The pool has a single
// connection, so every query must go through the Querier it was handed: a
// db.Query issued while a transaction is open would deadlock.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// ErrNotFound is returned when a row doesn't exist.
var ErrNotFound = errors.New("not found")

// Querier is implemented by both *sql.DB and *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

// Store is an open database.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (creating if needed) the database at path. It doesn't migrate;
// call Migrate after any startup backup.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	// WAL with synchronous=NORMAL keeps the file consistent after a power
	// cut; at worst the last transaction is lost.
	q := url.Values{}
	for _, p := range []string{"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(1)", "synchronous(NORMAL)"} {
		q.Add("_pragma", p)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &Store{db: db, path: path}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying pool, which is also a Querier.
func (s *Store) DB() *sql.DB { return s.db }

// Ping checks that the database answers.
func (s *Store) Ping(ctx context.Context) error {
	var one int
	return s.db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

// Tx runs fn in a transaction, committing if it returns nil.
func (s *Store) Tx(ctx context.Context, fn func(q Querier) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ms converts a time to stored unix milliseconds.
func ms(t time.Time) int64 { return t.UnixMilli() }

// fromMS converts stored unix milliseconds to a UTC time.
func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

// nullMS converts an optional time to a nullable column value.
func nullMS(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UnixMilli()
}

// fromNullMS converts a nullable column to an optional time.
func fromNullMS(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromMS(v.Int64)
	return &t
}

// nullInt converts an optional ID to a nullable column value.
func nullInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func fromNullInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
