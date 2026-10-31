package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const backupLayout = "20060102T150405Z"

// Snapshot writes a consistent copy of the database to dst with VACUUM INTO.
// dst must not exist yet.
func (s *Store) Snapshot(ctx context.Context, dst string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dst)
	return err
}

// Backups manages the rotating copies in one directory, named
// humidity-<UTC timestamp>.db.
type Backups struct {
	Store *Store
	Dir   string
	Keep  int
	Log   *slog.Logger
	Now   func() time.Time
}

// list returns the backup file names, oldest first. The timestamp layout
// sorts lexically.
func (b *Backups) list() ([]string, error) {
	entries, err := os.ReadDir(b.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if n := e.Name(); strings.HasPrefix(n, "humidity-") && strings.HasSuffix(n, ".db") {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names, nil
}

// Newest returns the time of the newest backup, or the zero time if there
// is none.
func (b *Backups) Newest() (time.Time, error) {
	names, err := b.list()
	if err != nil || len(names) == 0 {
		return time.Time{}, err
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(names[len(names)-1], "humidity-"), ".db")
	t, err := time.Parse(backupLayout, stamp)
	if err != nil {
		return time.Time{}, nil // a stray name; treat as no backup
	}
	return t, nil
}

// Take writes a new backup (temp file, then rename) and prunes the oldest
// ones beyond Keep. It returns the backup's path.
func (b *Backups) Take(ctx context.Context) (string, error) {
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		return "", err
	}
	name := "humidity-" + b.Now().UTC().Format(backupLayout) + ".db"
	final := filepath.Join(b.Dir, name)
	tmp := filepath.Join(b.Dir, ".tmp-"+name)
	os.Remove(tmp)

	if err := b.Store.Snapshot(ctx, tmp); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("backup: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("backup: %w", err)
	}
	return final, b.prune()
}

func (b *Backups) prune() error {
	names, err := b.list()
	if err != nil {
		return err
	}
	for len(names) > b.Keep {
		if err := os.Remove(filepath.Join(b.Dir, names[0])); err != nil {
			return err
		}
		names = names[1:]
	}
	return nil
}

// AtStartup takes a backup before migrations run, when the newest backup is
// older than 24 h or migrations are pending. A database that has never been
// migrated has nothing worth saving and is skipped.
func (b *Backups) AtStartup(ctx context.Context) error {
	fresh, err := b.Store.Fresh(ctx)
	if err != nil || fresh {
		return err
	}
	pending, err := b.Store.PendingMigrations(ctx)
	if err != nil {
		return err
	}
	newest, err := b.Newest()
	if err != nil {
		return err
	}
	if pending == 0 && b.Now().Sub(newest) < 24*time.Hour {
		return nil
	}
	path, err := b.Take(ctx)
	if err != nil {
		return err
	}
	b.Log.Info("startup backup", "path", path, "pending_migrations", pending)
	return nil
}

// RunDaily takes a backup whenever the newest one is 24 h old, checking
// hourly, until ctx is cancelled. Failures are logged, not fatal.
func (b *Backups) RunDaily(ctx context.Context) error {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		newest, err := b.Newest()
		if err == nil && b.Now().Sub(newest) >= 24*time.Hour {
			var path string
			if path, err = b.Take(ctx); err == nil {
				b.Log.Info("daily backup", "path", path)
			}
		}
		if err != nil && ctx.Err() == nil {
			b.Log.Warn("daily backup failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
