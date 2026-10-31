package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func migrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		num, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %q: want NNNN_name.sql", e.Name())
		}
		body, err := fs.ReadFile(migrationFS, "migrations/"+e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: e.Name(), sql: string(body)})
	}
	slices.SortFunc(out, func(a, b migration) int { return a.version - b.version })
	return out, nil
}

// applied returns the applied migration versions, or nil for a database
// that has never been migrated.
func (s *Store) applied(ctx context.Context) (map[int]bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&n)
	if err != nil || n == 0 {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	done := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		done[v] = true
	}
	return done, rows.Err()
}

// Fresh reports whether the database has never been migrated.
func (s *Store) Fresh(ctx context.Context) (bool, error) {
	done, err := s.applied(ctx)
	return done == nil, err
}

// PendingMigrations returns how many migrations haven't been applied yet.
func (s *Store) PendingMigrations(ctx context.Context) (int, error) {
	all, err := migrations()
	if err != nil {
		return 0, err
	}
	done, err := s.applied(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range all {
		if !done[m.version] {
			n++
		}
	}
	return n, nil
}

// Migrate applies every pending migration, each in its own transaction.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	all, err := migrations()
	if err != nil {
		return err
	}
	done, err := s.applied(ctx)
	if err != nil {
		return err
	}
	for _, m := range all {
		if done[m.version] {
			continue
		}
		err := s.Tx(ctx, func(q Querier) error {
			if _, err := q.ExecContext(ctx, m.sql); err != nil {
				return err
			}
			_, err := q.ExecContext(ctx,
				`INSERT INTO schema_migrations (version, applied_at) VALUES (?, strftime('%s','now') * 1000)`, m.version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
	}
	return nil
}
