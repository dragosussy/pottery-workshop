# Frontend implementation plan

Status: draft, **revision 3** (2026-10-31). It follows `00-domain-brief.md`
rev. 3 and `api-contract.md` rev. 3:
- plain HTTP, so no service worker
- **precise, continuous dryness sliders**
- a deadline-aware trim card
- sessions with the device switched off in between

## 1. Principles

1. **The slider is the instrument.** The potter's gut feeling is the
   measurement the whole app learns from. The dryness slider gets the most
   design and testing effort in the app (§5.1).
2. **Phone first, one hand, messy hands.** Touch targets are at least 48px,
   and actions sit within thumb reach. Nothing depends on hover, and there
   is no typing on the main paths.
3. **Answer the two questions fast.** Opening the app shows how the shelf is,
   and when to come in.
4. **Don't steer the judgment.** When recording dryness, the slider starts at
   the *last observed* value. The model's guess isn't shown in check-in, so it
   can't bias the reading.
5. **Honest uncertainty.** Windows, not minutes. The app says when it is
   still learning, and when it can't tell because the controller was off.
6. **A small surface.** Six screens, a few sheets, no component library.

## 2. Stack

| Concern | Choice | Why |
|---|---|---|
| Framework | Vue 3 (`<script setup>`) + TypeScript | The owner knows Vue |
| Build | Vite | |
| Routing | Vue Router (history mode) | Go serves an SPA fallback |
| State | Pinia: `live`, `shelf` and `reference` stores | |
| HTTP | A thin typed `fetch` wrapper | |
| Types | Hand-written `src/api/types.ts` (contract §8) | |
| Charts | uPlot behind `<TimeChart>` | About 50 KB, built for time series |
| Dates | `Intl` + a small `duration.ts` | |
| Photos | `<input type="file" accept="image/*" capture="environment">`, `createImageBitmap` + canvas | Works over plain HTTP |
| Home screen | A web manifest, icons, and `apple-mobile-web-app-*` meta tags. **No service worker**, since that needs HTTPS (owner decision). | "Add to Home Screen" still gives an app icon, and on iOS a full-screen app. |
| Styling | Plain CSS with design tokens, light/dark | |
| Tests | Vitest + Vue Test Utils, one Playwright smoke test | |

Location: `web/`. `npm run build` writes `web/dist`, which the Go binary
embeds.

**What plain HTTP means for the app**
- There's no offline shell. When the controller is switched off, the app
  can't load at all, because the controller serves it. A tab that's already
  open keeps showing the last known state, cached in `localStorage`, with a
  banner (§6).
- There is no `crypto.randomUUID`, and nothing needs it (contract §1.2).

## 3. Information architecture

```
Bottom bar:   [ Now ]   [ Shelf ]   [ History ]        ⚙ in the Now header
                    (+) floating action → Add pieces

/                Now (home)
/shelf           Pieces on the shelf, grouped by status; Select for group actions
/check           Check-in: one piece per screen
/pieces/:id      Piece detail
/history         Ended pieces
/climate         Humidity/temperature chart
/settings        Time zone, sensors, colours (trial), backup, about
```

Sheets are addressed by a query param (`?sheet=add`, `?sheet=flip&ids=…`,
`?sheet=trim&ids=…`, `?sheet=discard&ids=…`, `?sheet=observe&ids=…`), so the
back button closes them.

## 4. Screens

### 4.1 Now (home)

1. **Banners**, only when relevant:
   - **Back online:** "Back on after 9 days · **3 pieces** still listed on
     the shelf" → `Check them`, which opens check-in with those pieces, or
     `Mark all trimmed / discarded`. Shown while
     `status.last_offline.pieces_to_check > 0`.
   - **Controller unreachable:** "The drying controller is offline (probably
     switched off) · showing data from Thu 18:05". Everything below turns
     grey.
   - **Clock not synced:** a small note, usually only for a minute after
     power-on.
2. **Climate card.** The room's humidity in large type, the temperature, a
   6 h sparkline and a freshness dot. Tapping it opens `/climate`. It shows a
   stale state when no sensor is `ok`.
   - **Target row** under it: "Target **82%** · on target", or "· shelf 5%
     below" when the shelf is more than 3 points off, or "none · predictions
     follow the last day". Tapping it opens the **target sheet**:
     - a precise `HumiditySlider`: 50–98 %, 1 % steps, fine scrubbing and
       ± nudges like the `DrynessSlider`, and a ring at the shelf's humidity
       now;
     - a live preview of the trim card with that target ("With 86 %: Come Sat
       09:00–11:30 · 5 ready"), from `GET /control/preview`, debounced while
       dragging. The potter raises the target until the visit lands on a day
       that suits them;
     - `Set 86%`, plus `No target` to clear it;
     - a note on how the pump holds the target (pulse, rest, only adds
       moisture), or, without a pump, that the potter holds it by hand.
3. **Trim card**, the main element. Its states (`mid-batch-changes.md`
   §7.3):

   | State | Shows |
   |---|---|
   | Ready now | "**3 ready now** · best before Sat 14:00". The pieces' thumbnails. |
   | Next visit | "Come **Sat 09:00–11:30**" · "5 ready, 2 maybe". Tapping it lists the pieces in each group. |
   | Nothing soon | "Nothing to trim in the next few days" |
   | Empty shelf | "The shelf is empty", plus `Add pieces` |

   - **Learning** adds "rough guess · learning your shelf (2 of 5)" under
     any of these states.
   - **Confidence** is a word (rough / decent / good), never a percentage.
   - **Times** are shown in the studio's time zone, as relative day names:
     "today", "tomorrow", "Sat".
4. **Shelf summary.** "**8 pieces** · oldest **3d 4h**", a stacked bar of
   pieces by stage, and the first three "check these". Ages tick
   client-side.
5. **Actions.** `Check pieces` (main button) and `Add pieces`.

### 4.2 Add pieces (sheet)

1. **Photo (optional).**
   - Tapping the tile opens the camera. The image is resized on the device
     and **uploads in the background** while the form is filled in.
   - The tile shows the thumbnail, progress, ✕ to remove, and Retry if the
     upload fails.
   - Add waits for an upload in progress, and never blocks on a failed one.
2. **Form**: chips, defaulting to the last one used.
3. **Dryness**: `DrynessSlider`, defaulting to the last one used (wet, 1.0,
   the first time).
4. **Size**: S / M / L / XL. **Thickness**: Thin / Medium / Thick.
5. **Quantity stepper** (1–20).
6. **When**: Now, or chips for 1h ago · 3h ago · this morning · pick.
7. "More": label, notes.

Buttons: **Add** and **Add & next**. Add & next keeps the values and resets
the photo, quantity and label. The toast shows the numbers in large type:
"**#3 #4 #5 #6**: write them on the bats".

If the POST gets no response, the app doesn't retry. It refetches the shelf
and says "Couldn't confirm. Check the shelf before adding again" (contract
§1.2).

### 4.3 Check-in (`/check`): one piece per screen

Gut-feel readings need focus, and a list of sliders scrolls badly, so
check-in shows **one piece at a time**:

```
┌──────────────────────────────┐
│ 3 of 8                    ✕  │
│ ┌──────────────────────────┐ │
│ │          photo           │ │
│ └──────────────────────────┘ │
│ #6  Mug · M · medium wall    │
│ in 2d 4h · flipped           │
│ last: Soft leather hard ·    │
│       firming up (14h ago)   │
│                              │
│  Leather hard · a touch soft │  ← live readout
│ ─[−]═══════════◉════════[+]─ │  ← DrynessSlider
│                              │
│ Judge the foot               │
│ [Flip] [Trim] [Discard]      │
│                              │
│ [  Skip  ]  [ Save & next ]  │
└──────────────────────────────┘
```

- **Order:** the "check these" pieces first (probably ready, back from
  offline), then the rest by number. Starting from a banner or a selection
  limits the list to those pieces.
- **The slider starts at the last observed value**, drawn as untouched (a
  hollow thumb with the label "same as last time?"). The model's estimate is
  **not shown** here, so the reading isn't biased.
- **Save & next** records the slider value, even if it wasn't moved, because
  "unchanged" is a real reading. It sends one `POST /observations` with one
  item. **Skip** records nothing.
- **Saving per piece** means nothing is lost if the phone locks or the
  signal drops halfway through the shelf.
- **Follow-up questions** appear on the same card, after Save, before
  moving on. They are skippable.
- **Flip / Trim / Discard** open their sheets. Flip pre-fills its dryness
  from the slider's current value.
- **No swipe navigation.** Horizontal swipes belong to the slider; the
  buttons move between pieces.
- **End screen:** "Checked 6 · skipped 2". It shows the updated trim card,
  and "#3 and #5 are ready: trim them now?" when there are ready pieces.

### 4.4 Shelf (`/shelf`)

- Pieces are grouped by status: **Ready now** → **Check these** → **Soon**
  (< 12 h) → **Later** → **Past trim**.
- Each row shows the thumbnail, #number, form + size/thickness, age, the
  current dryness label, the ETA window, and a flipped badge.
- **Select** (a button, not long-press) offers "select group"
  ("Tue · 4 mugs"). The action bar has **Check · Flip · Trim · Discard**.
  Check opens check-in limited to the selection.

### 4.5 Piece detail (`/pieces/:id`)

- The photo, #number, specs, label and age.
- **Lifecycle line:** Added → Flipped → Trimmed, with times. Tapping a time
  corrects it.
- **Prediction card:** the window, best before (`ready_until`), confidence,
  the main uncertainty reason ("wider: the controller was off for 2 days"),
  and the basis.
- **Dryness history:** a list of the observations, each showing its label
  and source, and editable or deletable with Undo.
- Actions: **Record dryness** (a sheet with the slider; here the model's
  estimate *is* shown as a faint marker, because this is not the check-in
  flow), **Flip**, **Trim**, **Discard**, **Edit details**.
- (F5) A small chart of the piece's dryness over time, with room humidity
  behind it.

### 4.6 Flip, Trim and Discard (sheets)

All three send `PATCH /pieces/{id}`, or `PATCH /pieces` with `ids`, setting
`state` (contract §3.5).

- **Trim:**
  - The `DrynessSlider` "How dry is it now?", pre-filled at **3.0**, plus
    `WhenChips`.
  - A hint: "*Covering it to trim later? Set how dry it is now.*"
  - One tap for the usual case.
- **Discard:** an optional slider (off until touched), a note, an "Entered by
  mistake" checkbox, and `WhenChips`.
- **Flip:** `WhenChips`, plus an optional slider pre-filled with the last
  observed value. Skipping it records the flip alone.
- **Undo toast:** sends the same PATCH with the previous `state`. It's safe to
  retry (same state = no-op), and works from another device too.

### 4.7 History (`/history`)

Ended pieces, newest first and paginated, with Trimmed/Discarded filters.
Each row shows the thumbnail, "#6 · Oct 29", the form, and "trimmed after
2d 6h at Leather hard". Tapping a row opens the read-only piece detail, which
still allows corrections and "Back on the shelf".

### 4.8 Climate (`/climate`)

- uPlot with range chips `6h · 24h · 7d · 30d` and humidity / temperature /
  VPD toggles.
- Gaps are shaded, with **"controller off"** periods labelled. Calibration
  markers are shown.
- Min, average and max for the range. The setpoint line is reserved for the
  future control loop.

### 4.9 Settings (`/settings`)

- **Time zone**, pre-filled with the phone's. It is only used to show times.
  There is no studio schedule: the potter goes whenever they want.
- **Pump:** running or off and why ("waiting for the mist to settle"), the
  runtime over the last 24 h, `Run 5 s` / `Stop` for checking the wiring and
  priming after a refill, the limits in words, and `Resume` after a safety
  stop. Without a pump: how to configure one. A safety stop also shows a red
  banner on Now with `Resume`, and the target row adds "· pump running" or
  "· pump stopped".
- **Colours (trying out):** a palette picker (Tide, Sage, Cobalt, Kiln), kept
  per device. `?palette=name` in the URL also switches it. It's temporary,
  until one palette sticks (§5.3).
- **Sensors:** the live reading and offset for each sensor, a salt-test guide
  (sealed box, damp salt, wait 8 h; entering the reading sets the offset),
  and "I replaced this sensor".
- **Backup:** a "Download backup" link (`GET /api/v1/backup`) with a note:
  "Keep a copy off the controller's SD card. Photos aren't included."
- **About:** versions, clock sync state, and how to switch off cleanly: "Use
  the controller's power button, don't pull the plug."

## 5. Components

### 5.1 `DrynessSlider`: the core component

The owner's requirement: precise, sensitive and trustworthy with muddy
fingers. A native `<input type="range">` can't meet it. On iOS, tapping the
track does nothing, the thumb is small and sits under the finger, and a
vertical scroll easily moves the value. So this is a **custom pointer-events
slider**.

**Look**
- A full-width track (the screen minus the gutters), with a **56px touch
  area** and a 12px visual bar shaded in clay colours from dark wet clay to
  pale bone dry.
- **Landmarks** at 0–5, with tick marks and short labels beneath (very
  wet · wet · soft LH · **leather hard ✂** · hard LH · bone dry). On narrow
  screens the labels alternate between two rows.
- The **trim band** 2.8–3.6 is shaded as a subtle band behind the track.
- A **thumb** 36px across. While dragging, a **readout bubble** floats above
  the finger (the finger covers the thumb), showing the label and qualifier:
  "Leather hard · a touch soft".
- A large **readout** below the track repeats the label, because it's the
  part read at a glance.
- **Markers:**
  - A hollow ring at the last observed value, always shown.
  - A faint tick for the model's estimate, only in piece detail, never in
    check-in.

**Behaviour**
- **Value:** 0.0–5.0 in steps of 0.1 (51 positions). Over about 330px, that's
  about 6.5px per step, so one finger can hit any step.
- **Starting a drag:** the slider takes the gesture only once horizontal
  movement passes 6px before vertical movement passes 10px (`touch-action:
  pan-y`). A vertical scroll never changes the value. A tap with no movement
  moves the thumb to the tapped point.
- **Fine scrubbing:** while dragging, moving the finger *down, away from the
  track* reduces sensitivity: 1× at the track, ½× after 60px, ¼× after
  120px. A small "fine" hint appears. This is the same idea as iOS audio
  scrubbing.
- **Nudge buttons** `−` and `+` at the two ends (48px) change the value by
  ±0.1, and repeat while held. They give exact adjustment without dragging.
- **Haptic tick** (`navigator.vibrate(8)`, which works on Android over HTTP;
  iOS has no haptics): when crossing a landmark, and when entering or
  leaving the trim band.
- **Untouched state:** a hollow thumb with "same as last time?" until it is
  moved or saved, so the potter can see whether they changed anything.
- **Accessibility:** `role="slider"`, `aria-valuemin`, `aria-valuemax`,
  `aria-valuenow` and `aria-valuetext` (the label). Arrow keys move ±0.1,
  PageUp/PageDown ±1.0.

**Labels** (`utils/dryness.ts`): the nearest landmark plus a qualifier from
the distance `d`:

| `d` | Qualifier | Example (near 3) |
|---|---|---|
| \|d\| < 0.15 | none | "Leather hard" |
| −0.35 < d ≤ −0.15 | "a touch soft" | 2.8 |
| d ≤ −0.35 | "soft side" | 2.6 |
| 0.15 ≤ d < 0.35 | "a touch firm" | 3.2 |
| d ≥ 0.35 | "firm side" | 3.4 |

Between soft-leather-hard and wetter landmarks, "soft"/"firm" become
"wetter"/"firming up". The exact wording is to be tuned with the owner.

**Validation plan:** it is built first, as a standalone page (`/dev/slider`),
and tried on the owner's phone **with wet or muddy fingers** before any
screen uses it. Things to check:
- whether any value can be set within ±0.1 in under 3 s
- that scrolling never moves it
- that the readout is legible at arm's length

The thresholds above are tuned from that test.

### 5.2 Other components

| Component | Notes |
|---|---|
| `DrynessChip` | Label + colour swatch, never colour alone |
| `EtaWindow` | "in 3–6h", "tonight 20:00–23:00", "Sat morning", in studio time, plus confidence and best-before |
| `TrimCard` | The states from §4.1 |
| `Segmented`, `Chips` | Size, thickness, form, days |
| `WhenChips` | Now / 1h ago / 3h ago / this morning / pick. "Now" sends no time. The others use the server clock offset. "This morning" is 08:00 in studio time. |
| `PhotoPicker` | Camera → resize → background upload → `photo_id` |
| `PieceRow`, `PieceCard` | List row, and the check-in card |
| `BottomSheet` | Route-query driven, swipe down to close (a vertical gesture, so it doesn't conflict with the slider) |
| `TimeChart` | uPlot wrapper with gap shading and markers |
| `Age`, `Toast` (+ undo), `FollowUpPrompt`, `Banner` | |

### 5.3 Colours

The original warm "fired terracotta" browns were rejected by the owner
(2026-10-31).
- All colours live in `src/styles/tokens.css` as **palettes**: blocks that
  apply to `[data-palette="…"]`, each with a light and a dark version.
- Each palette sets the surfaces, text, accent, trim-band tint, and the wet →
  dry scale (dark and saturated = wet, pale = dry).
- State colours (ok / warn / danger) are shared by all palettes.
- `src/theme.ts` picks the palette from `?palette=`, then `localStorage`,
  then the default (`tide`).
- Once one palette sticks, it becomes the plain `:root` tokens, and the
  picker and the other palettes are deleted.

## 6. Data flow

- **`stores/live.ts`** owns `GET /status` and the `EventSource`
  (`readings` → the climate card; `changed` → a debounced refetch; `hello` →
  a refetch).
  - It falls back to polling every 10 s after repeated stream failures.
  - It closes the stream while the page is hidden.
  - It refetches every 5 min while visible.
  - It caches the last `/status` in `localStorage` (inside try/catch), for
    the "controller offline" state.
- **`stores/shelf.ts`** owns `GET /shelf`, also cached for the offline state.
- **`stores/reference.ts`** fetches `GET /reference` once.
- **Writes:**
  - PATCH and DELETE are retried once on a network error, since they are
    idempotent.
  - POST is **never retried automatically**. With no response, the app
    refetches the shelf and asks the user to check (contract §1.2).
- **Clock:** `useServerClock` keeps the offset from `server_time`.

## 7. Project layout

```
web/
  index.html  public/manifest.webmanifest  public/icons/*
  vite.config.ts                 # proxy /api → http://localhost:8080 in dev
  src/
    main.ts  App.vue  router.ts
    api/      client.ts types.ts endpoints.ts
    stores/   live.ts shelf.ts reference.ts
    composables/ useServerClock.ts useSheet.ts useStickyDefaults.ts usePhotoUpload.ts usePointerSlider.ts
    components/  DrynessSlider.vue DrynessChip.vue EtaWindow.vue TrimCard.vue
                 Segmented.vue Chips.vue WhenChips.vue PhotoPicker.vue PieceRow.vue PieceCard.vue
                 BottomSheet.vue TimeChart.vue Age.vue Toast.vue FollowUpPrompt.vue Banner.vue
    views/       NowView.vue ShelfView.vue CheckView.vue PieceView.vue
                 HistoryView.vue ClimateView.vue SettingsView.vue DevSliderView.vue
    sheets/      AddPieces.vue Observe.vue Flip.vue Trim.vue Discard.vue
    styles/      tokens.css base.css
    utils/       dryness.ts duration.ts format.ts image.ts
```

## 8. Dev workflow

- Run `SENSOR_DRIVER=sim DEMO_SEED=true go run ./cmd/humidity` together with
  `npm run dev` (Vite proxy). The demo seed includes:
  - pieces on the shelf
  - finished pieces
  - a past `offline` period with pieces "left on the shelf"

  so every card state can be seen.
- `make dev` runs both.
- To test on a phone, open `http://<laptop-tailscale-name>:5173`, with Vite
  set to `--host`.

## 9. Testing

- **Unit:**
  - `dryness.ts` labels and qualifiers at each boundary
  - `usePointerSlider`: the gesture-capture thresholds, the fine-scrub
    scaling, nudge, and keyboard
  - `EtaWindow` and `WhenChips` in the studio time zone, including DST
  - check-in: Skip sends nothing, and an untouched Save sends the last value
  - POST failure handling with no retry
- **Component:** `DrynessSlider` (aria, untouched state, markers), each
  `TrimCard` state from fixtures, `AddPieces` photo states.
- **E2E smoke** (Playwright, mobile viewport): add 3 pieces
  → check-in → trim one, then undo → the trim card shows a visit.
- **Manual:** the slider test on the owner's phone (§5.1).

## 10. Milestones

These align with the backend plan §13.

| # | Scope | Done when |
|---|---|---|
| F1 | Scaffold, tokens, API client, **DrynessSlider + `/dev/slider`, tested on the owner's phone**, Now climate card (stream, stale/offline/clock states), Settings (time zone, sensor offset + salt test, backup link) | Live calibrated humidity on the phone, and the slider has passed the muddy-finger test |
| F2 | Add pieces, Shelf, Piece detail, Flip/Trim/Discard sheets with Undo, History | Pieces tracked end to end |
| F3 | Check-in (one piece per screen, save per piece, follow-ups), observation edit/delete, Select + group actions | The daily loop is complete |
| F4 | Trim card (all states), the learning and back-online banners, check-these, Climate screen | Predictions visible and trustworthy |
| F5 | Home-screen manifest and icons, piece dryness chart, polish, accessibility pass | v1 |

**Deferred:** alerts (push needs HTTPS anyway, so this would be ntfy or
similar from the backend), an offline write queue, the control UI, multiple
photos per piece, and observer names for studio mates.
