package kit

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	_ "modernc.org/sqlite" // the database driver
)

const snapshotsKept = 7

// OpenDB opens dataDir/name, creating it and the photos and backups
// directories as needed, and applies any new migrations. Migrations are
// applied in order and PRAGMA user_version counts the applied ones, so only
// ever append to the list.
func OpenDB(dataDir, name string, migrations []string) (*sql.DB, error) {
	for _, dir := range []string{"photos", "backups"} {
		if err := os.MkdirAll(filepath.Join(dataDir, dir), 0o755); err != nil {
			return nil, err
		}
	}
	dsn := "file:" + filepath.Join(dataDir, name) +
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db, migrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB, migrations []string) error {
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

// DailySnapshot writes backups/YYYY-MM-DD.db unless today's exists, then
// keeps only the newest snapshotsKept.
func DailySnapshot(db *sql.DB, dataDir string, now time.Time) error {
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
	for len(old) > snapshotsKept {
		if err := os.Remove(old[0]); err != nil {
			return err
		}
		old = old[1:]
	}
	return nil
}

// ServeSnapshot answers with a fresh copy of the database, as a download
// called app-YYYY-MM-DD.db.
func ServeSnapshot(w http.ResponseWriter, r *http.Request, db *sql.DB, app string) {
	f, err := os.CreateTemp("", app+"-*.db")
	if err != nil {
		WriteError(w, err)
		return
	}
	f.Close()
	defer os.Remove(f.Name())
	// VACUUM INTO accepts an existing file only if it is empty, as it is.
	if _, err := db.ExecContext(r.Context(), `VACUUM INTO ?`, f.Name()); err != nil {
		WriteError(w, err)
		return
	}
	name := app + "-" + time.Now().Format("2006-01-02") + ".db"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, f.Name())
}
