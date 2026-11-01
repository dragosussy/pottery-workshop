package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	_ "modernc.org/sqlite"
)

// migrations are applied in order; PRAGMA user_version counts the applied
// ones. Append, never edit.
var migrations = []string{`
CREATE TABLE readings (
  minute INTEGER PRIMARY KEY,
  rh     REAL NOT NULL,
  temp   REAL NOT NULL
);
CREATE TABLE pieces (
  id         INTEGER PRIMARY KEY,
  number     INTEGER NOT NULL,
  form       TEXT NOT NULL,
  size       TEXT NOT NULL,
  thickness  TEXT NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  notes      TEXT NOT NULL DEFAULT '',
  added_at   INTEGER NOT NULL,
  flipped_at INTEGER,
  ended_at   INTEGER,
  end_kind   TEXT
);
CREATE UNIQUE INDEX pieces_number_on_shelf ON pieces(number) WHERE ended_at IS NULL;
CREATE TABLE checks (
  id       INTEGER PRIMARY KEY,
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  dryness  REAL NOT NULL,
  at       INTEGER NOT NULL
);
CREATE INDEX checks_piece ON checks(piece_id, at);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`}

const backupsKept = 7

// OpenDB opens $dataDir/humidity.db, creating it and the photos and backups
// directories as needed, and brings the schema up to date.
func OpenDB(dataDir string) (*sql.DB, error) {
	for _, dir := range []string{"photos", "backups"} {
		if err := os.MkdirAll(filepath.Join(dataDir, dir), 0o755); err != nil {
			return nil, err
		}
	}
	dsn := "file:" + filepath.Join(dataDir, "humidity.db") +
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func loadSettings(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

func setSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func deleteSetting(ctx context.Context, db *sql.DB, key string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	return err
}

func saveMinute(db *sql.DB, minute int64, rh, temp float64) error {
	_, err := db.Exec(`INSERT INTO readings (minute, rh, temp) VALUES (?, ?, ?)
		ON CONFLICT (minute) DO UPDATE SET rh = excluded.rh, temp = excluded.temp`, minute, rh, temp)
	return err
}

// History is the room readings averaged into buckets of step seconds.
type History struct {
	Step int64     `json:"step"`
	T    []int64   `json:"t"`
	RH   []float64 `json:"rh"`
	Temp []float64 `json:"temp"`
}

// history returns the last `buckets` buckets of step seconds, the current
// one included; empty buckets are left out.
func history(ctx context.Context, db *sql.DB, now time.Time, step, buckets int64) (History, error) {
	h := History{Step: step, T: []int64{}, RH: []float64{}, Temp: []float64{}}
	from := (now.Unix()/step - buckets + 1) * step
	rows, err := db.QueryContext(ctx, `
		SELECT minute / ? * ? AS t, round(avg(rh), 2), round(avg(temp), 2)
		FROM readings WHERE minute >= ? GROUP BY t ORDER BY t`, step, step, from)
	if err != nil {
		return h, err
	}
	defer rows.Close()
	for rows.Next() {
		var t int64
		var rh, temp float64
		if err := rows.Scan(&t, &rh, &temp); err != nil {
			return h, err
		}
		h.T, h.RH, h.Temp = append(h.T, t), append(h.RH, rh), append(h.Temp, temp)
	}
	return h, rows.Err()
}

// dailyBackup writes backups/YYYY-MM-DD.db unless today's exists, then
// keeps only the newest backupsKept.
func dailyBackup(db *sql.DB, dataDir string, now time.Time) error {
	dir := filepath.Join(dataDir, "backups")
	path := filepath.Join(dir, now.Format("2006-01-02")+".db")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if _, err := db.Exec(`VACUUM INTO ?`, path); err != nil {
		return err
	}
	log.Printf("backup: wrote %s", path)

	old, err := filepath.Glob(filepath.Join(dir, "[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9].db"))
	if err != nil {
		return err
	}
	slices.Sort(old)
	for len(old) > backupsKept {
		if err := os.Remove(old[0]); err != nil {
			return err
		}
		old = old[1:]
	}
	return nil
}
