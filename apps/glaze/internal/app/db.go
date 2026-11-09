package app

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"atelier/shared/kit"
)

// migrations are applied in order. Append, never edit.
var migrations = []string{`
CREATE TABLE clay_bodies (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  notes TEXT NOT NULL DEFAULT '',
  archived_at INTEGER
);
CREATE TABLE materials (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  maker TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  archived_at INTEGER
);
CREATE TABLE recipes (
  id INTEGER PRIMARY KEY,
  material_id INTEGER NOT NULL REFERENCES materials(id),
  version INTEGER NOT NULL,
  change_note TEXT NOT NULL DEFAULT '',
  water REAL,
  consistency TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (material_id, version)
);
CREATE TABLE recipe_lines (
  id INTEGER PRIMARY KEY,
  recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  ingredient TEXT NOT NULL,
  amount REAL NOT NULL,
  addition INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE firings (
  id INTEGER PRIMARY KEY,
  state TEXT NOT NULL,
  fired_on TEXT,
  cone TEXT NOT NULL DEFAULT '',
  schedule TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX firings_one_at_a_time ON firings ((state != 'unloaded')) WHERE state != 'unloaded';
CREATE TABLE pieces (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL,
  number INTEGER,
  clay_body_id INTEGER REFERENCES clay_bodies(id),
  firing_id INTEGER REFERENCES firings(id),
  level TEXT,
  pos_x REAL,
  pos_y REAL,
  spot TEXT,
  refire_of INTEGER REFERENCES pieces(id),
  notes TEXT NOT NULL DEFAULT '',
  from_memory INTEGER NOT NULL DEFAULT 0,
  rating TEXT,
  result_notes TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX pieces_number ON pieces(number) WHERE refire_of IS NULL;
CREATE INDEX pieces_firing ON pieces(firing_id);
CREATE TABLE layers (
  id INTEGER PRIMARY KEY,
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  material_id INTEGER NOT NULL REFERENCES materials(id),
  recipe_id INTEGER REFERENCES recipes(id),
  stage TEXT,
  method TEXT,
  thickness REAL,
  coats INTEGER,
  area TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT ''
);
CREATE INDEX layers_piece ON layers(piece_id, position);
CREATE INDEX layers_material ON layers(material_id);
CREATE INDEX layers_recipe ON layers(recipe_id);
CREATE TABLE piece_defects (
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  defect TEXT NOT NULL,
  PRIMARY KEY (piece_id, defect)
);
CREATE TABLE photos (
  id INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: photos are cached by id
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  stage TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX photos_piece ON photos(piece_id, id);
`}

// OpenDB opens $dataDir/glaze.db and brings the schema up to date.
func OpenDB(dataDir string) (*sql.DB, error) {
	return kit.OpenDB(dataDir, "glaze.db", migrations)
}

// SnapshotLoop writes the daily database snapshot until ctx ends.
func (a *App) SnapshotLoop(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		// It skips the day's work when today's snapshot already exists.
		if err := kit.DailySnapshot(a.DB, a.DataDir, time.Now().UTC()); err != nil {
			log.Printf("backup: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func isConstraint(err error, kind string) bool {
	return err != nil && strings.Contains(err.Error(), kind+" constraint failed")
}

// placeholders returns "?,?,?" for n values.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
