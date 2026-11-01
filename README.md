# humidity-control

A drying-shelf controller for pottery. A Raspberry Pi 5 next to a
plastic-wrapped shelf:

- reads the shelf's humidity and temperature (SHT31/SHT4x over I²C);
- runs a misting pump to hold a humidity target;
- tracks the pieces drying on the shelf and your dryness checks, and
  estimates when each will be leather hard, learning from your trims;
- serves a phone web app over Tailscale at `http://atelier`.

Go backend (one binary, SQLite) with the Vue app embedded in it.

## Docs

- [docs/design.md](docs/design.md): what the app does and how it's built:
  features, the estimate maths, the pump loop, data, API, code layout.
- [docs/ideas.md](docs/ideas.md): next steps and ideas for later.

## Develop

Needs Go 1.26 and Node 22+.

```sh
make dev     # backend with a simulated sensor and pump (data in ./data),
             # plus the Vite dev server on http://localhost:5173
make test    # gofmt, go vet (Mac and Pi), Go tests
npm --prefix web test           # web unit tests
npm --prefix web run type-check
npm --prefix web run lint
make build   # bin/humidity with the web app embedded
make build-pi
```

The simulated sensor reacts to the simulated pump, so setting a target above
the shelf's humidity shows the pump at work. Delete `./data` to start over.

To use the dev server from a phone on the tailnet, open
`http://<this-machine>.<tailnet>.ts.net:5173`.

## Deploy to the Pi

1. On the Pi: enable I²C (`raspi-config`), install Docker, join the tailnet
   with the machine name `atelier`, and fit the RTC battery.
2. Create `.env.dev` here with `PI_USER`, `PI_HOST` and `PI_DIR`.
3. `./deploy.sh dev` builds the arm64 image, pushes it, copies
   `docker-compose.yml` to the Pi and restarts the app.

The app is then at `http://atelier` from any device on the tailnet.

### Wiring the pump

The pump is off (`PUMP=none`) until you set it up in `docker-compose.yml`:

```yaml
- PUMP=gpio
- PUMP_PIN=17            # BCM GPIO number (17 = physical pin 11)
- PUMP_ACTIVE_LOW=true   # most cheap relay boards switch on when their input is low
```

`PUMP_ACTIVE_LOW` has no default, because a wrong guess would run the pump
while the app shows it off. After deploying, press **Run 1 s** on the home
screen and check the pump runs for a second and stops.

Other settings (`PUMP_PULSE`, `PUMP_REST`, `PUMP_MAX_RH`, `SENSOR`,
`SENSOR_ADDRS`, `DATA_DIR`, `ADDR`) are listed in
[docs/design.md](docs/design.md#configuration-environment).
