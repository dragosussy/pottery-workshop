# humidity

A drying-shelf controller for pottery. A Raspberry Pi 5 next to a
plastic-wrapped shelf:

- reads the shelf's humidity and temperature (SHT31/SHT4x over I²C);
- runs a misting pump to hold a humidity target;
- shows the humidity and temperature on a small screen next to it;
- tracks the pieces drying on the shelf and your dryness checks, and
  estimates when each will be leather hard, learning from your trims;
- serves a phone web app over Tailscale at `http://atelier`.

Go backend (one binary, SQLite) with the Vue app embedded in it. The code is
in [apps/humidity/](../../../apps/humidity).

## Docs

- [Hardware](hardware.md): enabling the interfaces, wiring the pump and the
  screen.
- [Design](design.md): what the app does and how it's built: features, the
  estimate maths, the pump loop, data, API, code layout.
- [Ideas](ideas.md): next steps and ideas for later.
- [Deployment](../../deployment.md): setting up the Pi and shipping changes.

## Develop

Needs Go 1.26 and Node 22+. Run these from `apps/humidity/`:

```sh
make dev     # backend with a simulated sensor and pump (data in ./data),
             # plus the Vite dev server on http://localhost:5173
make test    # Go: gofmt, vet (Mac and Pi), tests; web: types, lint, tests
```

Run `npm --prefix web ci` once first (and again when `package-lock.json`
changes).

The simulated sensor reacts to the simulated pump, so setting a target above
the shelf's humidity shows the pump at work. Delete `./data` to start over.

To use the dev server from a phone on the tailnet, open
`http://<this-machine>.<tailnet>.ts.net:5173`.
