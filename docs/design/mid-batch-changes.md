# Changes while pieces dry

**Revision 3** (2026-10-31).

- **Revision 2** (2026-10-30) dropped batches, covers, the fan, sensor zones and
  clay bodies.
- **Revision 3** applies the owner's decisions of 2026-10-31:
  - dryness is a **continuous** value set with a precise slider, not 6
    fixed steps
  - the trim recommendation must respect the pieces' **deadlines**. A studio
    schedule (days and hours) was added and then **removed** the same day,
    because the potter can go whenever they want
  - throwing happens in **sessions**, and the device is **off** between them
- The review fixes still relevant are folded in: B1, B2, B6, B7, C1, C2, C4,
  X2, X3 and X7. B3 (anchoring bias) is no longer needed, because values are
  continuous.

---

## TL;DR

1. **No batches.** The app tracks **pieces**, each with its own clock, on one
   shelf, plus **one room climate** recorded every minute while the device is
   on.
2. **Four piece states:** `in` → `flipped` → `trimmed` | `discarded`. Covering
   a piece means ending it in the app, at its current dryness.
3. **Dryness is a continuous 0.0–5.0 value** from a precise slider. The
   potter's gut feeling is the measurement. Continuous values remove the
   biggest modelling headache (coarse 6-step observations) and make every
   observation directly usable as the model's progress value.
4. **Humidity changes are captured by recording, not by asking.** Each piece's
   prediction uses the climate it actually got (its **drying dose**) and a
   forecast for the rest of the time.
5. **The trim card respects deadlines.**
   - Pieces that are ready now count, until their "best before" time.
   - It aims for the slot that catches the most pieces **before the first
     one goes past**.
   - Any time of day is a candidate, because there is no studio schedule.
     The potter steers the drying (humidity target, plastic) when a day
     doesn't suit.
6. **Sessions and power-offs are normal.**
   - A gap while the shelf is empty costs nothing.
   - A gap with pieces on the shelf is filled with an estimate, flagged, and
     followed by a "check these" prompt when the device comes back.
   - Learning carries over between sessions.
7. **Predictions adapt from the start**, using studio, recent, sibling and
   piece factors, after a visible 5-piece learning period.

---

## 1. Owner decisions

| Date | Decision | Consequence |
|---|---|---|
| 10-30 | One clay body | No clay field, and not a model factor |
| 10-30 | Piece states: freshly in, flipped, discarded, trimmed | Small lifecycle (§4) |
| 10-30 | One shelf, enclosed in plastic | One climate. No zones, no "move" action. |
| 10-30 | No fan | No fan state |
| 10-30 | Covered pieces are not tracked | Covering = trim (at current dryness) or discard |
| 10-30 | No batches; humidity is global | Pieces are added individually; climate is room-level |
| 10-30 | Predictions must know how humidity changed while pieces dried | Continuous climate history, per-piece drying dose (§5) |
| 10-31 | **Sliders are a must, and must be precise**; gut feeling is the measurement | Continuous dryness 0.0–5.0 (§2), slider UX (FE plan §5) |
| 10-31 | **The trim card must respect the pieces' deadlines** | Deadline-aware visit planning around the clock (§7) |
| 10-31 | **No studio schedule.** The potter goes whenever; if a day doesn't suit, they raise the humidity target or wrap the piece | No days or hours in Settings, no at-risk warning (§7) |
| 10-31 | **Throwing in sessions; the device is off in between** | Offline handling (§6) |

---

## 2. Dryness: a continuous scale

- The slider records **dryness** from 0.0 (very wet) to 5.0 (bone dry) in steps
  of 0.1. The six stage names sit at the whole numbers and serve as
  landmarks.
- **Target 3.0** (leather hard). **Trim band 2.8–3.6**: from "leather hard, a
  touch soft" to "leather hard, getting firm". The band is a constant in v1.
- **Display:** the nearest stage name plus a qualifier for the distance from
  it ("Leather hard · a touch soft" for 2.8, "Soft leather hard · firming
  up" for 2.4). The number itself isn't shown to the potter by default.
- **Why continuous is better for the model:**
  - Observations no longer round to a whole stage, so the error that
    rounding adds disappears.
  - Each observation **is** the model's progress value, with no "where in the
    band" guess.
  - The time the trim target is reached can be **interpolated** between two
    observations instead of only being bracketed.
- **Noise:** gut feeling varies by about ±0.3 between judgments. The model
  carries this as observation noise (`σ_obs = 0.3`). "Judge the foot" stays
  as the hint under the slider.

---

## 3. What the setup means physically

- **The sensor sees what the pieces see.** Inside the plastic around one
  shelf, the air is roughly uniform.
- **The pieces humidify the tent.** A session's worth of fresh pieces raises
  RH and slows everything. The sensor records this, so drying dose accounts
  for it.
- **Opening the tent** causes a brief RH dip. It is recorded and included
  automatically.
- **The tent runs damp (70–90 % RH)**, where small RH errors make large
  drying-speed errors. Hence the SHT31/SHT4x sensors and the calibration
  offset (§5.3).
- **Drying physics:**
  - The rate is near-constant down to leather hard, then slows. The trim
    target sits at the end of the constant-rate phase, so a linear model in
    dose fits the part we care about.
  - Thickness dominates; size matters less.
  - The foot dries last.

---

## 4. Piece lifecycle

```
            flip              trim
  [in] ──────────────▶ [flipped] ──────▶ [trimmed]
   │                       │
   │        trim           │   discard
   ├───────────────────────┼───────────▶ [discarded]
   └──────── discard ──────┘
```

| State | Meaning | Recorded |
|---|---|---|
| `in` | On the shelf, not yet flipped | `added_at` (backdatable). The initial dryness becomes the first observation. |
| `flipped` | Turned over so the foot dries | `flipped_at`, plus an optional dryness reading (pre-filled with the *last observed* value) |
| `trimmed` | Left the shelf to be trimmed, now or later after covering it | `ended_at` and **`end_dryness`** (pre-filled at 3.0, adjustable) |
| `discarded` | Left for any other reason (cracked, reclaimed, given up, entered by mistake) | `ended_at`, optional dryness, an optional note, and a "mistake" flag |

- **Covering = ending.** The trim sheet says: *"Covering it to trim later?
  Set how dry it is now."* The `end_dryness` keeps training data honest. A
  piece covered at 2.4 teaches "reached 2.4 by now", not "reached 3.0".
- **Undo** is the reverse state change (contract §3.5). A piece that was
  covered and later goes back on the shelf is added again as a new piece.
- A piece entered **by mistake** is discarded with the mistake flag and
  excluded everywhere.
- There is **no event log**. Every change is one of the timestamps above.

---

## 5. Humidity changes while pieces dry

### 5.1 Sources and how they are captured

| Source | How the system knows | User input |
|---|---|---|
| Setpoint changed (future controller) | Logged by the backend | None |
| Controller can't hold the setpoint | Readings diverge from the setpoint | None. The UI shows a warning. |
| Pieces added (wet clay humidifies the tent) | Readings | None |
| Tent opened | Readings | None |
| Weather and seasons | Readings | None |
| Sensor fault while running | Missing readings (`sensor_gap`) | None |
| **Device switched off** | Missing minutes, detected at startup (`offline`) | None (§6) |
| Sensor drift or replacement | Calibration | A salt test now and then |

### 5.2 Recording

- One row per minute while the device is on: mean RH, mean T and mean VPD.
  VPD is computed per reading, not from the averages.
- With several sensors, the room climate is their mean. One sensor is
  enough.
- The **climate log** is written by the system only: `offline {from, to}`,
  `sensor_gap {sensor, from, to}`, `sensor_calibrated`, `sensor_replaced`
  and, later, `setpoint_set`.
- Readings are never tied to pieces. A piece's climate is "readings while it
  was on the shelf".

### 5.3 Making the record trustworthy

- **Sensor accuracy:**
  - SHT31 or SHT4x, about ±1.5–2 % RH.
  - A per-sensor RH offset from a salt test (damp table salt in a sealed box
    sits at about 75 % RH).
  - SHT4x can creep upward after long periods above 80 % RH. If the salt test
    shows drift, a heater pulse can be added later.
- **Placement:** inside the plastic, at ware height.
- **Clock (review X2):** the device is often powered on after days off.
  - Readings are held until the system clock has synced.
  - Minutes are never written backwards.
  - An RTC battery for the Pi 5 is recommended.
- **Gaps:**

  | Gap | Shelf empty? | Handling |
  |---|---|---|
  | < 1 h | any | Carry the last VPD forward |
  | ≥ 1 h | empty | Nothing to do. The chart shows the gap. |
  | ≥ 1 h | pieces on the shelf | Fill with the mean VPD of the 24 h before the gap (not the floor, which would freeze drying, review B2). Uncertainty reason `device_was_off` or `sensor_gap`. Excluded from training. |

### 5.4 Drying dose

- `VPD = 0.6108 · exp(17.27·T / (T + 237.3)) · (1 − RH/100)` kPa, with a
  floor of 0.05 kPa.
- **Effective hours** = Σ VPD·Δt / VPD_ref, where VPD_ref is 20 °C / 65 % RH
  ≈ 0.82 kPa. In the damp tent, one wall-clock hour may be only 0.3–0.5
  effective hours.
- A piece's progress is driven by the dose it actually received. A setpoint
  change, a hot day or a session's worth of wet clay all show up
  automatically.
- **Forecast dose:**
  - With a controller holding a setpoint: VPD at the setpoint.
  - Otherwise: the last 6 h for the next 6 h, then the last 24 h
    (review B6).
  - The basis is shown ("assuming the shelf stays like the last day").

---

## 6. Sessions and power-offs

The usual cycle:

1. The device is on.
2. A throwing session puts many pieces on the shelf.
3. Over the next days they're flipped and trimmed.
4. The shelf empties.
5. The device is switched off for days or weeks.

What follows from this:

- **Empty-shelf gaps are free.** Nothing on the shelf means nothing to
  predict. When the device comes back on, the history continues.
- **Pieces left listed on the shelf across a power-off.** This happens when
  someone forgot to mark them, or switched off early.
  - On startup the backend logs `offline {from, to}`.
  - Every on-shelf piece whose last observation is older than the restart
    goes into **"check these"** with reason `not_checked_since_restart`.
  - The Now screen shows a banner: "Back on after 9 days · 3 pieces still
    listed on the shelf". It offers to check them, or to mark them
    trimmed or discarded.
  - Predictions for these pieces carry `device_was_off`, with a wide window,
    until they are observed again.
- **Learning carries over between sessions.** Everything learned persists.
  The "recent" factor (§8.3) uses the **last 10 pieces that reached the
  target**, not the last *n* days, so a 3-week break doesn't erase it.
- **A session is a natural learning burst.** One session of 10–20 pieces can
  complete the 5-sample learning period on its own, if the potter checks
  pieces at least once a day while they're near leather hard.
- **Backups:** a backup is taken at startup (before any migration) and daily
  while running. A device that's only on for a few days still gets backed up.
- **Clean shutdown:** the Pi 5's power button triggers a proper shutdown. The
  backend flushes the open minute on SIGTERM.

---

## 7. "When should I come?" (the trim card)

This is the most important output. It must never call a stale "ready"
ready, and must never leave out the most urgent pieces (reviews C1, C2).

**No studio schedule** (owner decision 2026-10-31). The potter can go
whenever they want, so every time of day counts. When a recommended day
doesn't suit, the potter adjusts the drying instead:
- raise the target humidity to slow it (once the controller exists; the
  setpoint advisor in §12 is the natural follow-up), or
- wrap a ready piece tightly in plastic, which in the app is "trim at the
  current dryness" (§4).

### 7.1 Inputs

For each piece on the shelf (`in` or `flipped`, not past the band), from §8:
- `eta`: the point estimate of reaching 3.0.
- `eta_low`: the early end of its 80 % window.
- `ready_until`: a conservative "best before", **anchored on `eta`**:
  - `eta` plus the time to go from 3.0 to 3.6 at the **fast** rate (the fast
    end of the 80 % rate range).
  - For a piece observed inside the band: the observation time plus the
    fast-rate time from its observed value to 3.6.
  - It is never earlier than `eta`.
  - (An earlier definition used the fast-side time all the way from the
    anchor to 3.6. With the wide learning-period windows, that put "best
    before" *before* `eta`, so the recommendation came too early. Fixed
    2026-10-31.)
- A piece that is *ready now* (last observation in 2.8–3.6, and still
  predicted under 3.6) has `eta` = its observation time.

The studio time zone (Settings) is used only to *display* times ("today",
"Sat 09:00") and for "this morning" in the time chips.

### 7.2 Algorithm

1. **Slots:** now, then every 30 minutes around the clock over the next
   7 days.
2. **Deadline `D`:** the earliest `ready_until` that is still in the future.
   Any visit after `D` loses a piece.
3. **Score** of each slot `t ≤ D`:

   | Piece's position at `t` | Points |
   |---|---|
   | `eta ≤ t ≤ ready_until` (ready) | 1 |
   | `eta_low ≤ t < eta` (maybe ready) | 0.5 |
   | otherwise | 0 |

4. **Recommend** the highest-scoring slot at or before `D`, taking the
   earliest on a tie. The window `[start, end]` is the contiguous run of
   slots around it with the same score.
5. The result lists the pieces **ready**, **maybe ready** and **not yet**
   ready at that time. The not-yet pieces get their own recommendation once
   these are trimmed.

### 7.3 What the card says

| Situation | Card |
|---|---|
| Pieces ready now | "**3 ready now** · best before Sat 14:00" |
| Normal | "Come **Sat 09:00–11:30** · 5 ready, 2 maybe" |
| Nothing pending | "Nothing to trim soon" (the earliest `eta_low` is more than 7 days away), or an empty-shelf state |
| Learning | Any of the above, plus "rough guess · learning your shelf (2 of 5)" |
| Device unreachable | The last known card, greyed out, "as of Thu 18:05" |

---

## 8. Prediction

All arithmetic, in Go, with no libraries.

### 8.1 Prior

Effective hours per unit of dryness (one stage), medium size, at reference
conditions:

| Thickness | Eff-h per unit |
|---|---|
| thin | 3 |
| medium | 6 |
| thick | 12 |

Size multiplier on time: small ×0.75 · medium ×1 · large ×1.4 · xlarge ×1.9.

These are estimates. Owner decision: no reference times will be provided,
and the app learns instead. The damp tent is already accounted for by the
dose, so the table only needs to be right in relative terms. `a_studio`
corrects the overall scale.

**Learning period:** until **5 pieces** have produced a training sample:
- Confidence is `learning`.
- Windows are wider (σ_class = 0.7 instead of 0.4).
- "Check these" includes every piece not observed in the last 24 h.
- The `much_slower` follow-up stays off.

**Flip:** not a factor in v1. Nearly every piece is flipped at a similar
moment, so its effect is already inside the base rate. `flipped_at` is kept
so a factor can be learned later.

### 8.2 Live ETA

1. **Anchor** at the latest observation: dryness `d₀` at `t₀`, with noise
   `σ_obs`. A piece only just added anchors at its initial dryness.
2. **Walk forward** from `t₀`: `d(t) = d₀ + rate × eff_hours(t₀, t)`, using
   measured dose up to now and forecast dose afterwards. Gaps are filled as
   in §5.3.
3. `eta` is when `d` reaches 3.0. `ready_until` = `eta` + the fast-rate time
   from 3.0 to 3.6, or, for a piece observed in the band, the observation time
   + the fast-rate time from its value to 3.6 (§7.1).
4. **Window:** 80 % range from the rate uncertainty (`σ_rate`, log scale)
   combined with `σ_obs` converted to time at the current rate. The window
   widens with time since the anchor, and with gap-filled dose.
5. **Slower than expected:** if a new observation sits below the predicted
   value, the piece factor `e_piece` (§8.3) absorbs it immediately. With
   continuous values there's no special "not yet crossed" rule.

### 8.3 Live adaptation (all in v1)

`ln rate = ln prior(thickness, size) + a_studio + e_recent + e_sibling + e_piece`

| Factor | Learned from | Shrinkage `n0` |
|---|---|---|
| `a_studio` | All past training samples | 5 |
| `e_recent` | The **last 10** pieces that reached the target, weighted by order (newest ×1, oldest ×0.5). Robust to breaks between sessions. | 3 |
| `e_sibling` | Pieces in the same group that have already reached the target | 2 |
| `e_piece` | The piece's own observations: the slope of dryness against effective hours | 2 |

Each factor is a weighted mean of residuals shrunk toward 0:
`Σ wᵢ·residᵢ / (Σ wᵢ + n0)`. Thickness and size factors fitted from history
come later, after at least 10 samples.

### 8.4 Training samples

One sample per piece: the effective hours from the start to **3.0**.

- **Start:** the add observation (dryness `d_s` < 2.8).
- **Reaching 3.0:**
  - Take the last observation below 3.0 and the first observation at or
    above 3.0 (a user, flip or end observation), and interpolate linearly in
    dose between them.
  - The sample's weight shrinks with the dose gap between those two
    observations, and the sample is dropped if that gap exceeds the dose
    itself.
- `rate_sample = (3.0 − d_s) / dose`.

| History | Use |
|---|---|
| Normal (reaches ≥ 3.0 by observation or trim) | ✓ |
| Trimmed with `end_dryness` < 3.0 (covered early) | No sample. Its observations still feed `e_piece` while it is live. |
| Discarded after reaching 3.0 | ✓ |
| Discarded before reaching 3.0, or `mistake` | ✗ |
| Spans a gap-filled stretch (`offline`, `sensor_gap`) of ≥ 1 h | ✗ |
| Contains an `inconsistent` observation | ✗ |
| Added at or above 2.8 | ✗ |
| Group of identical pieces | One sample per group (the median) |
| Residual > 3σ | Dropped |

### 8.5 Status per piece

| Status | When |
|---|---|
| `drying` | Predicted dryness < 2.8 |
| `probably_ready_check` | Predicted ≥ 2.8, but the last observation is below the band |
| `ready_now` | Last observation in 2.8–3.6 **and** predicted dryness < 3.6 (so "ready" **expires**, review C1) |
| `may_be_past_check` | Predicted > 3.6 without an observation confirming it |
| `past_trim` | Last observation > 3.6 |
| `ended` | Trimmed or discarded |

### 8.6 Accuracy tracking

On every observation insert, store what was predicted at that moment
(`eta`, window, method). Comparing these snapshots with what actually
happened is the honest accuracy measure (review B7).

---

## 9. Follow-up questions

There are few left. At most one per observation, always skippable, never
repeated for the same piece.

| Kind | Trigger | Options |
|---|---|---|
| `much_slower` | ≥ 1.5 below the prediction. **Off during the learning period.** | `Still drying` · `I covered it / took it off` (opens Trim/Discard with a "when" picker) |
| `went_wetter` | ≥ 1.5 wetter than the previous observation | `Last entry was wrong` (opens it for correction) · `It's fine` |

A smaller backward step, more than 0.5, is treated as judgment noise. It is
accepted without a question and flagged `inconsistent`, which excludes it
from training. Skipping a question means *unknown* (review C4).

---

## 10. UX implications

- **Now:** the climate card, the trim card (§7.3), a shelf summary ("8 pieces
  · oldest 3d 4h"), and the back-online banner (§6).
- **Sliders everywhere dryness is entered**: add, check-in, flip, trim and
  discard. The slider's design is the most important UI work in the app (FE
  plan §5).
- **Check-in** is one piece per screen: the photo, a big slider pre-filled
  with the last observed value, and Save & next / Skip.
- **Settings:** studio time zone (for displaying times), sensor
  calibration, and backup download.

---

## 11. Edge cases

| Case | Handling |
|---|---|
| Added at 2.8–3.6 | `ready_now`. No training sample. |
| Added above 3.6 | Allowed, `past_trim`. The add sheet warns. |
| Covered, but the user forgot to mark it | It looks slower and slower and ends up in "check these". After the learning period, `much_slower` offers "I covered it", which ends it backdated. The 3σ rule keeps it out of training. |
| Device switched off with pieces listed | §6: an `offline` log entry, the banner, check-these, a wide window |
| The device was off when the deadline passed | Those pieces are `may_be_past_check` and the card says so |
| Clock wrong after power-on | Recording waits for sync; nothing is written backwards |
| Setpoint changed while pieces dry | Measured dose up to the change, the setpoint forecast after |
| Shelf numbers reused across sessions | Numbers are unique only on the shelf. History shows "#3 · Oct 12". |
| Two people observe the same piece close together | Both are kept. The latest drives the ETA. Training uses the median within 30 minutes. |

---

## 12. Deferred

- A setpoint advisor ("set 72 % now to be ready Saturday 09:00"), once the
  controller exists.
- A learned flip factor; thickness and size factors from history; a VPD
  exponent.
- Bone-dry predictions for kiln loading.
- Alerts (ntfy or similar).
- An SHT4x heater pulse against drift.
- Configurable trim band.
