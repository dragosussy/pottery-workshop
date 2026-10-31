package store

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestMigrate_EmptyDatabase(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()

	if fresh, _ := st.Fresh(ctx); !fresh {
		t.Error("new database not reported fresh")
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n, _ := st.PendingMigrations(ctx); n != 0 {
		t.Errorf("pending after migrate = %d", n)
	}
	// Idempotent.
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	for _, table := range []string{"photos", "pieces", "observations", "follow_ups", "readings_1m", "climate_log", "settings", "prediction_snapshots"} {
		var n int
		st.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		if n != 1 {
			t.Errorf("table %s missing", table)
		}
	}
}

func TestShelfNumberUniqueOnlyOnShelf(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	st.Migrate(ctx)
	now := time.Now()
	p := Piece{Number: 1, Form: "mug", Size: "small", Thickness: "thin", AddedAt: now}
	if _, err := InsertPiece(ctx, st.DB(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertPiece(ctx, st.DB(), p); err == nil {
		t.Error("two pieces on the shelf with number 1 were allowed")
	}
	p.EndedAt, p.EndKind = &now, "trimmed"
	if _, err := InsertPiece(ctx, st.DB(), p); err != nil {
		t.Errorf("an ended piece reusing number 1 was refused: %v", err)
	}
}

func newBackups(st *Store, dir string, now time.Time) *Backups {
	return &Backups{Store: st, Dir: dir, Keep: 2, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }}
}

func TestBackups_StartupBeforeMigrationAndRotation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "h.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	t0 := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	b := newBackups(st, filepath.Join(dir, "backups"), t0)

	// Fresh database: nothing to save.
	if err := b.AtStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if newest, _ := b.Newest(); !newest.IsZero() {
		t.Fatal("backed up a fresh database")
	}

	// Simulate a database at an older schema: only the migrations table.
	st.DB().Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`)
	if err := b.AtStartup(ctx); err != nil {
		t.Fatal(err)
	}
	newest, _ := b.Newest()
	if !newest.Equal(t0) {
		t.Fatalf("newest backup = %v, want one at %v (migrations pending)", newest, t0)
	}
	// The pre-migration copy has no pieces table yet.
	snap, err := sql.Open("sqlite", filepath.Join(b.Dir, "humidity-20261030T120000Z.db"))
	if err != nil {
		t.Fatal(err)
	}
	var n int
	snap.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='pieces'`).Scan(&n)
	snap.Close()
	if n != 0 {
		t.Error("startup backup was taken after migrating")
	}

	st.Migrate(ctx)
	// Up to date and a recent backup: skipped.
	b.Now = func() time.Time { return t0.Add(time.Hour) }
	b.AtStartup(ctx)
	if newest, _ := b.Newest(); !newest.Equal(t0) {
		t.Errorf("took a backup with no reason: %v", newest)
	}
	// A day later: taken. Two more: rotation keeps the newest 2.
	for i := 1; i <= 3; i++ {
		b.Now = func() time.Time { return t0.Add(time.Duration(i) * 25 * time.Hour) }
		if err := b.AtStartup(ctx); err != nil {
			t.Fatal(err)
		}
	}
	names, _ := b.list()
	if len(names) != 2 || names[1] != "humidity-20261102T150000Z.db" {
		t.Errorf("backups = %v, want the newest 2", names)
	}
}

func TestSnapshot_PassesIntegrityCheck(t *testing.T) {
	st := openTemp(t)
	ctx := context.Background()
	st.Migrate(ctx)
	InsertPiece(ctx, st.DB(), Piece{Number: 1, Form: "mug", Size: "small", Thickness: "thin", AddedAt: time.Now()})

	dst := filepath.Join(t.TempDir(), "snap.db")
	if err := st.Snapshot(ctx, dst); err != nil {
		t.Fatal(err)
	}
	snap, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	var res string
	snap.DB().QueryRow(`PRAGMA integrity_check`).Scan(&res)
	if res != "ok" {
		t.Errorf("integrity_check = %q", res)
	}
	if n, _ := CountPieces(ctx, snap.DB()); n != 1 {
		t.Errorf("snapshot has %d pieces, want 1", n)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Error(err)
	}
}
