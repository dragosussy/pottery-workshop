# Ideas and next steps

Things that could come next, roughly in order of how much they'd help. None
are promises. The rule from the rewrite still holds: add something when using
the app shows it's missing, not because it might be nice. Each idea says what
would justify it and roughly what it costs.

## Before the first real session

Not features: things to do once, with the hardware.

- **Wire the pump and check it.** Set `PUMP=gpio`, `PUMP_PIN` and
  `PUMP_ACTIVE_LOW` in `docker-compose.yml`. Then use Run 1 s and check the
  pump runs for one second and stops. If it runs while the app says it's off,
  flip `PUMP_ACTIVE_LOW`.
- **Calibrate the sensor** with a salt test (sealed box, damp table salt,
  8 h, should read 75 %). Enter the offset in Settings.
- **Fit the Pi 5's RTC battery** so checks made right after boot get the
  right time. The app no longer waits for the clock to sync.
- **Move the old database aside** on the Pi (`/data/humidity.db`) if the first
  version ever ran there.
- **Upload a photo from the phone itself.** The photo pipeline was fixed by
  moving resizing to the server, but it has only been tested from desktop
  Chromium. An iPhone should hand over a JPEG; check that one goes through
  and comes out upright.
- **Commit the rewrite** (nothing is committed yet), and consider a small CI
  job that runs `make test` on every push.

## Pump and climate

- **A push notification when the pump stops for safety.** A `no_rise` stop
  usually means the tank is empty, which matters most when nobody is in the
  studio. Post to ntfy.sh (or Telegram) from `pump.go` when the loop latches
  `stopped`. About 20 lines, plus one env var for the topic. Needs outbound
  internet from the Pi.
- **A water-level float switch** on a second GPIO pin, read before each
  pulse. It would stop the pump before it runs dry instead of after six
  misses. Worth it if the tank runs dry more than once. About 30 lines, plus
  hardware.
- **Lowering humidity.** The pump can only add moisture; above the target the
  app just waits. A small fan on a second relay, run when the shelf is well
  above the target, would make the target two-sided. Only if wet pieces stay
  too wet in practice.
- **Pump runs on the chart.** Log each run (start, seconds) to a small table
  and shade the runs on the humidity chart. Shows how hard the pump works and
  makes leaks in the plastic easy to spot. About 40 lines across Go and the
  chart.
- **Per-sensor history.** Only the room mean is stored. With two sensors (top
  and bottom shelf, or one near the nozzle), a per-sensor line would show
  whether the shelf is even. Add a `sensor` column to `readings`.

## Estimates

- **Temperature in the drying speed.** The model uses `100 − RH` only. If a
  cold or hot spell visibly throws estimates off, switch `speed` to the
  vapour-pressure deficit (temperature and humidity). Readings already store
  temperature, so it's about 10 lines in `predict` plus a mean temperature
  per stretch.
- **A learned factor per wall thickness.** Once a thickness class has 3 or
  more trimmed pieces, give it its own `k` instead of the one studio factor.
  Small change in `predict.go`; useful if thick pieces are consistently off
  while thin ones are right.
- **"How good were the guesses".** For trimmed pieces, compare the estimate
  made at the last check before trimming with when it was actually trimmed.
  Show the typical error on the Finished page. It tells you how far to trust
  "ready around", and whether the ideas above are needed.
- **A range instead of a single time**, such as "Sat 10:00–16:00". It was
  dropped as complexity, and is worth bringing back only if single times turn
  out to mislead. Base the width on the error measured by the previous idea,
  not on theory.
- **"Not checked since the Pi was off".** After a long gap, estimates still
  extrapolate from the last check. A badge on pieces whose last check is older
  than the latest gap in `readings` would prompt a check. About 20 lines.

## Pieces and the app

- **Backdating a check** ("felt it an hour ago, entering it now"). It was
  dropped as rarely needed. If it comes back, keep it to one optional time
  field on the check form.
- **Finished pieces: filter and simple stats**, such as how many were trimmed
  vs discarded per month, and the average drying time per form. Useful once
  there are a few dozen finished pieces.
- **Export** pieces and checks as CSV from Settings, for a spreadsheet. About
  30 lines.
- **Photos in backups.** The daily backup is the database only. Either add
  `photos/` to the download as a zip, or document copying the folder.
- **Off-device backups.** Everything lives on the Pi's SD card. A nightly
  `rclone` copy of `/data/backups` and `/data/photos` to a NAS or cloud folder
  would cover a dead card. This is a cron job, not app code.

## Deliberately not planned

These were in the first version and removed. They shouldn't come back
without a concrete problem they solve: live push updates, the visit planner,
follow-up questions, the in-memory drying-dose index with gap filling, confidence levels,
soft deletes, the mock backend, and the colour picker. See the
"Deliberately dropped" list in [design.md](design.md).
