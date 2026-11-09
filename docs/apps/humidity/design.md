# Humidity control: design

What the app does and how it's built, as it is now (November 2026). This is
the second version: the first one grew into a much larger system and was
rewritten to this page. Keep the two in step: when the code changes in a way
this page describes, update the page. Ideas for later are in
[ideas.md](ideas.md); getting started is in the [README](README.md).

## 1. What it is for

One potter, one drying shelf wrapped in plastic, one clay body, no fan. A
Raspberry Pi 5 next to the shelf:

- reads one or two SHT31/SHT4x humidity sensors over I²C;
- runs a misting pump (relay on a GPIO pin) to hold a humidity target;
- shows the room's humidity and temperature on a small 1.9" ST7789 screen
  (SPI) beside it;
- keeps a list of the pottery pieces on the shelf, with the potter's dryness
  checks, and estimates when each piece will be leather hard (trimmable);
- serves a phone web app over Tailscale at `http://atelier/humidity/` (plain
  HTTP, no auth; the tailnet is the boundary).

The Pi stays on. If it's ever off, nothing special happens: the history simply
has a gap. Fit the Pi 5's RTC battery so the clock is right straight after
boot.

## 2. Features

**Climate**
- Current humidity and temperature, with how old the reading is.
- A chart of humidity and temperature: last hour, 24 h or 7 days.
- A calibration offset per sensor (set after a salt test).

**Humidity target and pump**
- One target (whole percent, 50 up to `PUMP_MAX_RH` − 1), or none. The pump
  holds it. No target means humidity control is off (e.g. the shelf is empty
  and left open): the app and the screen show "Off", readings carry on.
- Pump state on the home screen: running / resting / idle / stopped, and why.
- Run the pump by hand (the app's button runs it for 1 s, to test the wiring
  or prime it), Stop it (it stays stopped until Resume), and Resume.

**Pieces**
- Add one or more identical pieces: form, size, wall thickness, starting
  dryness, optional label, optional photo, quantity (creates N independent
  pieces).
- Each piece on the shelf has a short number, written on the bat in pencil.
- States: `in` → `flipped` → `trimmed` or `discarded`. Flip can be undone, and
  "Put back" undoes an end.
- Record dryness checks with a precise slider (0.0–5.0, step 0.1). Delete a
  wrong check.
- Flipped / Trimmed / Discarded as large buttons on the piece page, each
  with Undo.
- Edit a piece's label, notes, form, size and thickness. Delete a piece.
- An estimate per piece: "ready around Tue 10 Nov 13:00 (in 8 days)", "ready
  now, until today 21:00", or "probably past leather hard". Times say
  "today", "tomorrow" or "yesterday", and otherwise always carry the date, so
  next Tuesday is never mistaken for tomorrow.
- A list of finished pieces (trimmed/discarded).

**The screen next to the Pi**
- A 1.9" 170 × 320 IPS module (ST7789, SPI), used in landscape: "Humidity",
  the value in large type, and the temperature underneath; "Target 80 %" or
  "Control off" top right; "no reading" when no sensor has reported for 30 s. It redraws when the values change (checked
  every 5 s, in the same process as the sensor loop, not over HTTP) and goes
  dark when the app stops. Wiring is in [hardware.md](hardware.md).

**Housekeeping**
- A daily database backup on the Pi (7 kept; photos aren't included), and a
  download button.

### Dryness scale

A continuous value 0.0–5.0, stored and shown to one decimal. The whole
numbers are named landmarks:

| value | label |
|---|---|
| 0 | Very wet |
| 1 | Wet |
| 2 | Soft leather hard |
| 3 | **Leather hard** (trim target) |
| 4 | Hard leather hard |
| 5 | Bone dry |

A piece is trimmable between **2.8 and 3.6**.

### Deliberately dropped

These existed in the first version and were removed on purpose: live push updates (SSE),
backdating ("this happened 2 hours ago"), piece groups, follow-up questions,
soft deletes and undo toasts, "entered by mistake", observation editing
(delete and re-add instead), the visit planner ("come Sat 09:00–11:30"), the
target preview, confidence levels and uncertainty reasons, the climate event
log, offline and sensor-gap detection, the clock-sync guard, the VPD
(temperature + humidity) drying-dose model with gap filling, the studio time-zone setting (the phone shows local time), photo
thumbnails, pagination, demo data seeding, the mock frontend
backend, the colour palette picker, and version/commit build stamping.

## 3. Prediction

Clay dries at a speed set by how dry the air is: `100 − RH`. The model keeps
every rate at a reference humidity of **80 %**. It converts what a piece did
using the humidity recorded while it was watched, and projects its future at
the humidity the shelf is expected to have. So changing the target moves
every estimate straight away, with no new check. Checks are always ordered by
`(at, id)`.

**Drying speed** relative to 80 %:

```
speed(rh) = max(100 − rh, 1) / 20          // 80 % → 1, 90 % → 0.5, 60 % → 2
```

For a past stretch, `rh` is the mean of the recorded minutes in it. When
nothing was recorded (the Pi was off), the expected humidity below stands in.
The **expected humidity** is the target, else the latest reading, else the
last day's mean, else 80.

**Prior rate** (dryness units per hour at 80 %) from the piece's build:

```
hoursPerUnit = {thin: 6, medium: 12, thick: 24}
sizeFactor   = {small: 0.75, medium: 1, large: 1.4, xlarge: 1.9}
prior        = 1 / (hoursPerUnit[thickness] × sizeFactor[size])
```

**Studio factor `k`**, learned from trimmed pieces. A piece is a sample when
its first check is below 2.8, its last check is at or above 2.8, and they are
at least 1 h apart:

```
r_i = (d_last − d_first) / hours(first → last) / speed(first → last)
k   = exp( Σ ln(r_i / prior_i) / (n + 1) )      // n samples; k = 1 with none
```

**Per-piece rate** at 80 %. Its own slope is blended in by how long it has
been watched:

```
base  = k × prior
h     = hours(first check → last check)
own   = (d_last − d_first) / h / speed(first → last)
w     = (h ≥ 1 and own > 0) ? min(h / 12, 6) : 0
own'  = clamp(own, 0.1·base, 10·base)
rate  = exp( (w·ln(own') + 2·ln(base)) / (w + 2) )      // = base when w = 0
```

**Outputs** for a piece on the shelf (`t_last`, `d_last` = its latest check).
Since the last check it dried at the recorded humidity; from now on it dries
at the expected one:

```
dryness_now = min(d_last + rate × speed(t_last → now) × hours(t_last → now), 5)
future      = rate × speed(expected humidity)
status      = dryness_now < 2.8 → "drying"
              dryness_now ≤ 3.6 → "ready"
              otherwise         → "past"
ready_at    = drying: now + (3.0 − dryness_now) / future;          else null
ready_until = drying or ready: now + (3.6 − dryness_now) / future; else null
learning    = n < 5     // the UI says "rough guess" while true
```

`ready_at` is when the piece reaches the trim target (3.0), even though its
status turns `ready` at 2.8. Ended pieces get no estimate.

`predict.Run(pieces, now, climate) → (estimates, n)` is a pure function. The
app passes a `Climate`: a function giving the mean recorded humidity for a
stretch (one SQL `avg` over `readings`) and the expected humidity. It is
computed on every request (tens of pieces). After the target is saved, the
web app refreshes the piece list so the new estimates show at once.

The model only uses humidity, not temperature: with the shelf held at a
target, temperature swings are small and average out, and whatever stays
constant is absorbed into `k`.

## 4. Pump control

One goroutine. It waits with `select` on its timer, a command channel and the
context, so Stop and manual runs act immediately. An HTTP handler sends a
command and waits for the loop to apply it before returning Now.

**Readings.** Room RH is the mean of the sensors read in the last 30 s,
offsets applied, used for control. The safety cutoff uses the **highest**
fresh sensor (one near the nozzle reads high).

**Decision**, taken every 5 s while idle:

```
stopped                      → stay off   (reason: user | no_rise | switch_error)
no target                    → idle       (reason: no_target)
no fresh reading             → idle       (reason: no_data)
highest RH ≥ PUMP_MAX_RH     → idle       (reason: above_max)
room RH ≥ target             → idle       (reason: at_target)
otherwise                    → pulse: on for PUMP_PULSE, off, rest PUMP_REST
```

The fixed rest is what prevents chatter, so there's no hysteresis band.

**Miss check**, after each automatic pulse and its rest: `rise` is the highest
room RH seen during pulse + rest minus the room RH when the pulse started. If
`rise < 0.3`, `misses++`; if it rose, `misses = 0`. If there was no fresh
reading at the start or at the end, the cycle doesn't count either way. At 6
misses the pump is `stopped` with reason `no_rise` (usually an empty tank or
a blocked nozzle). Resume clears it and resets `misses`. The stop is held in
memory: a reboot clears it.

**Commands**

- **Run** (1–60 s): allowed while stopped and without a reading (to test the
  wiring), refused with 409 when the highest fresh reading is ≥ `PUMP_MAX_RH`.
  It doesn't clear a stop. Running while already running restarts the timer
  for the new duration. Afterwards comes the normal rest (or, if stopped,
  straight back to stopped), with no miss check.
- **Stop**: switches the pump off now and latches `stopped` (reason `user`)
  until Resume.
- **Resume**: clears `stopped` and `misses`; the next decision follows.
- A target change takes effect at the next decision; a pulse in progress
  finishes.

**Safety**

- The GPIO line is requested driven off. The loop switches the pump off when
  it exits, and `main` does it again in a `defer`; `main` catches SIGTERM with
  `signal.NotifyContext` so `docker stop` and the Pi's power button reach that
  code.
- A failed GPIO write latches `stopped` with reason `switch_error`, after one
  more attempt to switch off.
- No runtime cap: holding a damp shelf can take days of pumping.

## 5. Data

SQLite via modernc at `$DATA_DIR/humidity.db`, WAL, default pool. The pragmas
go in the DSN so every pooled connection has them:
`file:…?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)`.
The schema is versioned with `PRAGMA user_version` and a list of SQL strings in
Go. This is a fresh schema; data from the first version isn't migrated.

```sql
CREATE TABLE readings (            -- one row per minute, room mean
  minute INTEGER PRIMARY KEY,      -- unix seconds, start of the minute
  rh     REAL NOT NULL,
  temp   REAL NOT NULL
);
CREATE TABLE pieces (
  id         INTEGER PRIMARY KEY,
  number     INTEGER NOT NULL,     -- shelf number
  form       TEXT NOT NULL,        -- bowl | mug | plate | vase | other
  size       TEXT NOT NULL,        -- small | medium | large | xlarge
  thickness  TEXT NOT NULL,        -- thin | medium | thick
  label      TEXT NOT NULL DEFAULT '',
  notes      TEXT NOT NULL DEFAULT '',
  added_at   INTEGER NOT NULL,     -- unix seconds
  flipped_at INTEGER,
  ended_at   INTEGER,
  end_kind   TEXT                  -- trimmed | discarded
);
CREATE UNIQUE INDEX pieces_number_on_shelf ON pieces(number) WHERE ended_at IS NULL;
CREATE TABLE checks (              -- dryness observations
  id       INTEGER PRIMARY KEY,
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  dryness  REAL NOT NULL,          -- 0.0–5.0, one decimal
  at       INTEGER NOT NULL
);
CREATE INDEX checks_piece ON checks(piece_id, at);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
  -- 'target' → "82" (absent: none), 'offset:0x44' → "1.5"
```

- **State** is derived: `ended_at` set → `end_kind`; else `flipped_at` set →
  `flipped`; else `in`.
- **Adding** a piece writes its starting dryness as its first check, and gives
  it the lowest number not used by a piece on the shelf.
- **Ending** (trim/discard) sets `ended_at`/`end_kind`. Trim writes a check at
  `ended_at` with the given dryness, or 3.0 when none is given. Discard writes
  a check only when a dryness is given.
- **Put back** clears `ended_at`/`end_kind`, deletes the check written at
  `ended_at` (if any), and keeps the piece's old number if it's free, else the
  lowest free one. `flipped_at` is kept, so a flipped piece comes back flipped.
- **Undo flip** clears `flipped_at`.
- A piece always keeps at least one check: deleting its only check is refused.
- **Photos** are files `$DATA_DIR/photos/<piece_id>.jpg`. The phone uploads
  the camera's original (JPEG or PNG, ≤ 25 MB); the server turns it upright
  from its EXIF orientation, shrinks it to 1600 px on the long edge and saves
  a JPEG (`disintegration/imaging`). It isn't done on the phone because
  browsers with fingerprinting protection scramble canvas output. A piece has
  a photo when its file exists.
  Deleting a piece deletes its photo.
- **Backups:** `VACUUM INTO $DATA_DIR/backups/YYYY-MM-DD.db` at startup and
  once a day, skipped when today's file exists; the newest 7 are kept.

## 6. HTTP API

JSON in and out. Times are RFC 3339 UTC strings, except the history arrays,
which use unix seconds. Errors are `{"error": "message"}` with 400 (invalid
input), 404, 409 (not allowed in the current state), 413 (photo too big) or
500. Unknown `/api/*` paths get a JSON 404, never the web app.

| Method + path | Body / query | Returns |
|---|---|---|
| `GET /api/summary` | | `{"text": "81 % · 3 pieces, 1 ready to trim"}`, for the homepage |
| `GET /api/now` | | Now |
| `GET /api/history` | `?hours=1`, `24` or `168` | History |
| `PUT /api/target` | `{"pct": 82}` or `{"pct": null}` | Now |
| `POST /api/pump/run` | `{"seconds": 5}` | Now |
| `POST /api/pump/stop` | | Now |
| `POST /api/pump/resume` | | Now |
| `PUT /api/sensors/{addr}/offset` | `{"offset": 1.5}` | Now |
| `GET /api/pieces` | `?ended=1` for finished ones | `[Piece]` |
| `POST /api/pieces` | NewPieces | `[Piece]` created, by number |
| `GET /api/pieces/{id}` | | Piece with `checks` |
| `PATCH /api/pieces/{id}` | PiecePatch | Piece with `checks` |
| `DELETE /api/pieces/{id}` | | 204 |
| `PUT /api/pieces/{id}/photo` | the raw image (JPEG or PNG), ≤ 25 MB | 204 |
| `GET /api/pieces/{id}/photo` | | the JPEG (404 if none) |
| `POST /api/pieces/{id}/checks` | `{"dryness": 2.4}` | Piece with `checks` |
| `DELETE /api/checks/{id}` | | Piece with `checks` |
| `GET /api/backup` | | a fresh `.db` snapshot as a download |
| `GET /*` | | the embedded web app (index.html fallback) |

### Validation

- `dryness`: number 0–5, rounded to 0.1 by the server.
- `pct`: integer, 50 to `PUMP_MAX_RH` − 1.
- `seconds`: integer 1–60.
- `offset`: number −10…10, applied to RH only. `addr` is lowercase hex
  (`0x44`) and must be a configured sensor (else 404).
- `quantity`: integer 1–20, default 1.
- `label` ≤ 100 characters, `notes` ≤ 2000.
- `form`, `size`, `thickness`: one of the values in §5.
- JSON bodies with unknown fields are a 400, so a misspelled field fails
  loudly. A missing `pct` in `PUT /api/target` means null (clear).
- `GET /api/history` without `hours` means 24.
- Anything else wrong is a 400 with a message.

### Now

```json
{
  "room": { "rh": 81.2, "temp": 18.6, "at": "2026-11-01T10:00:05Z" },
  "sensors": [
    { "addr": "0x44", "rh": 81.2, "raw_rh": 79.7, "temp": 18.6, "offset": 1.5,
      "at": "2026-11-01T10:00:05Z", "error": null }
  ],
  "target": 82,
  "max_rh": 95,
  "pump": { "enabled": true, "on": false, "status": "resting", "reason": null,
            "last_run_at": "2026-11-01T09:58:00Z" },
  "learning": true
}
```

- `room` is null without a sensor read in the last 30 s.
- A sensor's `rh`, `raw_rh`, `temp` and `at` are null before its first good
  read; `error` is the last read error, or null. `rh` = `raw_rh` + `offset`.
  The salt test uses `raw_rh`.
- `target` is null when none is set.
- `pump.status` is `running`, `resting`, `idle` or `stopped`. `reason` is set
  for `idle` (`no_target`, `no_data`, `at_target`, `above_max`) and `stopped`
  (`user`, `no_rise`, `switch_error`), else null.
- `pump.enabled` is false without a pump (`PUMP=none`), and the pump routes
  then answer 409. Without a pump, `pump` is
  `{enabled: false, on: false, status: "idle", reason: null, last_run_at: null}`
  (the one idle with a null reason).
- `pump.last_run_at` is when the last run started.
- `PUT /api/target` has the pump decide straight away, so the reply already
  shows the outcome (e.g. `running`). A run in progress finishes first.

### History

```json
{ "step": 300, "t": [1761984000, 1761984300], "rh": [80.9, 81.3], "temp": [18.5, 18.6] }
```

The room readings averaged into buckets: 1 min for 1 h (60 buckets), 5 min
for 24 h (288), 30 min for 168 h (336). `t` is the bucket start, a multiple of `step`, oldest first.
Empty buckets are left out; the client breaks the line where consecutive `t`
values are more than 2 × `step` apart.

### Pieces

**NewPieces**

```json
{ "form": "bowl", "size": "medium", "thickness": "medium", "dryness": 1.0,
  "label": "", "notes": "", "quantity": 3 }
```

`form`, `size`, `thickness` and `dryness` are required. The client uploads the
photo afterwards, with one `PUT …/photo` per created piece.

**Piece**

```json
{
  "id": 7, "number": 3, "form": "bowl", "size": "medium", "thickness": "medium",
  "label": "", "notes": "", "photo": true,
  "state": "flipped",
  "added_at": "…", "flipped_at": "…", "ended_at": null,
  "dryness": 2.6,
  "last_check": { "dryness": 2.4, "at": "…" },
  "estimate": { "status": "drying", "ready_at": "…", "ready_until": "…" },
  "checks": [ { "id": 31, "dryness": 1.0, "at": "…" } ]
}
```

- `dryness`: the estimated dryness now (one decimal) on the shelf; the last
  check once ended.
- `estimate`: null once ended. `ready_at`/`ready_until` as in §3 (null where
  §3 says so).
- `checks`: oldest first; present only where the table says "with `checks`".
- `GET /api/pieces` (on the shelf) is ordered: `past` first (by number), then
  `ready` by `ready_until`, then `drying` by `ready_at`; ties by number. With `?ended=1` it lists ended
  pieces, newest `ended_at` first.

**PiecePatch**: any of `label`, `notes` (strings; `""` clears; `null` is a 400),
`form`, `size`, `thickness`, `state`, `dryness`. All given fields are applied
in one transaction.

| `state` | from | effect |
|---|---|---|
| `flipped` | `in` | sets `flipped_at` |
| `in` | `flipped` | undo flip: clears `flipped_at` |
| `in` | `trimmed`, `discarded` | put back (§5); the result is `flipped` if it had been flipped |
| `trimmed` | `in`, `flipped` | ends it; check at `dryness` or 3.0 |
| `discarded` | `in`, `flipped` | ends it; check only if `dryness` is given |

Any other transition, including to the same state, is a 409. `dryness` is only
accepted together with `state: trimmed` or `discarded` (else 400). Adding a
check to an ended piece is a 409.

## 7. Code layout

Go: about 2,300 lines plus about 1,000 of tests. Flat and boring. Paths are
relative to `apps/humidity/`.

```
cmd/humidity/main.go     env config, open db and hardware, start the loops and
                         HTTP, catch SIGTERM, pump off on exit
internal/hw/             sht.go (SHT3x/SHT4x over i2c-dev, CRC, retries),
                         pump.go (gpiocdev relay: off at open, active-low option,
                         Pi 5 header chip found by its label pinctrl-rp1),
                         st7789_linux.go (the screen: spidev writes plus GPIO
                         data/command, reset and backlight lines),
                         screen.go (RGB565 packing, PNGScreen for a laptop),
                         sim.go (fake sensor + fake pump that raises its humidity)
internal/app/            db.go (schema, queries), loop.go (poll sensors every 5 s,
                         latest values, minute rows, daily backup), pump.go (the
                         controller), display.go (draws the screen with the Go
                         fonts), pieces.go (piece operations), http.go
                         (routes, handlers, static files)
internal/predict/        predict.go: the pure function in §3
web/                     the Vue app; embed.go embeds web/dist
```

Long-running goroutines besides HTTP: the sensor loop, the pump loop and the
display loop. The pump and display loops read the latest values from the
sensor loop (a mutex-guarded struct). The only interfaces are the three
hardware seams, `Sensor`, `Switch` and `Screen`, each with a real and a sim
implementation.

### Configuration (environment)

| Variable | Default | |
|---|---|---|
| `ADDR` | `:8080` | |
| `DATA_DIR` | `/data` | database, photos, backups |
| `SENSOR` | `sht4x` | `sht4x`, `sht3x` or `sim` |
| `SENSOR_ADDRS` | `0x44` | comma-separated I²C addresses on `/dev/i2c-1` |
| `PUMP` | `none` | `none`, `gpio` or `sim` |
| `PUMP_PIN` | | BCM GPIO number; required for `gpio` |
| `PUMP_ACTIVE_LOW` | | `true`/`false`; required for `gpio` (no default: a wrong guess runs the pump while the app shows it off) |
| `PUMP_PULSE` | `10s` | |
| `PUMP_REST` | `3m` | |
| `PUMP_MAX_RH` | `95` | |
| `SCREEN` | `none` | `none`, `st7789` or `sim` (writes `$DATA_DIR/screen.png`) |
| `SCREEN_DC` | `25` | BCM GPIO of the screen's data/command pin |
| `SCREEN_RST` | `27` | BCM GPIO of its reset pin |
| `SCREEN_BL` | `18` | BCM GPIO of its backlight pin |
| `SCREEN_FLIP` | `false` | `true` turns the picture upside down |

Everything else (poll interval, backups kept, I²C bus) is a constant.

### Build and deploy

- `make dev` (in `apps/humidity/`): backend with `SENSOR=sim PUMP=sim DATA_DIR=./data`, plus the
  Vite dev server proxying `/api`.
- `make test` (same place): gofmt, `go vet` (host and linux/arm64), Go tests, then the
  web app's type-check, lint and tests.
- Docker image and `docker-compose.yml` (repo root): reached through the
  gateway, `/dev/i2c-1` and `/dev/gpiochip0` mapped, `./data/humidity:/data`,
  logs capped at 3 × 10 MB. `deploy.sh humidity` (repo root) builds, pushes and
  restarts.

## 8. Web app

Vue 3 + Vite + TypeScript + vue-router. No Pinia, no chart library, no live
stream. About 2,100 lines in `web/src`:

```
api.ts          the §6 types and calls
live.ts         shared state, polling, the message bar (with Undo)
dryness.ts      dryness words ("Leather hard, a touch soft")
format.ts       labels, times, estimate and pump wording
views/          HomeView, PieceView, AddView, FinishedView, SettingsView
components/     ClimateCard, LineChart, PieceRow
styles/         dryness.css (the colours only this app shows)
```

- **Polling.** `GET /api/now` every 5 s while the page is visible; the piece
  list refreshes every 30 s and after every write.
- **Home `/`**
  - One climate card. Humidity in large type with the target beside it
    (tap it to open the slider with Save, Turn off, Cancel; "Off" with
    "Turn on" when there's none), and a 40–100 % scale bar under both:
    a fill for the humidity, a tick for the target. Then temperature and the
    reading's age (red when stale, unreachable or no sensor answers), the
    pump's status line with Run 1 s / Stop / Resume (no Stop while control
    is off and the pump is idle), and the history: small SVG charts
    (humidity with the target as a dashed line, and temperature),
    1 h / 24 h / 7 d.
  - Pieces on the shelf in the API's order: number, photo, form, dryness
    word, estimate. "Rough guess" while learning. Tapping a piece opens its
    page, where its events are (the list has no event buttons, on purpose).
  - "Add pieces" button. A "Finished" link.
- **Piece `/pieces/:id`**
  - Photo, number, then the estimate as a coloured block in large type:
    "Ready to trim around / Thu 5 Nov 20:11 / in 3 days · trimmable until …"
    (teal), "Ready to trim / Now / until today 21:12" (green) or "Past
    leather hard" (amber), plus "rough guess" while learning. Then specs and
    notes. Edit label/notes/form/size/thickness, and add or change the photo.
  - Right under the header, one card: the **dryness slider**, **Save check**,
    and large Flipped (or Undo flip) / Trimmed / Discarded buttons; once
    ended, Put back instead. Trimmed and Discarded send the slider's value
    **only if it was moved**; otherwise trim records 3.0 (so the model never
    learns from its own estimate) and discard records nothing. The buttons
    say what they'll record ("Trimmed at 3.0"), so no explanation is needed.
    Every event shows a message with Undo.
  - List of checks with delete. Delete piece. Deletes take two taps (the
    first arms the button) instead of a confirm dialog.
  - "Next piece →" walks the shelf in the order it had when the walk started
    from Home, so re-sorting after a check doesn't skip pieces.
- **Add `/add`**: photo, form, size, thickness, starting dryness slider,
  label, quantity. The last form/size/thickness are remembered on the phone.
  The photo is uploaded as is to each created piece.
- **Finished `/finished`**: ended pieces, newest first.
- **Settings `/settings`**: sensor offsets (showing `raw_rh`) with a one-line
  salt-test hint, backup download.

Photos load from `/api/pieces/{id}/photo?v=<something that changes after an
upload>`; the server sends `Last-Modified` (`http.ServeFile`).

**The slider** is the one place that gets care: the potter's feel is the
measurement. One `PreciseSlider` component, from `shared/ui/`, used for dryness (0–5, 0.1) and
the target (50–`max_rh`−1, 1): drag relative to the thumb, tap to jump, finer
steps while the finger is held below the track, big ± buttons with
hold-to-repeat, a readout in words ("Leather hard, a touch soft"), a ring at
the last value, and the 2.8–3.6 band marked. `touch-action: pan-y` lets the
page scroll.

Colours: the shared "tide" palette, light and dark. Labels and enums are
constants in the frontend.

## 9. Tests

- `predict`: table tests for the prior, `k`, the blend (including no movement,
  checks going down, checks under 1 h apart), and the outputs per status.
- Pump loop: fake clock and switch. Covers the pulse/rest cycle, each idle
  reason, the miss check and `no_rise` stop, Resume, manual run (also while
  stopped, refused above max), Stop during a pulse and during rest, a switch
  error, and off on exit.
- One HTTP test with the sim sensor and pump and a temp database: add pieces,
  photo, check, flip and undo, trim and put back, delete, target, pump run,
  history, backup, unknown `/api` path.
- SHT: CRC and decoding (kept from the first version).
- Web: unit tests for the dryness words and slider maths only.

## 10. Housekeeping notes

- Data from the first version isn't migrated. If it ever ran on the Pi, move
  its `/data/humidity.db` aside before the first deploy: the new schema won't
  create on top of it.
- The pump's safety stop is held in memory; a reboot clears it.
- The app's own daily backup and download don't include photos. The
  system-wide [off-device backup](../../architecture.md#data) does.
