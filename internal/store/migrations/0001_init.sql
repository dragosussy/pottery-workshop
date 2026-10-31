-- Initial schema (backend plan §4). Times are unix milliseconds UTC.

CREATE TABLE photos (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  width       INTEGER NOT NULL,
  height      INTEGER NOT NULL,
  bytes       INTEGER NOT NULL,
  created_at  INTEGER NOT NULL               -- files: PHOTO_DIR/<id>.jpg, <id>_thumb.jpg
);

CREATE TABLE pieces (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  number      INTEGER NOT NULL,              -- shelf number
  group_id    INTEGER,
  form        TEXT NOT NULL,                 -- bowl|mug|plate|vase|other
  size        TEXT NOT NULL,                 -- small|medium|large|xlarge
  thickness   TEXT NOT NULL,                 -- thin|medium|thick
  label       TEXT NOT NULL DEFAULT '',
  notes       TEXT NOT NULL DEFAULT '',
  photo_id    INTEGER REFERENCES photos(id),
  added_at    INTEGER NOT NULL,
  flipped_at  INTEGER,
  ended_at    INTEGER,
  end_kind    TEXT,                          -- trimmed|discarded
  end_note    TEXT NOT NULL DEFAULT '',
  mistake     INTEGER NOT NULL DEFAULT 0,
  CHECK ((ended_at IS NULL) = (end_kind IS NULL))
);
-- One piece per number among pieces on the shelf.
CREATE UNIQUE INDEX shelf_number ON pieces(number) WHERE ended_at IS NULL AND mistake = 0;
CREATE INDEX pieces_ended ON pieces(ended_at);
CREATE INDEX pieces_group ON pieces(group_id);

CREATE TABLE observations (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  piece_id    INTEGER NOT NULL REFERENCES pieces(id),
  dryness     REAL NOT NULL CHECK (dryness BETWEEN 0 AND 5),  -- rounded to 0.1
  observed_at INTEGER NOT NULL,
  source      TEXT NOT NULL,                 -- add|user|flip|end
  note        TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  deleted_at  INTEGER
);
CREATE INDEX obs_piece ON observations(piece_id, observed_at);
CREATE INDEX obs_time ON observations(observed_at);

CREATE TABLE follow_ups (
  piece_id       INTEGER PRIMARY KEY REFERENCES pieces(id),
  observation_id INTEGER NOT NULL REFERENCES observations(id),
  kind           TEXT NOT NULL,              -- much_slower|went_wetter
  answer         TEXT,                       -- NULL = skipped = unknown
  asked_at       INTEGER NOT NULL
);
CREATE INDEX follow_ups_obs ON follow_ups(observation_id);

CREATE TABLE readings_1m (
  sensor      TEXT NOT NULL,                 -- sensor name, or 'room'
  minute      INTEGER NOT NULL,              -- start of the minute
  hum_avg REAL, hum_min REAL, hum_max REAL,
  temp_avg REAL,
  vpd_avg     REAL NOT NULL,
  n           INTEGER NOT NULL,
  PRIMARY KEY (sensor, minute)
) WITHOUT ROWID;

CREATE TABLE climate_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL,                 -- offline|sensor_gap|sensor_calibrated|sensor_replaced|setpoint_set
  value       TEXT NOT NULL,                 -- JSON
  at          INTEGER NOT NULL
);
CREATE INDEX climate_log_at ON climate_log(at);

CREATE TABLE settings (
  key         TEXT PRIMARY KEY,              -- studio_tz|studio_days|studio_hours|studio_configured|sensor_offsets
  value       TEXT NOT NULL                  -- JSON
);

CREATE TABLE prediction_snapshots (
  observation_id INTEGER PRIMARY KEY REFERENCES observations(id),
  piece_id    INTEGER NOT NULL,
  at          INTEGER NOT NULL,
  eta INTEGER, eta_low INTEGER, eta_high INTEGER, ready_until INTEGER,
  dryness_pred REAL,
  method      TEXT NOT NULL
);
