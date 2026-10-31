# Domain brief: pottery drying controller

The shared starting point for the API contract, the frontend plan and the
backend plan. **Revision 3** (2026-10-31). It records the owner's decisions so
far:

- **2026-10-30:** no batches, one plastic-enclosed shelf, one clay body, four
  piece states, covered pieces aren't tracked, SHT31/SHT4x sensors, an
  optional photo per piece.
- **2026-10-31:**
  - plain HTTP over Tailscale (no HTTPS)
  - precise, continuous sliders
  - a trim recommendation driven by the pieces' deadlines, with **no studio
    schedule**: the potter goes whenever they want, and steers the drying
    (humidity target, plastic) when a day doesn't suit
  - bounded logs, no deploy rollbacks
  - throwing happens in sessions, and the device is off in between

## What exists today (backend)

- A Go 1.26 service running in Docker on a Raspberry Pi 5.
- `internal/sensor`: `Reading{Sensor, Temperature °C, Humidity %RH, At}`.
  Drivers: `sht` (Sensirion SHT3x/SHT4x over I²C) and `sim` (for laptop
  development).
- `internal/monitor`: polls every sensor every `POLL_INTERVAL` (5s by default)
  and keeps the latest reading per sensor in memory.
- No HTTP server, no persistence and no control loop yet. A
  humidifier/pump control loop is planned.

## Users and context

- One potter, possibly a few studio mates. Mostly used on a **phone**, often
  in the workshop with clay on their hands, and also from home to decide
  *when to come in*.
- **Network:** plain **HTTP** over a private Tailscale tailnet
  at **`http://atelier`** (long form `http://atelier.tailb9c0c2.ts.net`).
  `atelier` is the Pi's Tailscale machine name, which MagicDNS resolves for
  tailnet devices only, so no domain is needed. The name is generic on
  purpose, because more studio apps may run on the same Pi later. No
  HTTPS. Anything that needs a secure context (a
  service worker, offline app shell, PWA install, `crypto.randomUUID`) is left
  out by design.

## How the studio works

- **Throwing happens in sessions.** Many pieces go on the shelf at once,
  then there is a break until the next session.
- **The device is usually switched off between sessions**, often for days or
  weeks. The software treats long gaps as normal, not as failures:
  - When the device comes back on, the clock may be wrong until it syncs.
  - The history has a gap.
  - Any pieces still listed on the shelf need a check.
- **Turn it off with the Pi 5's power button** (a clean shutdown), not by
  pulling the plug, to protect the SD card.

## The drying space

- **One shelf enclosed in plastic** to keep moisture in. The air inside is
  roughly uniform, so one sensor (or the mean of a few) represents every
  piece.
- It runs damp, likely 70–90 % RH. That is why the sensors are SHT31/SHT4x,
  which are far more accurate than the DHT22 in that range, plus a
  calibration offset set with a salt test.
- No fan, and only one clay body.
- Later, a controller holds one **global humidity target** for the shelf.

## Core concepts

**Dryness**: a **continuous** value from 0.0 to 5.0 (step 0.1), set with a
precise slider. The six named stages are reference points on the scale, not
buckets. The potter's gut feeling is the measurement, so "leather hard, but
still a touch soft" (2.8) is a valid and useful answer.

| value | key | label |
|---|---|---|
| 0 | `very_wet` | Very wet |
| 1 | `wet` | Wet |
| 2 | `soft_leather_hard` | Soft leather hard |
| **3** | `leather_hard` | **Leather hard ← trim target** |
| 4 | `hard_leather_hard` | Hard leather hard |
| 5 | `bone_dry` | Bone dry |

A piece is **trimmable** while its dryness is in the trim band **2.8–3.6**.

**Piece**: an item on the shelf, with its own clock. There are no batches.
- Captured when added: form (bowl / mug / plate / vase / other), initial
  dryness, size (S/M/L/XL), wall thickness (thin / medium / thick), an
  optional photo, an optional label, and the time added (backdatable).
- Adding several identical pieces at once (quantity) creates a **group**:
  separate pieces that share one photo and can be acted on together.
- Each piece gets a short **shelf number**: the lowest number not used by
  anything currently on the shelf. It's written in pencil on the bat.
- **States:** `in` → `flipped` → `trimmed` | `discarded`. Covered pieces
  aren't tracked: covering one means marking it trimmed (at its current
  dryness) or discarded.

**Observation**: the piece's dryness at a moment, recorded at any time while
the piece is on the shelf (backdatable). This history is what the
predictions learn from.

**Climate**: humidity and temperature, recorded every minute while the
device is on, independent of pieces. Humidity changes while pieces dry are
captured by this recording and turned into each piece's **drying dose**
(`mid-batch-changes.md` §5).

**Trim recommendation**: when to come, as one window that catches the most
pieces before the first one passes the trim band. It covers both pieces that
are ready now and pieces that will be. Any time of day counts; there is no
studio schedule. There is no real data at the start, so the app runs in a
visible **learning period** until 5 pieces have produced usable data.

## Defaults the plans assume

- Frontend: Vue 3 + Vite + TypeScript, mobile-first, built into static files
  and served by the Go binary (a single container). There is a web manifest
  and icons for "Add to Home Screen"; no service worker.
- Persistence: SQLite (a pure-Go driver) on a Docker volume. Photos are
  stored as files on the same volume. Backups are taken daily and at startup,
  and can be downloaded from Settings.
- No auth (tailnet only). Docker logs are size-bounded.
