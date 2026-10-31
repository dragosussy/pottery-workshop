# HTTP API contract (v1)

**Revision 3** (2026-10-31). It follows `00-domain-brief.md` rev. 3 and
`mid-batch-changes.md` rev. 3. The backend plan implements it and the
frontend plan consumes it. Sections marked **FUTURE** are reserved and not
part of v1.

What changed from revision 2:
- Dryness is a **continuous number** (0.0–5.0) instead of a stage key.
- The trim recommendation is deadline-driven and runs around the clock.
  The studio schedule (days and hours) and at-risk pieces were added and
  then removed on 2026-10-31.
- The `Idempotency-Key` mechanism is removed, because the app runs over plain
  HTTP (§1.2).
- An `offline` climate-log entry, `last_offline` on `/status`, and
  `GET /backup` are added.

Deviations, stated once:
- The size enum key is `xlarge`.
- "Add N identical pieces" creates N pieces sharing a `group_id` and a photo.
- Lifecycle changes are updates to a piece's `state` through PATCH (§3.5).

---

## 1. Conventions

| Topic | Rule |
|---|---|
| Transport | Plain **HTTP** over Tailscale at `http://atelier` (the Pi's Tailscale name, port 80). Same origin as the SPA, so no CORS. Unknown `/api/*` paths return the JSON 404 envelope, never `index.html`. |
| Base path | `/api/v1` |
| Content type | `application/json; charset=utf-8` both ways, with request bodies ≤ 64 KB. The exceptions are photo upload (§3.6) and the backup download (§3.11). |
| Casing | **snake_case** for field names, query params and enum values. The FE uses the fields as-is. |
| Timestamps | RFC 3339 UTC with `Z`, second precision. Requests may use any offset. Field names end in `_at`. **Exception:** chart arrays in `GET /readings` use unix seconds. |
| Dryness | A JSON number from **0.0 to 5.0**, one decimal place. The server rounds input to 0.1 and rejects values outside the range (`out_of_range`). The named stages sit at whole numbers (§3.8). Field names contain `dryness`. |
| Durations | Integer seconds, with names ending in `_seconds`. |
| Units | Part of the field name: `humidity_pct`, `temperature_c`, `vpd_kpa`. |
| IDs | Positive integers, never reused. |
| Null policy | Every documented field is always present. Optional values are `null`, arrays are `[]`, and empty text is `""`. |
| Enums | String keys. Labels and display order come from `GET /reference`. |
| Unknown input | Unknown request fields return 422 `unknown_field`. Clients ignore unknown response fields. |
| PATCH | Merge semantics: an absent field is unchanged, and `null` clears a nullable field. |
| Read-only fields | `number`, `group_id`, `current_dryness`, `flag`, counts and predictions. `state` **is** writable (§3.5). |
| Auth | None (tailnet only). |
| Clock | `/status`, `/health` and stream events carry `server_time`, and the FE keeps an offset from it. When the user picks "now", the FE **omits** the time field and the server fills it in. |
| Studio time | `settings.studio_tz` (§3.10) is used for display only: relative day names, "this morning", and reading buckets of 1 day or longer (aligned to studio-local midnight). Recommendations consider every time of day. |

### 1.1 Errors

Every non-2xx response uses one envelope:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "2 fields are invalid",
    "fields": [
      { "field": "items[1].dryness", "code": "out_of_range", "message": "must be between 0 and 5" },
      { "field": "observed_at", "code": "in_future", "message": "must not be in the future" }
    ]
  }
}
```

| HTTP | `code` | When |
|---|---|---|
| 400 | `bad_request` | Malformed JSON or bad query syntax |
| 404 | `not_found` | Unknown route or ID (soft-deleted rows count as not found) |
| 409 | `invalid_transition` | A `state` change the lifecycle doesn't allow (§3.5) |
| 409 | `piece_ended` | Recording an observation on a trimmed/discarded piece |
| 409 | `last_observation` | Deleting a piece's only observation |
| 413 | `too_large` | Photo over 3 MB |
| 415 | `unsupported_media_type` | Photo is not `image/jpeg` |
| 422 | `validation_failed` | Field-level errors in `fields` |
| 501 | `not_implemented` | FUTURE routes (control) |
| 500 | `internal` | A bug; details are logged, not returned |
| 503 | `unavailable` | Database not reachable, or too many streams |

Field codes: `required`, `invalid_enum`, `invalid_value`, `invalid_time`,
`in_future`, `before_added_at`, `after_ended_at`, `out_of_range`, `too_long`,
`duplicate`, `unknown_field`, `too_many_points`, `photo_not_found`.

User-supplied times may be backdated, and may be at most 2 minutes in the
future.

### 1.2 Retries and duplicates

There is **no idempotency-key mechanism.** It needed `crypto.randomUUID`,
which plain HTTP doesn't provide, plus a server table, and the duplicates it
prevents are rare and cheap to fix here:

- **PATCH and DELETE** are naturally idempotent. A state change to the state
  a piece already has is a no-op (§3.5), so the FE may retry these freely.
- **POST** (`/pieces`, `/observations`, `/photos`): the FE **never retries
  automatically.**
  - If the request fails with an HTTP error response, nothing was saved, and
    the FE offers "Try again".
  - If it fails with no response (a timeout or a dropped connection), the
    write may or may not have happened. The FE refetches `/shelf` and says
    "Couldn't confirm. Check the shelf before trying again."
- The rare duplicate is harmless or easy to fix:
  - A duplicate observation (same piece, dryness and time) does no harm,
    because training takes the median of observations within 30 minutes.
  - A duplicate piece shows up on the shelf straight away, and the user
    discards it as a mistake.
  - A duplicate photo is never attached, so the orphan cleanup removes it.

### 1.3 Pagination

Only `GET /pieces?state=ended|all` and `GET /observations` are paginated:
`limit` (default 20, max 100), an opaque `cursor`, newest first.

```json
{ "items": [ ... ], "next_cursor": "eyJiZWZvcmUiOjZ9" }
```

### 1.4 Status codes

`200` for reads, updates and state changes, `201` for creates, `204` for
deletes.

---

## 2. Endpoint overview

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | Liveness, version, clock sync, feature flags |
| GET | `/reference` | Dryness scale, stage landmarks, option lists, labels |
| GET | `/status` | **Home screen**: room climate, shelf summary, trim card, learning state, last offline period |
| GET | `/stream` | Server-Sent Events (§4) |
| GET | `/readings` | Downsampled climate series by time range |
| GET | `/climate-log` | Offline periods, sensor gaps, calibrations (chart markers) |
| GET | `/shelf` | Pieces on the shelf, with predictions, the visit recommendation and "check these" |
| POST | `/pieces` | Add pieces (`quantity` → group) |
| GET | `/pieces` | List pieces by state (ended ones paginated) |
| GET | `/pieces/{id}` | One piece, with its prediction |
| PATCH | `/pieces/{id}` | Change state (flip, trim, discard, undo) and/or correct fields |
| PATCH | `/pieces` | The same, for several pieces at once (`ids`), all or nothing |
| POST | `/photos` | Upload a photo, returns its `id` |
| GET | `/photos/{id}` | The image (`?size=thumb\|full`) |
| POST | `/observations` | Record dryness for one or more pieces (may return follow-ups) |
| GET | `/pieces/{id}/observations` | A piece's dryness history |
| GET | `/observations` | Recent observations (paginated) |
| PATCH | `/observations/{id}` | Correct an observation |
| DELETE | `/observations/{id}` | Delete an observation (soft) |
| POST | `/observations/{id}/undelete` | Undo a delete |
| POST | `/observations/{id}/follow-up` | Record the answer to a follow-up question |
| GET | `/settings` | Studio time zone, days and hours, sensor offsets |
| PATCH | `/settings` | Change them |
| GET | `/backup` | Download a fresh database snapshot |
| GET/PUT/POST/DELETE | `/control…` | The humidity target and the pump (§6) |

### Screen → calls

| Screen (FE plan) | Calls |
|---|---|
| Now | `GET /status` + `GET /stream`; sparkline `GET /readings?from=-6h` |
| Shelf | `GET /shelf`, `PATCH /pieces` (selection) |
| Check-in | `GET /shelf`, `POST /observations` (one piece per save), follow-ups, `PATCH /pieces/{id}` |
| Add pieces | `POST /photos` (optional, in the background), `POST /pieces` |
| Piece detail | `GET /pieces/{id}`, `PATCH /pieces/{id}`, `GET /pieces/{id}/observations`, `GET /readings` over its time range |
| Climate | `GET /readings`, `GET /climate-log` |
| History | `GET /pieces?state=ended` |
| Settings | `GET /settings`, `PATCH /settings`, `GET /health`, `GET /backup` |

---

## 3. Resources

### 3.1 Health: `GET /health`

```json
{
  "status": "ok",
  "version": "0.4.0",
  "commit": "aa8df28",
  "built_at": "2026-10-28T19:12:00Z",
  "api_version": 1,
  "server_time": "2026-10-30T14:05:03Z",
  "clock_synced": true,
  "started_at": "2026-10-30T08:00:00Z",
  "checks": { "database": "ok", "sensors": "ok" },
  "features": { "control": false }
}
```

It returns 503 when the database is down. A stale sensor gives
`status: "degraded"`, still with 200. `clock_synced: false` means the clock
hasn't synced since boot, which is likely after a power-off; the recorder
waits until it has.

### 3.2 Live status: `GET /status`

```json
{
  "server_time": "2026-10-30T14:05:03Z",
  "poll_interval_seconds": 5,
  "stale_after_seconds": 30,
  "room": { "humidity_pct": 82.4, "temperature_c": 18.6, "vpd_kpa": 0.38, "state": "ok", "read_at": "2026-10-30T14:05:01Z" },
  "sensors": [
    { "sensor": "main", "state": "ok", "humidity_pct": 82.4, "temperature_c": 18.6, "offset_pct": 1.5,
      "read_at": "2026-10-30T14:05:01Z", "age_seconds": 2, "failing_since": null, "last_error": null }
  ],
  "shelf": {
    "on_shelf": 8,
    "oldest_added_at": "2026-10-27T10:00:00Z",
    "by_stage": { "very_wet": 0, "wet": 2, "soft_leather_hard": 4, "leather_hard": 2, "hard_leather_hard": 0, "bone_dry": 0 },
    "last_check_at": "2026-10-30T08:10:00Z"
  },
  "trim": {
    "ready_now_count": 2,
    "ready_now_until": "2026-10-31T13:00:00Z",
    "check_count": 3,
    "late_count": 0,
    "next_visit": { "...": "Visit, §3.7" }
  },
  "learning": { "state": "learning", "samples": 2, "needed": 5 },
  "last_offline": { "from": "2026-10-21T19:02:00Z", "to": "2026-10-30T07:58:00Z", "pieces_to_check": 3 },
  "control": null
}
```

- **`room`**: the mean of the `ok` sensors, offsets applied. `state` is `ok`,
  `stale` or `no_data`.
- **`shelf.by_stage`**: on-shelf pieces counted by the stage **nearest** to
  their last observed dryness. All six keys are always present.
- **`trim`**: a summary of `/shelf`. `ready_now_until` is the earliest
  `ready_until` among `ready_now` pieces. `late_count` counts
  `may_be_past_check` plus `past_trim`.
- **`last_offline`**: the most recent `offline` period, if it ended within
  the last 7 days, otherwise `null`. `pieces_to_check` counts on-shelf pieces
  not observed since it ended. It drives the "Back on after 9 days" banner.
- `control` is always `null` in v1.

### 3.3 Piece resource

```json
{
  "id": 44,
  "number": 6,
  "group_id": 41,
  "group_size": 4,
  "form": "mug",
  "label": "",
  "size": "medium",
  "thickness": "medium",
  "photo_id": 12,
  "notes": "",
  "state": "flipped",
  "added_at": "2026-10-29T18:05:00Z",
  "flipped_at": "2026-10-30T09:00:00Z",
  "flip_dryness": 2.3,
  "ended_at": null,
  "end_dryness": null,
  "end_note": "",
  "mistake": false,
  "initial_dryness": 1.2,
  "current_dryness": 2.3,
  "last_observed_at": "2026-10-30T09:00:00Z",
  "observation_count": 3,
  "prediction": { "...": "PiecePrediction, below" }
}
```

- `state`: `in | flipped | trimmed | discarded`, changed with PATCH (§3.5).
- `number`: the lowest number not in use on the shelf when the piece was
  added. It is unique only among pieces on the shelf.
- `group_id`: the ID of the first piece of the same add when `quantity > 1`.
  Group members share `photo_id`.
- `initial_dryness`, `flip_dryness` and `end_dryness` mirror the observations
  written when the piece was added, flipped and ended (`source` `add` /
  `flip` / `end`). They are `null` when no value was given.
  `current_dryness` is the last observed value.
- `mistake: true` hides the piece everywhere. It stays reachable by ID, so
  Undo works.

Enums: `form`: `bowl | mug | plate | vase | other`. `size`:
`small | medium | large | xlarge`. `thickness`: `thin | medium | thick`.

**PiecePrediction** (embedded in Piece):

```json
{
  "status": "drying",
  "eta": "2026-10-31T18:00:00Z",
  "eta_low": "2026-10-31T09:00:00Z",
  "eta_high": "2026-11-01T08:00:00Z",
  "ready_until": "2026-11-01T14:00:00Z",
  "dryness_now": 2.4,
  "confidence": "learning",
  "uncertainty_reasons": ["learning", "stale_observation"],
  "forecast_basis": "recent",
  "basis": { "method": "prior", "past_pieces": 2, "summary": "Default rates for a medium mug, adjusted by 2 finished pieces" },
  "anchor": { "dryness": 2.3, "observed_at": "2026-10-30T09:00:00Z" },
  "computed_at": "2026-10-30T14:05:03Z"
}
```

| `status` | Meaning (`mid-batch-changes.md` §8.5) | `eta*` | `ready_until` |
|---|---|---|---|
| `drying` | Predicted dryness < 2.8 | set | set |
| `probably_ready_check` | Predicted ≥ 2.8, last observation below the band | set | set |
| `ready_now` | Last observation in 2.8–3.6, and predicted < 3.6 | `eta` = time of that observation | set |
| `may_be_past_check` | Predicted > 3.6, not confirmed | `null` | set (past) |
| `past_trim` | Last observation > 3.6 | `null` | `null` |
| `ended` | Trimmed or discarded | `null` | `null` |

- `eta` is when dryness reaches 3.0. `[eta_low, eta_high]` is the 80 % window.
- `ready_until` is a conservative "best before". It is `eta` plus the
  fast-rate time from 3.0 to 3.6, or, for `ready_now`, the observation time
  plus the fast-rate time from the observed value to 3.6. When `eta` is set,
  `ready_until` is never earlier than it (`mid-batch-changes.md` §7.1).
- `dryness_now` is the model's estimate. The FE shows it only as a faint
  marker on the slider. The slider is pre-filled with `current_dryness`, the
  last *observed* value.
- `confidence`: `learning | rough | decent | good`.
- `uncertainty_reasons`, dominant first: `learning`, `stale_observation`,
  `device_was_off`, `sensor_gap`, `few_checks`.
- `forecast_basis`: `recent` or `setpoint` (FUTURE).
- `basis.method`: `prior` or `learned`.

| Method | Path | Request | Response | Errors |
|---|---|---|---|---|
| POST | `/pieces` | AddPieces | 201 `{ "items": [Piece] }` | 422 |
| GET | `/pieces?state=on_shelf\|ended\|all&limit&cursor` | — | `on_shelf`: `{ "items" }` ordered by number. `ended`/`all`: Page<Piece>, newest `ended_at` first | — |
| GET | `/pieces/{id}` | — | 200 Piece | 404 |
| PATCH | `/pieces/{id}` | PiecePatch | 200 Piece | 404, 409 `invalid_transition`, 422 |
| PATCH | `/pieces` | PiecePatch + `"ids": [int]` (1–50) | 200 `{ "items": [Piece] }` | 404, 409, 422 |

**AddPieces**

```json
{
  "quantity": 4,
  "form": "mug",
  "initial_dryness": 1.2,
  "size": "medium",
  "thickness": "medium",
  "photo_id": 12,
  "label": "",
  "notes": "",
  "added_at": "2026-10-29T18:05:00Z"
}
```

- Required: `form`, `initial_dryness`, `size`, `thickness`. `quantity` is 1–20
  (default 1). `added_at` defaults to now and may be backdated.
- All N pieces are created in one transaction, each taking the lowest free
  shelf number in turn.
- `label` ≤ 80 chars, `notes` ≤ 2000.

**PiecePatch**: `{ state?, flipped_at?, flip_dryness?, ended_at?, end_dryness?,
end_note?, mistake?, form?, label?, size?, thickness?, notes?, photo_id?,
added_at?, initial_dryness? }`.
- **With `state`**, the request is a lifecycle change (§3.5).
- **Without `state`**, it is a correction:
  - `initial_dryness` and `added_at` rewrite the add observation.
  - `flipped_at`/`flip_dryness` and `ended_at`/`end_dryness` correct the flip
    or the end, and the observation it wrote. They only apply in that state.
  - `photo_id: null` detaches the photo from this piece only.
- On `PATCH /pieces`, every listed piece gets the same body, in one
  transaction. Errors carry per-item paths (`ids[2]`).

### 3.4 Readings and the climate log

**`GET /readings`**

| Param | Default | Meaning |
|---|---|---|
| `from` | required | RFC 3339, or relative: `-6h`, `-24h`, `-7d` |
| `to` | now | RFC 3339 |
| `sensors` | `room` | `room` (the calibrated mean) or a comma-separated list of sensor names |
| `resolution` | `auto` | `auto`, `1m`, `5m`, `15m`, `1h`, `6h`, `1d`. `auto` picks the finest step that gives ≤ 360 buckets. |

```json
{
  "from": "2026-10-29T14:00:00Z",
  "to": "2026-10-30T14:00:00Z",
  "resolution_seconds": 300,
  "t": [1793282400, 1793282700, 1793283000],
  "series": [
    { "sensor": "room", "humidity_avg": [82.1, 82.3, null], "humidity_min": [81.6, 81.9, null],
      "humidity_max": [82.7, 82.8, null], "temperature_avg": [18.6, 18.7, null],
      "vpd_avg": [0.38, 0.38, null], "samples": [60, 60, 0] }
  ]
}
```

- Every bucket in the range is present; buckets with no data are `null`, so
  charts show gaps, including weeks when the device was off.
- An explicit `resolution` that would give more than 2000 buckets fails with
  `too_many_points`.

**`GET /climate-log?from&to`** returns `{ "items": [ClimateLogEntry] }`,
oldest first:

```json
{ "id": 9, "kind": "offline", "value": { "from": "2026-10-21T19:02:00Z", "to": "2026-10-30T07:58:00Z" }, "at": "2026-10-30T07:58:00Z" }
```

| `kind` | `value` | Written by |
|---|---|---|
| `offline` | `{ "from", "to" }` | The recorder at startup, when the last stored minute is more than 1 h ago |
| `sensor_gap` | `{ "sensor", "from", "to" }` | The recorder, when a sensor fails for more than 1 h while running |
| `sensor_calibrated` | `{ "sensor", "offset_pct", "previous_offset_pct" }` | `PATCH /settings` |
| `sensor_replaced` | `{ "sensor" }` | `PATCH /settings` |
| `setpoint_set` | `{ "target_humidity_pct", "previous_pct" }` | The humidity target changed (`null`: cleared) |
| `pump_manual` | `{ "seconds" }` | A manual pump run |
| `pump_tripped` | `{ "reason", "pulses" }` | The pump stopped for safety |
| `pump_reset` | `{ "reason" }` | Resumed after a trip |

### 3.5 Piece state changes (lifecycle)

Flipping, trimming and discarding are updates to `state`, sent as
`PATCH /pieces/{id}`, or `PATCH /pieces` with `ids`. Undo is the reverse
transition.

```json
PATCH /api/v1/pieces
{ "ids": [41, 42, 43], "state": "trimmed", "end_dryness": 3.0 }
```

| From → to | Meaning | Fields | Side effects |
|---|---|---|---|
| `in` → `flipped` | Flip | `flipped_at` (default now), `flip_dryness`? | Writes a `flip` observation if `flip_dryness` is given |
| `in`/`flipped` → `trimmed` | Trim | `ended_at` (default now), `end_dryness` (default 3.0), `end_note`? | Always writes an `end` observation |
| `in`/`flipped` → `discarded` | Discard | `ended_at` (default now), `end_dryness`?, `end_note`?, `mistake`? | Writes an `end` observation if `end_dryness` is given |
| `flipped` → `in` | **Undo** a flip | — | Clears the flip, soft-deletes its observation |
| `trimmed`/`discarded` → the state before the end | **Undo** an end ("Back on the shelf") | — | Clears the end, soft-deletes its observation. If its shelf number has been reused, it gets the lowest free one. |
| `trimmed` ↔ `discarded` | Correct the kind of end | `end_dryness`?, `end_note`?, `mistake`? | Keeps `ended_at` |
| Same state as now | **No-op** | — or correction fields | 200. Any correction fields are applied as a correction. Makes retries safe. |

Any other transition returns 409 `invalid_transition`. One PATCH makes at
most one transition.

- **Time rules:** times are backdatable, at most 2 minutes in the future,
  ≥ `added_at`, an end ≥ `flipped_at`, and ≥ the last observation. Fields that
  don't belong to the transition give 422 `invalid_value`.
- `flip_dryness` is pre-filled by the FE with the last observed value. In a
  group PATCH it applies to every listed piece.
- **Trim** covers "covering it to trim later": the potter sets
  `end_dryness` to how dry the piece is now.

### 3.6 Photos

| Method | Path | Request | Response | Errors |
|---|---|---|---|---|
| POST | `/photos` | Body: raw `image/jpeg`, ≤ 3 MB | 201 `{ "id", "width", "height", "created_at" }` | 413, 415, 422 |
| GET | `/photos/{id}?size=thumb\|full` | — | The image bytes (`thumb` is 320px on the long edge). `Cache-Control: private, max-age=31536000, immutable` | 404 |

- The FE resizes before upload (1600px, JPEG 0.8), drawing through
  `createImageBitmap(file, { imageOrientation: "from-image" })` and a canvas.
  That bakes in the rotation and drops EXIF, including GPS. Both work over
  plain HTTP.
- The server re-encodes the image and generates the thumbnail.
- A photo that no piece refers to is deleted after 24 h. Photos live on the
  data volume.

### 3.7 Shelf: `GET /shelf`

```json
{
  "computed_at": "2026-10-30T14:05:03Z",
  "items": [ "Piece (with prediction) …, ordered by number" ],
  "next_visit": {
    "recommended_at": "2026-10-31T09:00:00Z",
    "start": "2026-10-31T09:00:00Z",
    "end": "2026-10-31T11:30:00Z",
    "deadline": "2026-10-31T13:00:00Z",
    "ready_piece_ids": [41, 42, 43, 45],
    "maybe_piece_ids": [46, 47],
    "not_yet_piece_ids": [44],
    "score": 5.0,
    "confidence": "learning"
  },
  "check_these": [
    { "piece_id": 50, "reasons": ["probably_ready"] },
    { "piece_id": 51, "reasons": ["not_checked_since_restart"] }
  ]
}
```

The visit is computed as in `mid-batch-changes.md` §7.2:

- **Slots:** now, then every :00 and :30 (in `studio_tz`) **around the
  clock** over the next 7 days. There is no studio schedule.
- **`deadline`:** the earliest `ready_until` that is still in the future.
  Pieces whose `ready_until` has passed are not planned.
- **Score per slot** (only slots ≤ `deadline`): 1 for each piece with
  `eta ≤ t ≤ ready_until`, and 0.5 for each with `eta_low ≤ t < eta`.
  `ready_now` pieces count with `eta` = their observation time.
- `recommended_at` is the highest-scoring slot, earliest on a tie.
  `[start, end]` is the contiguous run of slots with the same score.
- At `recommended_at`, the pieces are split into ready, maybe and not-yet
  lists.
- `next_visit` is `null` when no slot scores above 0.

**`check_these`** reasons:
- `probably_ready`
- `may_be_past`
- `not_checked_today` (learning period only, for pieces not observed in 24 h)
- `not_checked_since_restart` (not observed since the last `offline` ended)
- `stale_observation` (after the learning period, when the window has
  widened a lot since the last check)

### 3.8 Reference data: `GET /reference`

```json
{
  "dryness": { "min": 0.0, "max": 5.0, "step": 0.1, "target": 3.0, "trim_band": [2.8, 3.6] },
  "stages": [
    { "key": "very_wet",          "label": "Very wet",          "value": 0 },
    { "key": "wet",               "label": "Wet",               "value": 1 },
    { "key": "soft_leather_hard", "label": "Soft leather hard", "value": 2 },
    { "key": "leather_hard",      "label": "Leather hard",      "value": 3 },
    { "key": "hard_leather_hard", "label": "Hard leather hard", "value": 4 },
    { "key": "bone_dry",          "label": "Bone dry",          "value": 5 }
  ],
  "forms":       [ { "key": "bowl", "label": "Bowl" }, { "key": "mug", "label": "Mug" }, { "key": "plate", "label": "Plate" }, { "key": "vase", "label": "Vase" }, { "key": "other", "label": "Other" } ],
  "sizes":       [ { "key": "small", "label": "S" }, { "key": "medium", "label": "M" }, { "key": "large", "label": "L" }, { "key": "xlarge", "label": "XL" } ],
  "thicknesses": [ { "key": "thin", "label": "Thin" }, { "key": "medium", "label": "Medium" }, { "key": "thick", "label": "Thick" } ],
  "piece_states": [ { "key": "in", "label": "On shelf" }, { "key": "flipped", "label": "Flipped" }, { "key": "trimmed", "label": "Trimmed" }, { "key": "discarded", "label": "Discarded" } ],
  "prediction_statuses": [ { "key": "drying", "label": "Drying" }, "…" ],
  "uncertainty_reasons": [ { "key": "device_was_off", "label": "the controller was off" }, "…" ],
  "check_reasons": [ { "key": "not_checked_since_restart", "label": "Not checked since the controller came back on" }, "…" ]
}
```

The FE builds slider labels from `stages` (the nearest landmark plus a
qualifier) and draws the trim band from `dryness.trim_band`.

### 3.9 Observations

```json
{
  "id": 310,
  "piece_id": 44,
  "dryness": 2.3,
  "observed_at": "2026-10-30T08:10:00Z",
  "source": "user",
  "flag": null,
  "note": "",
  "created_at": "2026-10-30T08:10:04Z"
}
```

- `source`: `add | user | flip | end`. Only `user` observations come from
  `POST /observations`.
- `flag`: `null` or `inconsistent` (more than 0.5 wetter than the previous
  observation). It is computed and read-only. An inconsistent observation
  still anchors the live ETA but is excluded from training.

| Method | Path | Request | Response | Errors |
|---|---|---|---|---|
| POST | `/observations` | NewObservations | 201 `{ "items": [Observation], "follow_ups": [FollowUp] }` | 409 `piece_ended`, 422 |
| GET | `/pieces/{id}/observations` | — | 200 `{ "items" }`, oldest first | 404 |
| GET | `/observations?limit&cursor` | — | 200 Page<Observation> | — |
| PATCH | `/observations/{id}` | `{ "dryness"?, "observed_at"?, "note"? }` | 200 Observation | 404, 422 |
| DELETE | `/observations/{id}` | — | 204 (soft) | 404, 409 `last_observation` |
| POST | `/observations/{id}/undelete` | `{}` | 200 Observation | 404 |
| POST | `/observations/{id}/follow-up` | `{ "answer" }` | 200 `{}` | 404, 422 |

**NewObservations**

```json
{ "observed_at": "2026-10-30T14:00:00Z", "items": [ { "piece_id": 44, "dryness": 2.6 } ], "note": "" }
```

- `items`: 1–50 entries with unique on-shelf piece IDs. It's all or
  nothing. The check-in screen sends **one item per save** (FE plan §4.3).
  Selections send several.
- `observed_at` defaults to now, must be ≥ `added_at`, and must not be in the
  future.

**FollowUp**: at most one per observation, never asked twice for a piece. The
server records that it asked when it returns the question, so skipping needs
no call.

```json
{
  "kind": "went_wetter",
  "piece_id": 44,
  "observation_id": 311,
  "previous_observation_id": 310,
  "question": "Wetter than last time. What happened?",
  "options": [
    { "key": "last_entry_wrong", "label": "Last entry was wrong", "action": "correct_observation" },
    { "key": "its_fine",         "label": "It's fine",            "action": "none" }
  ]
}
```

| `kind` | Trigger | Options |
|---|---|---|
| `much_slower` | ≥ 1.5 below the prediction. Never sent during the learning period. | `still_drying` (none), `covered_or_removed` (`end_piece`: the FE opens the trim/discard sheet) |
| `went_wetter` | ≥ 1.5 wetter than the previous observation | `last_entry_wrong` (`correct_observation`), `its_fine` (none) |

A skipped question means *unknown*, and the pair is excluded from training.

### 3.10 Settings

```json
{
  "studio_tz": "Europe/Bucharest",
  "sensors": [ { "sensor": "main", "offset_pct": 1.5, "calibrated_at": "2026-10-30T12:00:00Z" } ]
}
```

| Method | Path | Request | Response | Errors |
|---|---|---|---|---|
| GET | `/settings` | — | 200 Settings | — |
| PATCH | `/settings` | `{ "studio_tz"?, "sensors"?: [{ "sensor", "offset_pct", "replaced"? }] }` | 200 Settings | 422 |

- `studio_tz` is an IANA name. Until it is set, it defaults to `STUDIO_TZ`
  or `UTC`. The FE proposes the phone's time zone on first setup.
- `studio_tz` is for display only (§1, "Studio time").
- `offset_pct` is −10 to +10, applied to raw RH from now on, and logged in the
  climate log. Sensor names are permanent keys in the history.

### 3.11 Backup: `GET /backup`

Returns `application/vnd.sqlite3`, with
`Content-Disposition: attachment; filename="humidity-20261030T140503Z.db"`.
The body is a fresh, consistent snapshot of the database (`VACUUM INTO` a
temp file, streamed, then deleted). Photos are not included. It's a plain
link in Settings ("Download backup"), so a copy can live off the SD card.

---

## 4. Live updates: Server-Sent Events

SSE works over plain HTTP. `net/http` with a flusher implements it, and
`EventSource` reconnects by itself.

### 4.1 Endpoint

`GET /api/v1/stream` returns `text/event-stream`, with
`Cache-Control: no-store` and `X-Accel-Buffering: no`. The server sends
`retry: 5000` first.

### 4.2 Events

| `event:` | When | `data:` |
|---|---|---|
| `hello` | Once, on connect | `{ "server_time", "version", "api_version", "poll_interval_seconds", "stale_after_seconds", "clock_synced" }` |
| `readings` | On connect, then after every poll cycle | `{ "server_time", "room", "sensors": [SensorStatus] }` |
| `changed` | After any successful write or system log entry, debounced 500 ms | `{ "scopes": [...], "piece_ids": [...] }` |
| (comment) | Every 20 s | `: ping` |

`scopes` is a subset of `pieces, observations, predictions, settings,
climate_log`.

- `changed` is a signal to refetch, not data.
- There is no replay. On (re)connect, the FE refetches `/status`.
- If `hello.version` differs from the FE's build version (a tab left open
  across a deploy), the FE offers a reload.
- Server: a 16-message buffer per client. Stale `readings` messages are
  dropped first; if a `changed` message would be dropped, the client is
  disconnected instead. At most 16 streams.

### 4.3 Client behaviour

- The stream is open while the app is visible, and closed while it's hidden.
  When the app becomes visible again, the FE reconnects and refetches
  `/status`.
- The FE refetches `/status` every 5 min while visible, because predictions
  move with time.
- **Fallback:** after 3 failures within 60 s, poll `/status` every 10 s and
  retry the stream every 60 s.
- **Unreachable** (usually because the device is switched off): the FE shows
  the last `/status` it cached (in `localStorage`), greyed out with "as of
  …", and a banner reading "The drying controller is offline (probably
  switched off)".

---

## 5. Corrections vs. changes

| The user wants to… | Call |
|---|---|
| Record dryness, now or earlier | `POST /observations` |
| Fix a wrong slider value or time | `PATCH /observations/{id}` |
| Flip / trim / discard | `PATCH /pieces/{id}` or `PATCH /pieces` with `state` |
| Undo that tap | The same PATCH with the previous `state` |
| Fix the time or value of a flip or trim | `PATCH /pieces/{id}` without `state` |
| Fix the form, size, thickness or photo | `PATCH /pieces/{id}` |
| "This piece never existed" | `PATCH` with `state: "discarded"`, `mistake: true` |

Every write triggers recomputation and a `changed` event.

---

## 6. Humidity target (v1) and actuator control (FUTURE)

The **humidity target** is part of v1 (owner decision 2026-10-31), and so is
the **pump** that holds it (backend plan §10.1). Without a pump
(`PUMP_DRIVER=none`: `controller: false`, `pump`, `safety` and `limits`
null) the potter keeps the shelf near the target by hand. While a target is set, predictions assume
the shelf sits at it from now on: the forecast rate is the VPD at the target
and the mean temperature of the last 24 h (`forecast_basis: "setpoint"`). The
measured dose up to now is unchanged. Without a target, the forecast follows
the recent climate (`"recent"`). `status.control` carries the same body as
`GET /control`:

```json
{
  "target_humidity_pct": 82.0,
  "set_at": "2026-10-31T17:40:00Z",
  "min_pct": 50,
  "max_pct": 98,
  "controller": true,
  "pump": {
    "name": "pump", "on": false, "since": "2026-10-31T17:52:10Z",
    "reason": "resting", "rest_until": "2026-10-31T17:55:10Z",
    "runtime_24h_seconds": 140, "last_run_at": "2026-10-31T17:52:00Z", "error": null
  },
  "safety": { "tripped": false, "reason": null, "since": null },
  "limits": {
    "pulse_seconds": 10, "rest_seconds": 180, "hysteresis_pct": 2,
    "max_rh_pct": 95, "max_manual_seconds": 60
  }
}
```

`pump.reason`: `below_target` (running), `manual` (running by hand),
`resting`, `at_target`, `no_target`, `no_sensor_data`, `above_max`,
`tripped`, `switch_error`. `safety.reason`: `not_rising`.
Pump state changes emit `changed` with scope `control`.

| Method | Path | Purpose |
|---|---|---|
| GET | `/control` | The target (`null` when none) |
| PUT | `/control/setpoint` | `{ "target_humidity_pct": 82 }` sets the target (50–98, rounded to 0.1), `null` clears it. Logs `setpoint_set` `{ target_humidity_pct, previous_pct }` when it changes, and emits `changed` with `control`, `predictions` and `climate_log`. |
| GET | `/control/preview?target_humidity_pct=86` | Saves nothing. Plans the shelf as if the target were 86 (absent: no target) and returns `{ target_humidity_pct, forecast_temp_c, next_visit, ready_now_count }`. `next_visit` has the `/shelf` shape. |
| POST | `/control/override` | `{ "seconds": 5 }` runs the pump by hand now (1 to `max_manual_seconds`), whatever the target, humidity or trip. Logs `pump_manual`. |
| DELETE | `/control/override` | Stops the pump now; the controller rests before deciding again |
| POST | `/control/reset` | Clears a safety trip (logs `pump_reset`) |

Without a pump, the three pump routes answer **409** `no_pump`.
`health.features.control` is `true` when a pump is configured.

---

## 7. Versioning

The SPA and the API ship in one binary. Within `/api/v1`, changes are
additive only. Revision 3 replaces revisions 1–2 before anything was
implemented.

## 8. TypeScript types

Hand-written `web/src/api/types.ts`, about 30 types:

| Type | Section |
|---|---|
| `FormKey`, `SizeKey`, `ThicknessKey`, `StageKey`, `PieceState`, `ObservationSource`, `ObservationFlag`, `Confidence`, `PredictionStatus`, `UncertaintyReason`, `CheckReason`, `SensorState`, `ClimateLogKind` | various |
| `Dryness` (`number`, 0–5) | §1 |
| `ApiError`, `FieldError`, `Page<T>` | §1 |
| `Health` | §3.1 |
| `Room`, `SensorStatus`, `LiveStatus`, `ShelfSummary`, `TrimSummary`, `Learning`, `OfflinePeriod` | §3.2 |
| `Piece`, `PiecePrediction`, `PredictionBasis`, `AddPieces`, `PiecePatch`, `PiecesPatch` | §3.3, §3.5 |
| `ReadingsSeries`, `SensorSeries`, `ClimateLogEntry` | §3.4 |
| `PhotoCreated` | §3.6 |
| `Shelf`, `Visit`, `CheckItem` | §3.7 |
| `Reference`, `DrynessScale`, `StageLandmark`, `Option` | §3.8 |
| `Observation`, `NewObservations`, `ObservationPatch`, `ObservationsCreated`, `FollowUp`, `FollowUpOption` | §3.9 |
| `Settings`, `SettingsPatch` | §3.10 |
| `StreamHello`, `StreamReadings`, `StreamChanged` | §4 |
| `ControlState`, `Actuator` (declared, not used) | §6 |
