# Design review: contract, frontend plan, backend plan

Reviewed: `00-domain-brief.md`, `mid-batch-changes.md` (adopted research),
`api-contract.md`, `frontend-plan.md`, `backend-plan.md`, and the current Go
code, Dockerfile, compose file and `deploy.sh`. Nothing was changed except this
file.

## Resolution (2026-10-31)

This review was written against revision 1 of the design (batches, covers, a
fan, the DHT22). The design docs are now at **revision 3**. The findings
below are kept as written for reference. This section records what happened
to each one. "Owner" marks a decision the owner made; the rest are the
author's calls, made with simplicity first.

**Owner decisions on the review**

| Item | Decision | Where |
|---|---|---|
| X1 HTTPS | **Rejected.** Plain HTTP over Tailscale. Features that need a secure context are dropped or simplified: no service worker or PWA (a manifest + icons give "Add to Home Screen"), and **no idempotency keys** (POSTs are never auto-retried, and duplicates are easy to fix). | Contract §1.2, FE §2 |
| F2 / decision 2: check-in control | **Sliders, and they must be precise.** Dryness is now a **continuous 0.0–5.0** value. A custom pointer slider with fine scrubbing, nudge buttons and a muddy-finger test comes first in F1. Check-in shows one piece per screen, so no slider sits in a scrolling list. | Research §2, FE §4.3, §5.1 |
| X3, C1, C2: the trim card | **Must be addressed thoroughly.** (Studio days and hours were added, then **removed the same day**: the owner goes whenever, and steers drying with the humidity target or plastic instead.) Studio time zone for display. `ready_until` expires "ready". Ready pieces count in visit planning. A **deadline-aware**, around-the-clock slot search returns one window. | Research §7, contract §3.7, FE §4.1, BE §8 |
| X6: deploys | **No rollback machinery. Docker logs are bounded** (10 MB × 3, already in `docker-compose.yml`). | BE §11 |
| Studio practice (new) | Throwing in **sessions**; the **device is off between them**. Offline periods are detected at startup, a "back on" banner and check-these follow, learning carries over between sessions, backups run at startup, and the device is switched off with the power button. | Research §6, BE §5 |
| Decisions 1, 5, 7, 9 | Earlier: no batches, SHT31/SHT4x, no fan, a learning period instead of reference times. | Rev. 2 |

**Everything else**

| Finding | Outcome |
|---|---|
| X2 clock after power-cut | **Adopted, and more important now.** Writes wait for clock sync and never go backwards; an RTC battery is recommended. |
| X4, C3, I2, B5, F4 | Moot: batches, covers and the fan are gone. |
| X5 off-device backup | **Simplified.** A "Download backup" link in Settings (`GET /backup`), plus a backup at startup and daily. No cron or litestream. |
| X7 sensor accuracy | Adopted: SHT31/SHT4x plus a salt-test offset. Heater pulse deferred. |
| X8 / decision 6 alerts | **Deferred.** Push needs HTTPS anyway; ntfy from the backend later. |
| S1 scope | Adopted in spirit. Cuts: events/covers/zones, three visits → one, what-ifs. Promoted: studio hours, live adaptation, offset, `ready_until`. The HTTPS promotion is rejected by the owner. |
| I1 milestones | Adopted: FE and BE milestones are numbered to match. |
| I3 mismatches | Fixed by the rev. 3 rewrite. |
| C4 skip = unknown | Adopted. |
| C5 moving the initial observation with `added_at` | Adopted. |
| C6 stdlib gotchas | Noted where relevant (DSN pragmas, write deadlines). Nothing else. |
| F1 model grading itself | Adopted, and stronger: check-in pre-fills with the **last observed** value and **hides the model estimate** entirely. |
| F3 windows wider than the mock-ups | Accepted. The trim card shows windows and a confidence word, never minutes. |
| F5 follow-up count | Fixed: at most one per piece, shown on the check-in card. |
| F6 offline/PWA | Replaced: no service worker. An open tab shows the cached last state; the app can't load while the device is off (an accepted HTTP trade-off). |
| F7 long-press | Adopted: a Select button. |
| B1 live adaptation in v1 | Adopted (`a_studio`, `e_recent`, `e_sibling`, `e_piece`). `e_recent` now uses the last 10 pieces instead of the last 7 days, because of the breaks between sessions. |
| B2 gap fill | Adopted: the 24 h mean before the gap, only when pieces were on the shelf. |
| B3 anchor bias | Moot: continuous dryness anchors at the observed value. |
| B4 follow-ups in cold start | Adopted: `much_slower` is off during learning. |
| B6, B7, B8, B9, B11 | Adopted. |
| B10 sensor names are permanent | Adopted (documented). |
| B12 build details | Partly adopted: ldflags version, `embedweb` stub, `DEMO_SEED` guard. No image tags. |
| Decision 4 backup target | The download link (above). |
| Decision 8 signal at the shelf | Still unknown. Check-in saves **per piece**, so a dropped signal loses at most one reading. No offline queue. |
| Decision 10 studio mates | Deferred. |
| Decisions 11–17 (contract open points) | Moot (batches and events are gone), except 13 (thresholds backend-only: yes) and 17 (band constant: yes). |

## TL;DR

1. **Blocker: there is no HTTPS.** Over `http://<pi>:8080` the browser has no
   secure context. That means no service worker (so no PWA install and no
   offline shell) and no `crypto.randomUUID()` (which the FE's idempotency keys
   depend on). The compose file also publishes 8080 on every interface, so the
   unauthenticated API is open to anyone on the workshop Wi‑Fi, not just the
   tailnet. Fix both with `tailscale serve` and a port bound to `127.0.0.1` (X1).
2. **Check-in can train the model on its own guesses.** Each row is pre-filled
   with the *predicted* stage and has a one-tap ✓ "confirm". With muddy hands
   people tap ✓, so the observations echo the model. A list of ~20 range
   sliders is also a poor control on iOS Safari. Pre-fill from the last
   observation instead, and use big `Same · +1 · other` buttons (F1, F2).
3. **The hero "come trim at X" card can be wrong in both directions.** It
   doesn't know the studio's time zone or when the potter can actually come
   (it can recommend 03:00). `ready_now` never expires, so a piece observed
   leather hard three days ago still says "ready now". And ready pieces are
   left out of visit planning (X3, C1, C2).
4. **Several prediction defaults all make ETAs late, and late is the costly
   direction (pieces too dry):**
   - long sensor gaps count as floor VPD, so progress freezes (B2);
   - after a stage increase, progress is anchored at the bottom of the stage
     band (B3);
   - the adaptation layers (`e_batch`, `a_studio`) are scheduled for B5, so
     the first weeks run on guessed priors (B1);
   - follow-up questions fire on prior error, not on unlogged covers (B4).
5. **Data safety is thin for "history is the product".** Backups live on the
   same SD card, no restore has ever been tested, there is no off-device copy,
   no snapshot before migrations, deploys only use the `latest` tag (no
   rollback), and Docker logs are unbounded (X5, X6).
6. **Clock and sensor accuracy are taken for granted.** There is no RTC
   battery, so timestamps are wrong after a power cut and readings can collide
   on the `(sensor, minute)` primary key. DHT22 error at high RH is amplified
   2–6× in VPD, and there is no calibration offset (X2, X7).
7. **Carry-over will be the normal case** for a potter who throws daily.
   Contract validations break on carried pieces (C3), and "time since the batch
   started" loses its meaning. The owner needs to decide what a batch is (X4,
   Decision 1).
8. **Scope.** Cut sensor zones/Move, flip, event redo/PATCH, 3 visits →
   1, the 3‑way what-if → 1, and the piece drying-curve chart. Promote HTTPS,
   off-device backup, studio hours, live adaptation and calibration into v1
   (S1). The FE and BE milestones also don't line up (I1).

**What's solid:** the core model (per-piece clocks, events kept separate from
corrections, dose = ∫VPD, cover treated as a state) is right and well argued.
The VPD formula and `VPD_ref` check out (Tetens/FAO‑56: 0.818 kPa at 20 °C/65 %).
The contract is unusually precise: one error envelope, an explicit null
policy, readings returned as columns ready for uPlot, and SSE used as an
invalidation signal rather than a data channel. The partial unique index for
"one active batch" is valid SQLite.

**Counts:** 1 blocker · 17 major · 15 minor · 4 nit (37 findings).

---

## Cross-cutting

### X1 · blocker · FE §2 (PWA), FE §6 (idempotency), contract §1.2, `docker-compose.yml`. No secure context, and the API is exposed on the LAN
- **Problem:** The app will be opened as `http://pi:8080` or `http://100.x.y.z:8080`.
  Service workers and `crypto.randomUUID()` only work in secure contexts
  (HTTPS or localhost). Separately, `ports: "8080:8080"` binds `0.0.0.0`, so
  the API is reachable from the workshop LAN as well as the tailnet. The brief
  assumes "tailnet only, no auth".
- **Failure:** `vite-plugin-pwa` registers nothing, the app can't be installed
  and has no offline shell, and the FE's key generator throws on the first
  POST. Meanwhile any device on the studio Wi‑Fi can `POST /batches/7/end`.
- **Fix:** Bind compose to `127.0.0.1:8080:8080`. Put `tailscale serve --bg
  http://127.0.0.1:8080` on the host (HTTPS on `pi.<tailnet>.ts.net`; turn on
  HTTPS certificates in the admin console). Document this in BE §10, and check
  that SSE is not buffered through `serve`. Have a non-crypto UUID fallback
  for dev.

### X2 · major · contract §1 "Clock", §1.1 time validation; BE §5; `dht22.go` `At: time.Now()`. Wall-clock trust after a power cut
- **Problem:** The Pi 5 RTC doesn't keep time without its battery. After a
  power cut the clock is wrong until NTP syncs, and a workshop outbuilding is
  exactly where power cuts and missing network happen. Readings are stamped
  with `time.Now()`.
- **Failure:**
  - If the clock restarts in the past, the recorder writes `(sensor, minute)`
    keys that already exist. A plain INSERT then fails, and an
    `INSERT OR REPLACE` overwrites real data.
  - The dose prefix sum assumes minutes only go forward.
  - The phone's "now" is ahead of the Pi's, so writes are rejected as
    `in_future` (the server only allows 2 minutes).
  - Elapsed timers go negative.
- **Fix:** Fit the ~€5 RTC battery and say so in the deploy docs. While the
  clock is unsynced (`unix.Adjtimex` with modes=0, `STA_UNSYNC` flag, readable
  without privileges), the recorder holds writes. It never writes a minute
  older than the last one it persisted, and uses `ON CONFLICT DO NOTHING`. On
  the FE, "Now" sends no time at all (the server fills it in), and the time
  chips are computed with the server-clock offset.

### X3 · major · contract §3.8 `visits`, §5.5 "quiet hours"; BE §10 `TZ`; FE §4.3 `WhenChips`. No studio time zone, and no idea when the potter can come
- **Problem:** "When should I come to the workshop" is the core requirement,
  but the server is pure UTC. It has no studio time zone and no availability
  window, and quiet hours are still an open point. `debian:bookworm-slim` ships
  without tzdata, so `TZ` silently does nothing in Go. "This morning" is
  undefined. The `1d` and `6h` reading buckets are aligned to UTC midnight.
- **Failure:** The trim card says "Come trim 03:10–05:40". The earliest time
  when k pieces are in the band is often in the middle of the night, and the
  useful answer ("tomorrow 08:00; #3 will be slightly past by then") is never
  computed.
- **Fix:** Add `STUDIO_TZ` (and import `time/tzdata`) and `STUDIO_HOURS`
  (e.g. `07:00-22:00`, one setting). Restrict `visits` to those hours. Define
  "this morning" as 08:00 local, hidden before 09:00. Align day buckets to
  local midnight.

### X4 · major · brief "Batch"; research §4.4/§7; contract §5.4; FE §4.1. The batch timer stops meaning anything once carry-over is routine
- **Problem:** The research assumes daily throwing in one shared space. Under
  that assumption every "End batch" carries pieces over, and the batch becomes
  an arbitrary interval between two taps. The owner's requirement, "time since
  batch started", then measures time since the last End tap, not the age of
  anything on the shelf. None of the docs says when or why the user ends a
  batch.
- **Failure:** Either the user never ends a batch (one batch lasts months, and
  its climate summary and "History" become meaningless), or they end it at
  random. Either way, the timer on the hero screen is noise.
- **Fix:** Owner decision (Decision 1). Whatever the answer, show each group's
  age on piece rows ("Tue mugs · 1d 4h"). Prompt "All pieces out, end the
  batch?" when the last piece is removed, and make that the normal way a batch
  ends.

### X5 · major · BE §4 "Backups"; brief "Readings history". Backups can't survive the failure they exist for
- **Problem:** `VACUUM INTO /data/backups/…` writes to the same SD card as the
  database. No restore procedure exists or has been tested, and there is no
  export. `VACUUM INTO` also fails if the target file exists, which happens
  when the process restarts on the same day and the daily name repeats.
- **Failure:** SD card wear-out or corruption, the most common way a Pi fails,
  takes out the database and all 14 backups together. That is months of
  calibration data gone.
- **Fix:** Get a copy off the device: add `GET /api/v1/admin/backup`, which
  streams a fresh `VACUUM INTO` temp file, and pull it from a laptop with cron
  (or use litestream to a bucket). Add a test that restores a backup and runs
  `PRAGMA integrity_check` plus a row count. Name files with a timestamp and
  write to a temp name before renaming. A JSON/CSV export comes almost free
  with this.

### X6 · major · BE §4 migrations, §10; `deploy.sh`; `docker-compose.yml`. Live deploys and migrations have no way back
- **Problem:** Migrations only go forward and run at startup, with no snapshot
  taken first. The image is only ever tagged `latest`, so there is no previous
  image to roll back to. Compose has no logging limits, and the default
  json-file driver grows without bound on the SD card.
- **Failure:** A bad migration or crash loop during a running batch, combined
  with `restart: always`, leaves the potter with no app and no old image that
  can read the new schema. Logs slowly fill the card.
- **Fix:** Before applying pending migrations, run `VACUUM INTO
  pre-migrate-<from>-<to>.db`. Tag images with the git SHA as well as
  `latest`, and have `deploy.sh` record the previous tag so a rollback is one
  command. Add `logging: {driver: json-file, options: {max-size: "10m",
  max-file: "3"}}`. Deploying mid-batch is otherwise fine: the gap is under a
  minute and the SIGTERM flush is already planned.

### X7 · major · research §1/§5.1; BE §3/§5; contract §3.2. DHT22 error is amplified at high RH, and there is no calibration
- **Problem:** The DHT22 is ±2 %RH typical and up to ±5 % near the extremes.
  It drifts upward over time and can stick near 99 % after condensation.
  VPD ∝ (100 − RH), so the relative error grows as RH rises:

  | RH | RH error | VPD error |
  |---|---|---|
  | 80 % | ±2 % | ±10 % |
  | 80 % | ±5 % | ±25 % |
  | 90 % | ±3 % | ±30 % |
  | 95 % | ±3 % | ±60 % |

  A damp drying room, which is where the control loop is heading, sits in
  exactly this range. `a_studio` absorbs a constant bias but not drift over
  months or a sensor swap. Sensor placement is not mentioned anywhere: next to
  the future humidifier's plume, on a wall, or on a different shelf from the
  ware all give misleading readings.
- **Failure:** Dose in a damp room is off by 30 % or more. Learned rates
  quietly move when the sensor drifts or gets replaced.
- **Fix:** Add a per-sensor offset (`SENSOR_OFFSETS=main:+1.8`) applied in the
  monitor, a one-paragraph salt-test procedure (NaCl ≈ 75 %RH) in Settings,
  placement guidance (at ware height, on the same shelf, away from the
  humidifier), and a `sensor_replaced` system event. If the room will run
  above 80 %, consider an SHT31/SHT4x (similar price, I²C, much better at high
  RH).

### X8 · minor · FE §10 "Deferred"; research §6.6 "no nagging". No signal reaches the user when something goes wrong while they're away
- **Problem:** The sensor dying, RH collapsing (door left open, heater) or
  pieces passing leather hard are only visible if the user opens the app.
  Notifications are deferred, and the research rules out reminders.
- **Failure:** The DHT22 dies on Friday evening. Every ETA through the weekend
  rests on the forecast, and the potter learns about it on Monday. Or the room
  drops to 35 % overnight and rims crack.
- **Fix:** Keep "no nagging" for logging prompts, but treat alarms as a
  separate thing. In v1.5, have the backend POST to ntfy or Pushover for three
  things: sensor down for more than 1 h, RH outside a band, and "pieces likely
  past leather hard within 3 h". That is one outbound HTTP call, with no Web
  Push or VAPID setup and no iOS PWA requirement. See Decision 6.

### S1 · major · all three plans. v1 is larger than "simple" calls for; some things are cut in the wrong direction
The core loop is: see the room → add pieces → record stages → see "come
trim at X" → mark trimmed, plus cover, which the research is right to call
essential.

**Cut or defer (no loss to the core loop):**

| Item | Why |
|---|---|
| `sensor_set` / Move, `back_in_sensor_zone`, piece `sensor` UI | Only one sensor is installed (`SENSORS=main:4`). Keep the kind name reserved. |
| `flipped` event | Explicitly not modelled. A `note` covers it. |
| `PATCH /events/{id}`, `POST /event-ops/{op_id}/undelete` | Undo plus re-log covers both; redo is never needed. |
| `GET /batches/{id}/observations` (paginated) | No screen calls it (contract §2 "Screen → calls"). |
| `GET /pieces/{id}/prediction` | Already embedded in Piece. |
| `visits` up to 3 → 1 | A second and third visit built on rough priors is false precision. |
| `what_if` for 3 levels → just `eta_if_uncovered` | The research itself puts the full what-if toggle in v1.5 (§6), while its TL;DR #8 keeps a cheap what-if in v1. The single number "~Sat 10:00 if uncovered now" carries the value. |
| `maybe_covered` follow-up | Fires on prior error until calibrated (B4). |
| Piece drying-curve chart (FE §4.6) | The most complex chart in the app, on a secondary screen. An observation list plus the ETA card is enough for v1. |
| SSE `changed` scopes/`piece_ids` | Payload-less "refetch what's visible" is simpler (the BE plan already assumes it, I3). |
| Idempotency "in flight" 409 and "reused key" 422 | Store the key and the response in the same transaction as the write; nothing more. |

**Promote into v1:** HTTPS (X1), off-device backup and restore test (X5),
pre-migration snapshot and tagged images (X6), studio TZ and hours (X3),
live `e_batch` and `a_studio` (B1), sensor offset (X7), `ready_now` decay (C1).

### I1 · minor · FE §10, BE §12. The milestones don't line up
- F1 promises "live humidity" over SSE, but the BE puts the SSE hub in B3
  ("live-update signal"). B1 should ship `/stream` with `hello` and
  `readings`.
- F3 (piece detail, undo by op) and F4 (climate markers) call
  `GET /batches/{id}/events?piece_id=` (contract §2, screen → calls), but
  the BE ships the "batch timeline endpoint" in B5.
- Where v1 ends is ambiguous. The FE says F5 = v1. The BE says "v1 ships
  … without `fit.go`" but puts `fit.go` in B5, which unblocks F5.
- **Fix:** Use one shared milestone table in the contract. Put the events GET
  in B3 and SSE `readings` in B1, and state that `fit.go` is post-v1 (with
  `e_batch` and `a_studio` pulled into B4, per B1).

### I2 · minor · BE §4 "Carry-over" vs contract §3.5 `number`. The renumbering clause can never trigger
The BE plan says carried pieces are "renumbered if not free". But carry-over
always creates a fresh batch, so their numbers are always free. The contract
("carried-over pieces keep theirs") is right. Delete the BE clause and its
"(see review)" note so nobody implements renumbering. (Side effect worth
knowing: if batches chain forever, numbers climb into the hundreds. That's
acceptable.)

### I3 · nit · various. Small mismatches to clean up in one pass

| Topic | A | B |
|---|---|---|
| Quantity limit | FE §4.3: 1–20 | contract §3.5: 1–50 |
| `changed` payload | contract §4.2: `scopes`, `batch_id`, `piece_ids` | BE §8: "payload-less" |
| ID allocation | contract §1: `AUTOINCREMENT` | BE schema: `INTEGER PRIMARY KEY` (rowids can be reused) |
| `op_id` type | contract §5.2: integer | BE schema: `TEXT`; neither says where it's allocated |
| Idempotency record | contract §1.2: method, path, body hash, in-flight state | BE schema: key, response, status, created_at |
| "Check these" | contract §3.8: server `check_these` | FE §4.5 also computes "last observation is old" itself |
| Batch detail | FE §4.8: "read-only" | …but it hosts the Fan toggle and End batch |
| End-batch options | FE §4.8: "discard" | contract §5.4: removal reasons only |
| `LogEvent` sheet | FE §3/§7 lists it | no screen describes it |

---

## Contract (`api-contract.md`)

### C1 · major · §3.7 status table, §3.8 `visits`. `ready_now` never expires, and ready pieces are left out of visit planning
- **Problem:** `ready_now` means "the last observation is `leather_hard`",
  whatever its age, and it has `eta: null`. Visits only plan `pending` pieces.
- **Failure:** A mug observed leather hard on Monday still shows "**3 pieces
  ready now**" in the accent colour on Thursday, when it's bone dry. Ready
  pieces have the tightest deadline, yet nothing tells the user "come before
  Fri 18:00 or #3 and #5 go past".
- **Fix:** Re-anchor at p = 3 and keep predicting. Report `ready_now` only
  while p < ~3.5, then switch to `may_be_past_check`. Give ready pieces a
  `ready_until` time and fold it into the first visit's `end`. The trim card
  then reads "3 ready now · best before Fri 18:00".

### C2 · major · §3.8 visit algorithm (from research §5.2 step 5). The visit window is vague exactly where the work is
- **Problem:**
  - It doesn't say whether "at least k pieces predicted in [2.8, 3.6]" uses
    point estimates or the 80% windows. Point estimates give a falsely narrow
    `start`/`end`.
  - "The first group" is undefined: the first added, or the first to cross?
  - Covered pieces are excluded from k, so a shelf that is mostly wrapped
    gets no visit at all.
  - Studio hours are ignored (X3).
- **Failure:** Two implementers would build different algorithms. The trim
  card shows a tight window built on numbers with ±60 % uncertainty.
- **Fix:** Define it as: candidate times = each hour inside `STUDIO_HOURS`
  over the next 72 h. Score each candidate t by the number of pieces with
  `eta_low ≤ t ≤ ready_until` (covered pieces included, using their current
  cover). Recommend the earliest t with the maximum score. `start`/`end` is
  the contiguous span with the same score. Return one visit.

### C3 · major · §3.4 PATCH `started_at`, §5.2 `occurred_at` "within the batch", §3.6 `batch_id`. The validations break carried pieces
- **Problem:**
  - "`started_at` must not be after any piece's `added_at`" is false the
    moment a batch has a carried piece, since their `added_at` comes before the
    new batch's start. So PATCHing `started_at` always fails on such batches.
  - Events on a carried piece backdated to before the carry ("I wrapped them
    this morning", carry happened at noon) are rejected as outside the batch.
  - A backdated observation's `batch_id` ("batch at record time") puts it in
    the new batch even though `observed_at` falls in the old one.
- **Failure:** With daily throwing, carried pieces are the normal case (X4),
  so these 422s will hit routine actions.
- **Fix:** Validate piece-level times against the piece's clock only. Exclude
  carried-in pieces from the `before_batch_start` check. Derive an
  observation's `batch_id` from `observed_at` (or drop the column; it's
  derivable).

### C4 · minor · §3.6 FollowUp "no answer is treated the same way". Skipping counts as confirmation
- **Problem:** Skip is the default with muddy hands. Treating a skip as
  `its_fine`/`just_slow` means unexplained backwards jumps and big lags
  "confirm the sample". That undercuts research §2(d), which relies on robust
  training rather than on answers.
- **Fix:** Treat a skip as *unknown*. For `went_wetter`, exclude the pair from
  training (as with `inconsistent`). For `maybe_covered`, keep the sample at
  half weight. Let the 3σ outlier rule handle the rest.

### C5 · minor · §3.5 PATCH `added_at`. The initial observation and cover event aren't moved with it
- **Problem:** "`added_at` must stay ≤ the first observation." But the first
  observation (`is_initial`) *is* at `added_at`, as is the initial `cover_set`.
  So moving `added_at` later always fails, and moving it earlier leaves them
  behind.
- **Fix:** State that PATCH `added_at` moves the `is_initial` observation and
  the add-time `cover_set` (same `op_id`) together. Validate against the first
  *non-initial* observation.

### C6 · nit · §1 conventions, §3.3. Cheap stdlib gotchas
- Go 1.22 `ServeMux` answers a wrong method with a plain-text
  `405 Method Not Allowed`, not the JSON envelope. Add 405
  `method_not_allowed` to §1.1 and wrap the mux.
- `from=-7d`: `time.ParseDuration` has no `d` unit, so it needs a small custom
  parser.
- Consider an optional per-piece `target_stage` for later (handles are
  attached at soft leather hard). The contract's additive rule allows it. No
  action is needed now.

---

## Frontend plan (`frontend-plan.md`)

### F1 · major · §4.4 check-in pre-fill and ✓. The model learns from its own predictions
- **Problem:** Every row is pre-filled with the *predicted* stage, and ✓
  "confirm as is" records it as a real observation. The plan only protects
  untouched rows. ✓-confirmed rows become training data.
- **Failure:** A tired potter ✓s the whole shelf. The observations equal the
  model's output, the crossing times equal the prior, and the "learned" rates
  converge to the defaults. Error tracking shows the model as accurate because
  it is grading itself. This is the main noise source the research warns
  about, made systematic.
- **Fix:** Pre-fill with the *last observed* stage. That bias is "no change",
  which is conservative and easy to detect. Show the prediction as a small
  ghost tick on the scale. If the guess pre-fill stays, store
  `source: "confirmed_prediction"` and down-weight it in `fit.go`.

### F2 · major · §4.4 and §5 `StageSlider`. A list of sliders is the wrong control for muddy hands on a phone
- **Problem:** On iOS Safari, tapping the track of `<input type=range>` does
  *not* move the thumb; you have to grab and drag it. Twenty horizontal
  sliders in a vertically scrolling list get changed by accident while
  scrolling. Each row also carries ✓, a cover icon, an overflow menu,
  multi-select and a sticky footer. That's a lot for the screen used "standing
  at the shelf".
- **Failure:** Wrong stages get saved without anyone noticing, and the user
  gives up on check-in after a week. Check-in is the only source of data, so
  predictions never improve.
- **Fix:** Use three large buttons per row: **Same** · **+1** · **…** (the
  last opens the 6-stage picker). Almost every check-in is "same" or "+1".
  Keep the slider in Add pieces and Piece detail, where the owner asked for
  it. Move cover and remove out of the check-in rows into piece detail and
  the Pieces multi-select. See Decision 2.

### F3 · minor · §4.1 trim card mock vs research §5.2 σ. Windows will be much wider than the mock-ups
- **Problem:** At cold start σ_class = 0.4 means the window spans ×0.60 to
  ×1.67 of the remaining time. With 24 h remaining that is about 14 h to 40 h,
  not "Sat 09:00–13:00".
- **Failure:** The hero card reads "Fri 20:00 – Sun 04:00", which looks broken
  even though it's honest.
- **Fix:** Design the card for wide windows. Lead with "check from Fri
  evening" (`eta_low`), then "most likely Sat morning" (`eta` as a day-part).
  Never show minutes. Show the window as a bar rather than as two clock times.

### F4 · minor · §4.8 Fan toggle and End batch under History; Fan in F5. Required logging is hidden two levels deep
- **Problem:** The research lists fan as one of only four *required* logging
  actions. The plan puts it on the batch detail screen under the History tab,
  ships it in F5, and its own rationale for putting cover in F3 applies
  equally to fan. End batch is buried in the same place.
- **Fix:** Put a Fan chip on the Now batch strip, but only if the owner uses a
  fan (Decision 7). Put End batch in the batch strip's ⋯ menu. Move fan to F3.

### F5 · minor · §4.4 vs §6 follow-ups; contract §3.6. "One per save" vs "all of them, one at a time"
- **Problem:** FE §4.4 says at most one follow-up per save. FE §6 and the
  contract say a bulk save returns them all and the FE shows them one at a
  time with "skip all". The server marks each question as asked when it
  returns it, so any the FE doesn't show are used up without being seen.
- **Fix:** Have the server return at most one follow-up per request (the one
  most informative for training) and mark only that one as asked.

### F6 · minor · §6 offline and §2 PWA. "Last status shown" and shell updates are underspecified
- **Problem:** An app-shell-only service worker doesn't keep the last
  `/status`; the FE has to save it to `localStorage`. On iOS standalone PWAs, a
  waiting service worker can serve a stale shell for days.
- **Fix:** Persist the last `/status` and pieces list with a timestamp. Use
  `registerType: 'prompt'` with `useRegisterSW` (`needRefresh` → the reload
  banner the contract already plans). Configure no `runtimeCaching` for
  `/api/` and set `navigateFallbackDenylist: [/^\/api\//]`. If there is no
  reliable signal at the shelf, an offline queue *for observations only* moves
  into v1 (Decision 8).

### F7 · nit · §4.5 long-press multi-select. Hard to discover, and it fights iOS
Long-press has no visual cue, and on iOS it triggers the text-selection
callout unless you set `-webkit-touch-callout: none; user-select: none`. Add
an explicit "Select" button in the list header and keep long-press as a
shortcut.

---

## Backend plan (`backend-plan.md`)

### B1 · major · §7 "`fit.go` lands in B5", including `e_batch` and `e_piece`. The first weeks run on invented priors
- **Problem:** The research calls the batch effect "usually the biggest live
  improvement", and it is plain arithmetic over the current batch with no
  persistence. Yet the plan bundles it with the cross-batch fit in B5. Until
  then, live ETAs are the round-number priors plus re-anchoring.
  `a_studio` ("your studio is 1.6× slower") only needs about 3 samples.
- **Failure:** If the studio is twice as slow as the defaults, every trim card
  is early by half the drying time until B5 ships and enough batches finish.
  The potter stops trusting the card in the first week.
- **Fix:** Move `e_batch`, `e_piece` and `a_studio` into B4 (`live.go`). They
  are a few lines each, computed on read with no `model_params` table needed.
  Thickness and size factors, cover multipliers and persistence stay in a
  later `fit.go`. Also ask the owner for 2–3 real reference times to set the
  priors before shipping (Decision 9).

### B2 · major · §5 "longer gaps contribute the floor VPD". Sensor outages freeze progress
- **Problem:** Research §7 says to carry the last good VPD for gaps under 1 h
  and mark longer stretches disturbed. The BE plan instead treats gaps over
  1 h as floor VPD, which is roughly no drying at all. That is a deviation
  from the research. DHT22 outages of hours are common, and power cuts are a
  "gap" with no recorder running.
- **Failure:** The sensor is dead for a day. The live model thinks the pieces
  barely dried, so the ETA slips by a day, and the potter arrives to
  hard-leather-hard pots. Crossing samples spanning the gap also come out as
  "fast" (less dose for the same progress).
- **Fix:** Fill long gaps with the forecast-basis VPD (mean of the surrounding
  6–24 h), widen σ through `sensor_gap`, and exclude the segment from
  training. On startup, detect a process-down gap from the last persisted
  minute and write the `sensor_gap` event then. Nothing else can write it.

### B3 · major · §7 `live.go` (research §5.2 step 1). Anchoring at the bottom of the band biases ETAs late and makes near-term windows too narrow
- **Problem:** On a stage increase the anchor is set to `p = stage` ("it just
  crossed"). That only holds if checks are frequent. With one check a day,
  the piece could be anywhere in the band. The window also only models
  log-rate σ, and that shrinks to zero as the remaining time shrinks, so the
  ±0.5-stage position uncertainty never shows up.
- **Failure:** The user observes "soft leather hard" once a day. The model
  assumes the piece entered that stage just now, so it predicts leather hard
  up to a full stage-width late, and late is the costly direction. When a
  piece is near its target, the window shows ±20 minutes when the honest
  answer is ±3 hours.
- **Fix:** Use the same-stage rule for increases too:
  `p = clamp(predicted_p_now, stage, stage + 0.9)`. Add the anchor position
  (uniform over the band, roughly ±0.3 stage at 80 %) to the window by
  converting it to time at the current rate.

### B4 · major · §7 `followup.go` (contract §3.6 `maybe_covered`). Follow-ups fire on prior error during cold start
- **Problem:** "≥ 1.5 stages behind the prediction, piece uncovered" is
  measured against uncalibrated priors. In a studio slower than the defaults,
  it fires for most pieces, and once per piece per batch uses up the one
  question that piece was allowed.
- **Failure:** "Slower than expected — was it covered?" pops up on every
  check-in in the first weeks. The user learns to skip it (and per C4, skips
  count as confirmations), and the prompt is worthless by the time it would
  be accurate.
- **Fix:** Disable `maybe_covered` until `a_studio` has at least 3 samples,
  and measure lag against the `e_batch`‑adjusted prediction. `went_wetter`
  doesn't depend on the model and can ship in v1.

### B5 · major · §4 "`cover`/`sensor` columns … only written … when it appends a `cover_set` event". Undo leaves the stored state stale
- **Problem:** The denormalized `pieces.cover` is updated only on append.
  Undo (`DELETE /event-ops`), `PATCH /events`, soft delete, and a backdated
  event inserted *before* the latest one all change the derived state without
  touching the column. The contract (§5.2) says "recomputed after every event
  write, edit or delete", so the two docs disagree.
- **Failure:** The user wraps 6 mugs, taps Undo, and the pieces still show
  "Wrapped" and predict 20× slower. The cover toggle is the main mid-batch
  control, so this is a common path.
- **Fix:** Drop the `cover`/`sensor` columns and derive them on read (tens of
  pieces, one indexed query). If they stay, recompute them in the same
  transaction after every event mutation. Same for `removed_stage`.

### B6 · minor · §7 "Forecast dose … mean VPD of the last 6 h". The forecast follows the daily cycle
- **Problem:** A 6 h mean taken at 17:00 includes a heated, dry afternoon,
  then gets projected across the night and the next two days.
- **Fix:** Use the 6 h mean for the first 6 h of the forecast and the 24 h
  mean after that. Keep the `forecast_basis` string honest ("assuming the room
  stays like the last day").

### B7 · minor · §7 "Accuracy tracking … store the prediction made at its previous observation". Recomputing later doesn't give what was shown
- **Problem:** Predictions are computed on request and never stored.
  Recomputing "the prediction at the previous observation" afterwards uses
  readings and model parameters from later, so it overstates accuracy.
- **Fix:** Add `prediction_snapshots(piece_id, observation_id, at, eta,
  eta_low, eta_high, method)`, written on every observation insert. It's also
  the dataset for tuning the priors.

### B8 · minor · §3 `Monitor.Subscribe` vs contract §4.2 `readings`. Per-reading fan-out can't express "this cycle failed"
- **Problem:** `Subscribe` sends only successful readings. But SSE `readings`
  is "after every poll cycle", and it has to include sensors that just failed
  so the FE "flips to stale without a timer".
- **Fix:** Have the monitor publish a per-cycle snapshot
  (`[]SensorStatus` with health). The recorder takes the readings from it and
  the hub forwards it as-is.

### B9 · minor · §4 store. SQLite and `database/sql` gotchas worth writing down
- With `SetMaxOpenConns(1)`, calling `db.Query` (rather than `tx.Query`)
  inside a transaction deadlocks. The `Service` needs a single `Querier`
  interface used everywhere.
- In modernc, pragmas must go in the DSN (`?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)…`)
  so every new connection gets them.
- SQLite table-rebuild migrations need `PRAGMA foreign_keys=OFF` *outside* the
  transaction, where the runner as described would put it inside, so plan
  for a per-migration "no-tx" flag.
- `VACUUM INTO` refuses an existing target file (see X5).

### B10 · minor · §3 config, contract §3.2. Sensor names are permanent keys, but nothing says so
Readings, `pieces.sensor` and events all key on the name from `SENSORS`.
Compose currently uses `main`, while every contract example uses `rack1`.
Renaming it orphans all history and every dose calculation that relies on it.
Document that names are permanent IDs and add a display label
(`SENSORS=main:4:Drying shelf`) if prettier names are wanted.

### B11 · minor · §8 SSE and the server. Shutdown and timeouts need specific stdlib handling
`http.Server.Shutdown` waits for handlers to return, and an SSE handler never
does. Cancel the hub's clients through `srv.RegisterOnShutdown` (or the hub's
context) first. Instead of having no `WriteTimeout` at all, keep a global
one and clear it only on `/stream` with
`http.NewResponseController(w).SetWriteDeadline(time.Time{})` (Go 1.20+).

### B12 · nit · §10 build; Dockerfile; `.dockerignore`. Build details
- `/health` promises `version`, `commit` and `built_at`, but the Dockerfile
  sets no `-ldflags -X`, and `.dockerignore` excludes `.git`. Pass them as
  build args from `deploy.sh`.
- Set `CGO_ENABLED=0` and `--platform=linux/arm64` on both stages. A build on
  an x86 machine otherwise produces an amd64 base image with an arm64 binary.
- A committed placeholder `web/dist/index.html` gets wiped by Vite's
  `emptyOutDir` on every build. Use a `//go:build !embedweb` stub file
  instead.
- Have `DEMO_SEED` refuse to run when `SENSOR_DRIVER=dht22`.

---

## Decisions for the owner

1. **What is a batch for you?** Do you dry discrete sessions (throw on
   Saturday, trim on Monday), or is the shelf continuously topped up? *Recommendation:*
   keep the batch timer (it's your requirement). Prompt "end the batch?" when
   the shelf empties, and show each group's age on piece rows. If you throw
   daily, accept that carry-over is the normal path and fix C3 before F2.
2. **Check-in control:** slider rows, or `Same · +1 · …` buttons?
   *Recommendation:* buttons in check-in, the slider in Add and Piece detail,
   and pre-fill from the last observation, not the prediction (F1, F2).
3. **When can you realistically be in the workshop?** *Recommendation:* one
   `STUDIO_HOURS` setting (e.g. 07:00–22:00) plus `STUDIO_TZ`, used for every
   visit recommendation (X3). This also settles the contract's "quiet hours"
   point.
4. **Where do off-device backups go?** A laptop pulling a nightly snapshot
   over Tailscale, or litestream to a cloud bucket? *Recommendation:* laptop
   cron pulling `GET /admin/backup`, plus a restore test in CI (X5).
5. **Sensor:** keep the DHT22, or swap it for an SHT31/SHT4x before history
   starts accumulating? *Recommendation:* swap it if the room will run above
   80 %RH. Either way, do one salt test and set an offset (X7).
6. **Alerts:** add ntfy/Pushover alarms (sensor down, RH out of band, pieces
   about to pass leather hard) in v1.5? *Recommendation:* yes, alarms only, no
   logging reminders (X8).
7. **Do you use a fan?** If not, drop `fan_set` from v1 entirely. If you do,
   put it on the Now screen in F3 (F4).
8. **Is there reliable signal at the drying shelf?** If not, an offline queue
   for observations only moves into v1 (F6).
9. **Priors:** give 2–3 real times from your studio (e.g. "a medium mug, wet
   → leather hard, takes about X h in winter"). *Recommendation:* set the
   thickness table from them before shipping B4 (B1).
10. **Studio mates:** will other people's pieces share the space? If yes, add
    an `owner` on pieces and a "mine" filter. If they only look, defer.
    *Recommendation:* defer unless the pieces are shared.

**Contract §5.5 open points:**

11. *No `discarded` reason?* Agree. Label `broken` as "Broken / reclaimed".
12. *Undo carry-over?* No. Confirm before the end instead. An accidental end
    only moves a boundary, and the pieces carry on.
13. *Follow-up thresholds backend-only?* Yes, and gate `maybe_covered` until
    the model is calibrated (B4). Skips mean "unknown" (C4).
14. *Manual `door_open`?* No. The RH chart shows it.
15. *Restore = one presence interval?* Yes. Restore is an undo, not "taken out
    and brought back".
16. *Expose segments/quality?* Not in the UI. Add a debug-only
    `GET /api/v1/debug/pieces/{id}/segments` JSON endpoint. You're the one
    who'll be diagnosing predictions.
17. *Visit band `[2.8, 3.6]` / k rule user-configurable?* No. Keep them as
    constants in `priors.go` and replace the k rule as in C2.
