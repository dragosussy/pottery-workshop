# Backend implementation plan

Status: draft, **revision 3** (2026-10-31). It follows `00-domain-brief.md`
rev. 3, `mid-batch-changes.md` rev. 3 and `api-contract.md` rev. 3:
- plain HTTP, with no idempotency layer
- continuous dryness
- a deadline-aware visit recommendation around the clock (no studio schedule)
- sessions with the device switched off in between
- bounded logs, and no deploy-rollback machinery

## 1. Starting point

- `monitor.Monitor` polls the sensors every 5 s and keeps the latest reading
  in memory.
- The sensor driver is `internal/sensor/sht` (Sensirion SHT3x/SHT4x over I²C,
  done). `sim` is for development.
- No HTTP server and no storage yet. Every reading is logged at Info.

This plan adds:
- persistence and climate history
- the HTTP API
- the piece model
- predictions and visit planning
- photos
- the embedded frontend

It keeps the existing style: standard library first, small packages,
dependencies passed in explicitly in `run()`, and `slog`.

## 2. Package layout

```
cmd/humidity/
  main.go          run(): config → store (backup, migrate) → sensors → monitor → recorder → api → serve
  sensors.go       driver selection (sht3x | sht4x | sim)
internal/
  config/          env config
  sensor/          Reading, Sensor interface; sht/ (done), sim/
  monitor/         polling, per-cycle snapshots, health, calibration offsets
  store/           SQLite open, migrate, backup; one file per table group
    migrations/    0001_init.sql … (embedded)
  recorder/        1-minute aggregation, offline/gap detection, dose index, clock guard
  shelf/           domain: pieces, lifecycle, numbering, observations, follow-ups, settings
  photos/          upload, re-encode, thumbnail, orphan cleanup
  predict/         ETA engine and visit planning (pure functions)
  api/             HTTP handlers, JSON/error helpers, SSE hub
web/               Vue app; embed.go exposes dist as an embed.FS
```

Dependencies to add: `modernc.org/sqlite` (pure Go),
`golang.org/x/sync/errgroup` and `golang.org/x/image/draw`. `golang.org/x/sys`
is already present.

## 3. Changes to existing code

### 3.1 Sensors (done)

`internal/sensor/sht` is done and tested. Configuration:
`SENSOR_DRIVER=sht3x|sht4x|sim`, `I2C_BUS`, and `SENSORS=name:addr`. The
SHT4x heater pulse against drift at high humidity is deferred.

### 3.2 monitor

- **Per-cycle snapshots:** after each poll cycle, publish
  `Cycle{At, Sensors []SensorStatus}`, including sensors that failed. Sends
  never block. The subscribers are the recorder and the SSE hub.
- **Health:** track `LastSuccess`, `LastError` and `FailingSince` per sensor.
- **Calibration:** `SetOffsets(map[string]float64)` applies RH offsets before
  readings are published.
- Per-reading logs move to `Debug`; state changes stay at Info/Warn. With
  bounded Docker logs (§11), this keeps the useful history.

### 3.3 config

| Var | Default | Purpose |
|---|---|---|
| `SENSOR_DRIVER` | `sht4x` | `sht3x` / `sht4x` / `sim` |
| `I2C_BUS` | `/dev/i2c-1` | |
| `SENSORS` | `main:0x44` | `name:address`. Names are permanent keys in the history. |
| `POLL_INTERVAL` | `5s` | |
| `HTTP_ADDR` | `:${PORT}` or `:8080` | |
| `DB_PATH` | `/data/humidity.db` | |
| `PHOTO_DIR` | `photos/` next to `DB_PATH` | |
| `BACKUP_DIR` | `backups/` next to `DB_PATH` | |
| `BACKUP_KEEP` | `14` | |
| `STUDIO_TZ` | `UTC` | Initial value of the `studio_tz` setting |
| `DEMO_SEED` | `false` | Backdated demo data on an empty DB. Only allowed with `SENSOR_DRIVER=sim`. |
| `LOG_LEVEL` | `info` | |

## 4. Persistence (`internal/store`)

- **Open:** the pragmas go in the DSN (`journal_mode(WAL)`,
  `busy_timeout(5000)`, `foreign_keys(1)`, `synchronous(NORMAL)`), with
  `SetMaxOpenConns(1)`. WAL with NORMAL keeps the database consistent after a
  power cut; at worst the last transaction is lost.
- **One `Querier` interface** (implemented by both `*sql.DB` and `*sql.Tx`)
  is used everywhere. With one connection, `db.Query` inside a transaction
  would deadlock.
- **Migrations:** embedded `NNNN_name.sql` files, tracked in
  `schema_migrations`.
- **Backups**, kept simple and suited to a device that is off most of the
  time:
  - **At startup**, *before* migrations run, if the newest backup is older
    than 24 h or migrations are pending. This doubles as the pre-migration
    safety copy.
  - **Daily** while running.
  - Each one is a `VACUUM INTO` a temp file, then a rename to
    `humidity-<UTC timestamp>.db`. Only the newest `BACKUP_KEEP` are kept.
  - **Off-device:** `GET /api/v1/backup` streams a fresh snapshot, and
    Settings has a download link. Photos are not part of it.
- **Time** is stored as INTEGER unix milliseconds UTC.

### Schema (initial)

```sql
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
-- One piece per number among pieces on the shelf (verified on sqlite3).
CREATE UNIQUE INDEX shelf_number ON pieces(number) WHERE ended_at IS NULL AND mistake = 0;

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

CREATE TABLE follow_ups (
  piece_id       INTEGER PRIMARY KEY REFERENCES pieces(id),
  observation_id INTEGER NOT NULL REFERENCES observations(id),
  kind           TEXT NOT NULL,              -- much_slower|went_wetter
  answer         TEXT,                       -- NULL = skipped = unknown
  asked_at       INTEGER NOT NULL
);

CREATE TABLE readings_1m (
  sensor      TEXT NOT NULL,                 -- sensor name, or 'room'
  minute      INTEGER NOT NULL,
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

CREATE TABLE settings (
  key         TEXT PRIMARY KEY,              -- studio_tz|sensor_offsets
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
```

`flip_dryness`, `end_dryness`, `initial_dryness` and `current_dryness` are not
columns. They are read from the observations (`source` = `flip` / `end` /
`add`, and the latest), so there is one source of truth.

Volume: about 2 reading rows per minute while the device is on. That's at
most 1M rows (50 MB) a year, and much less given the power-offs.

## 5. Recorder (`internal/recorder`)

- **Aggregation:** per sensor and per minute (average/minimum/maximum RH,
  average T, average of per-reading VPD, count), plus the `room` series (the
  mean of the `ok` sensors). Closed minutes are written in one transaction,
  and the open minute is flushed on SIGTERM.
- **Clock guard** (the device is often powered on after days off):
  - Nothing is written until the kernel reports the clock as synced
    (`unix.Adjtimex`, `STA_UNSYNC` cleared).
  - It never writes a minute earlier than the last one stored, and inserts
    use `ON CONFLICT DO NOTHING`.
  - `/health.clock_synced` reports the state.
- **Offline detection** at startup (once the clock is synced): if the last
  stored minute is more than 1 h ago, write `offline {from, to}` to the
  climate log and emit `changed`. This is the only way a power-off becomes
  visible.
- **Sensor gaps while running:** a sensor that fails for more than 1 h gets a
  `sensor_gap {sensor, from, to}` entry when it recovers. While it's still
  down, the open gap is shown through `/status`.
- **Dose index:** an in-memory, per-series prefix sum of effective hours over
  minutes. It is rebuilt from `readings_1m` at startup and extended as minutes
  close. `Dose(from, to)` returns the effective hours plus the share that was
  filled in.
  - Gaps under 1 h carry the last VPD forward.
  - Longer gaps are filled with the mean VPD of the 24 h before the gap, and
    only matter when pieces were on the shelf (`mid-batch-changes.md`
    §5.3).

## 6. Domain (`internal/shelf`)

`Service` wraps the store, and handlers call it.

- **Add** (`quantity` 1–20), in one transaction:
  - Each piece takes the lowest free shelf number.
  - Insert the piece plus its `add` observation at `added_at`.
  - Set `group_id` when there is more than one piece.
- **Update** (`PATCH /pieces/{id}` or `/pieces` with `ids`, one transaction,
  all or nothing):
  - With `state`, check against the transition table in contract §3.5,
    which lives as one map, `(from, to) → func`, in `shelf/lifecycle.go`, with
    a table test covering every pair.
  - The forward transitions write `flip`/`end` observations. The undo
    transitions soft-delete them and reassign a reused number.
  - Same state = no-op, apart from any correction fields.
  - Without `state`, apply corrections. Corrections to flip/end/initial
    values move or rewrite the matching observation.
- **Observations:** inserted in one transaction, with dryness rounded to 0.1.
  Then:
  - compute flags
  - decide follow-ups (`predict.FollowUp`) and record the ones asked
  - write a prediction snapshot for each observation
- **Settings:**
  - Validate the IANA zone (`time/tzdata` is embedded), the days and the
    hours.
  - When offsets change, update the monitor and the climate log.
- **Every write** ends with `hub.Changed(...)` after commit, and invalidates
  the prediction cache.

## 7. Photos (`internal/photos`)

- **Upload:** JPEG only, at most 3 MB, at most 20 MP after decoding. The
  server re-encodes at quality 85 (dropping any metadata) and makes a 320px
  thumbnail (`x/image/draw.CatmullRom`), each written to a temp file and then
  renamed.
- **Serving:** `Cache-Control: private, max-age=31536000, immutable`.
- **Cleanup:** an hourly pass deletes photos older than 24 h that no piece
  refers to.

## 8. Predictions and visit planning (`internal/predict`)

This implements `mid-batch-changes.md` §5, §7, §8 and §9. The functions are
pure: their inputs are pieces, observations, a `DoseFunc`, the climate log,
settings and now. Results are recomputed on request, with a cache that any
write invalidates and that expires after 60 s.

| File | Contents |
|---|---|
| `vpd.go` | `VPD(T, RH)`, effective hours, constants |
| `priors.go` | The §8.1 tables, the learning-period constants (5 samples, σ 0.7), the trim band (2.8–3.6) and the target (3.0), all in one place |
| `forecast.go` | The forecast dose: recent 6 h, then 24 h, or the setpoint (FUTURE) |
| `live.go` | Anchor at the last observation (with `σ_obs` = 0.3) → walk forward → `eta`, the 80 % window, `ready_until` (fast side), `dryness_now`, status, uncertainty reasons (including `device_was_off` when the anchor predates an `offline` period) |
| `adapt.go` | `a_studio`, `e_recent` (the last 10 pieces to reach the target), `e_sibling` and `e_piece`, with shrinkage |
| `samples.go` | Training samples: time to 3.0, interpolated in dose; the inclusion table; one sample per group; 3σ rejection |
| `slots.go` | Slots: now, then every :00 and :30 in `studio_tz`, around the clock over 7 days, DST-safe |
| `visit.go` | §7.2: the deadline (earliest future `ready_until`), scoring (1 / 0.5), the best slot ≤ the deadline, the window, and the ready/maybe/not-yet split |
| `checks.go` | The `check_these` reasons, including `not_checked_since_restart` |
| `followup.go` | `went_wetter` always; `much_slower` only after the learning period |

`visit.go` and `slots.go` are the code behind the trim card. They get the
most thorough table tests (§12).

## 9. HTTP API (`internal/api`)

- `api.New(deps)` registers every route in the contract under `/api/v1`.
- **Helpers:**
  - `decodeJSON` (64 KB limit, unknown fields rejected)
  - `writeJSON` and `writeError` (the envelope, with field paths)
  - `Optional[T]` for PATCH merge semantics
- **Middleware:** request logging (Debug, plus Warn on 5xx) and panic
  recovery. **No idempotency layer** (contract §1.2).
- **SSE hub:** forwards cycles as `readings` and `Changed` calls as
  `changed`, sends a 20 s ping, keeps a 16-message buffer per client, and
  allows at most 16 clients.
- **`GET /backup`:** `VACUUM INTO` a temp file, stream it with
  `Content-Disposition`, then delete it.
- **Static frontend:**
  - `web.Dist` served at `/`, with an SPA fallback outside `/api`.
  - Hashed `assets/*` are served as `immutable`. `index.html` and the
    manifest are served as `no-cache`.
  - A plain `go build` uses a `//go:build !embedweb` stub, so it works
    without Node.
- **Server:**
  - Timeouts: ReadHeader 5 s, Read 15 s, Write 30 s, Idle 60 s.
  - `/stream` clears its own write deadline
    (`http.NewResponseController(w).SetWriteDeadline(time.Time{})`).
  - `srv.RegisterOnShutdown(hub.Close)` lets `Shutdown` finish.

## 10. Process wiring and shutdown

`run()`:

1. config
2. open the store → startup backup if due → migrate
3. sensors
4. an `errgroup` running the monitor, the recorder, the daily backup, photo
   cleanup and the HTTP server

On SIGTERM (`docker stop`, or the Pi 5 power button triggering a system
shutdown), the recorder flushes the open minute and the server shuts down
within 5 s. The future control loop becomes one more group member, and turns
the humidifier off on shutdown.

## 11. Build and deploy

- **Pi setup:**
  - Enable I²C (`sudo raspi-config nonint do_i2c 0`) and reboot.
  - Wire the sensor: 3V3, GND, GPIO2/SDA (pin 3), GPIO3/SCL (pin 5).
  - Check that `i2cdetect -y 1` shows `44`.
  - Fit the Pi 5 RTC battery (recommended, because the device is often off).
  - Switch off with the **power button**, not by pulling the plug.
- **Placement:** inside the plastic, at ware height.
- **Compose:**
  - `devices: /dev/i2c-1`
  - `volumes: ./data:/data`
  - environment: `SENSOR_DRIVER`, `SENSORS`, `DB_PATH`, `STUDIO_TZ`
  - **Bounded logs:** `logging: { driver: json-file, options: { max-size:
    "10m", max-file: "3" } }`, at most about 30 MB.
  - Published on **port 80** (`"80:8080"`), so the app is at
    **`http://atelier`**.
    - `atelier` is the Pi's Tailscale machine name. MagicDNS resolves it on
      tailnet devices, so no domain or DNS record is needed. `deploy.sh`
      reaches the Pi as `atelier.tailb9c0c2.ts.net` (`.env.dev`).
    - On a shared workshop Wi-Fi, the port can be bound to the Tailscale IP
      instead. This isn't the default, because a bind to the Tailscale IP
      fails if Docker starts before Tailscale after a power-on.
  - **More apps on `atelier` later.** The name is generic on purpose. When a
    second app arrives:
    - put a small reverse proxy (e.g. Caddy) on port 80 that routes by path
      (`/humidity/`, `/…/`), and move this app to an internal port;
    - make the SPA path-aware at that point (Vite `base`, the router base,
      the API base URL). Until then the app stays at `/`, with nothing to
      configure.
- **Dockerfile:**
  - A `node:22` stage builds `web/`.
  - The Go stage uses `CGO_ENABLED=0` and `-tags embedweb`, plus
    `-ldflags -X` for the version and commit (passed as build args from
    `deploy.sh`).
  - `import _ "time/tzdata"` keeps time zones working.
- **`deploy.sh`** stays as it is (`latest` only, no rollback machinery, per
  the owner).

## 12. Testing

- **`sht`:** done.
- **`store`:** migrations on an empty DB, the startup backup taken before
  migrations, the backup endpoint producing a DB that opens and passes
  `PRAGMA integrity_check`.
- **`recorder`:**
  - bucketing and the room mean
  - the clock guard
  - offline detection at startup (a fake last minute 9 days ago)
  - the dose index, including gap fill
- **`shelf`:** numbering (reuse, undo), every transition pair, bulk
  all-or-nothing, dryness rounding and range, follow-ups asked once.
- **`predict`:**
  - `live`: learning vs. calibrated, `ready_until` expiry, `device_was_off`.
  - `slots`: the around-the-clock grid across DST changes, and "now" on and
    off the half-hour grid.
  - `visit`, table tests from synthetic shelves:
    - everything ready now
    - a night-time visit
    - a piece whose best-before has already passed
    - two clusters, where the deadline forces the earlier one
    - ties
    - an empty shelf
  - Sample inclusion and interpolation.
- **`api`:** `httptest` per endpoint, one SSE test, photo upload limits.
- **End to end:** the `sim` sensor with a temp DB.

## 13. Milestones

| # | Scope | Unblocks |
|---|---|---|
| B1 | Config, store + migrations + startup/daily backups + `/backup`, monitor cycles/health/offsets, recorder (room, clock guard, offline detection, gaps), HTTP skeleton, `/health`, `/status` (room, sensors, `studio_configured`, `last_offline`), `/stream`, `/readings`, `/climate-log`, `/settings`, static embed | F1 |
| B2 | Photos, pieces (add, list, get), PATCH transitions + undo + corrections (single and bulk), `/reference`, `/shelf` (without predictions), demo seed | F2 |
| B3 | Observations (POST/PATCH/DELETE/undelete), flags, `went_wetter`, `changed` events | F3 |
| B4 | `predict` complete: dose index, live ETA, adaptation, samples, learning period, slots + visit, check-these, `much_slower`, snapshots | F4 |
| B5 | Hardening on the Pi: a power-off/on cycle test, performance, accuracy summary for history | F5 / v1 |
| Later | Fitted thickness and size factors, the control loop, the SHT4x heater, alerts | |
