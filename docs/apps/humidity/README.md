# humidity

A drying-shelf controller for pottery. A Raspberry Pi 5 next to a
plastic-wrapped shelf:

- reads the shelf's humidity and temperature (SHT31/SHT4x over I²C);
- runs a misting pump to hold a humidity target;
- shows the humidity and temperature on a small screen next to it;
- tracks the pieces drying on the shelf and your dryness checks, and
  estimates when each will be leather hard, learning from your trims;
- serves a phone web app over Tailscale at `http://atelier/humidity/`.

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

Needs Go 1.26 and Node 22+. `make dev` from the repo root runs this app behind
the gateway, as on the Pi, on `http://localhost:8000` (it also needs Docker).
To work on it alone, run these from `apps/humidity/`:

```sh
make dev     # backend with a simulated sensor and pump (data in ./data),
             # plus the Vite dev server on http://localhost:5173/humidity/
make test    # Go: gofmt, vet (Mac and Pi), tests; web: types, lint, tests
```

Both install the web app's dependencies when they need to.

The simulated sensor reacts to the simulated pump, so setting a target above
the shelf's humidity shows the pump at work. Delete `./data` to start over.

To use it from a phone on the tailnet, open
`http://<this-machine>.<tailnet>.ts.net:8000` after a root `make dev`, or
`:5173/humidity/` on the same name when running the app alone.
