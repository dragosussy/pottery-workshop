# Architecture

`atelier` is one Raspberry Pi 5, reached over Tailscale, running several small
independent apps. A homepage is the way into all of them.

## Where things stand

| Part | Status |
|---|---|
| One repo with apps in `apps/` | Done |
| humidity app | Running as a single container on port 80 |
| Gateway and homepage | Planned |
| Tailnet-only access (today port 80 is also open on the local network) | Planned |
| Each app's data in `data/<app>/` on the Pi (today humidity uses `data/`) | Planned, with the gateway |
| Off-device backups | Planned |
| Glaze notebook | Planned |
| `shared/` code and npm workspaces | When the second app needs them |

## The target

```
phone or laptop ── Tailscale ──> Pi "atelier"
                                    │
                           Caddy gateway + homepage
                            │            │          │
                      /humidity/      /glaze/     /next-app/
                            │            │
                  humidity container   glaze container
                  (I²C, GPIO, SPI)     (no hardware)
                            │            │
                    data/humidity/     data/glaze/
```

## Why it is built this way

| Decision | Reason |
|---|---|
| One repo | One developer. Shared styling and deploys need no package publishing. |
| One container per app | Deploying or crashing one app can't stop the pump loop, and only humidity gets hardware access. An app can use any language or be off-the-shelf software. |
| Routing by path (`http://atelier/glaze/`) | Tailscale names don't give a machine subdomains without buying a domain. One address also lets the homepage call each app directly from the browser. |
| One SQLite database per app | Apps stay independent and can be backed up and removed separately. |
| One Go module at the root | One set of dependency versions. Go's `internal/` directories still keep each app's code private to it. |

## Repo layout

```
atelier/
├── apps/
│   └── humidity/         the app: cmd, internal, web, Dockerfile, Makefile
├── docs/                 all documentation
├── AGENTS.md             principles for developing here
├── docker-compose.yml    everything that runs on the Pi
├── deploy.sh             builds, pushes and restarts
├── Makefile              checks for the whole repo
└── go.mod
```

Planned additions:

- `gateway/`: the Caddyfile, the homepage and the list of apps.
- `apps/glaze/`: the glaze notebook.
- `shared/ui/` and `shared/kit/`: common Vue and Go code, added only when a
  second app needs it. Glaze will probably need photo handling, backups, JSON
  helpers and the design tokens.

## Rules every app follows

An app in this repo:

1. Listens on port 8080 and doesn't know its URL prefix on the server. The
   gateway strips it.
2. Builds its web app for `/<name>/`, including API calls, routes and the
   manifest.
3. Keeps all of its state in `/data`.
4. Writes a daily database snapshot to `/data/backups`.
5. Shuts down cleanly on SIGTERM.
6. Offers `GET /api/summary` for its homepage tile.
7. Caps its logs in `docker-compose.yml`, to protect the SD card.

humidity already follows 1, 3, 4, 5 and 7. It needs 2 and 6 when the gateway
is added.

## Data

Apps never read each other's databases. If one app needs another's data, it
calls that app's HTTP API.

All data is on the Pi's SD card until off-device backups exist. Glaze recipes
and test results can't be recreated, so backups have to be running before the
notebook holds real work.

## Access

The tailnet is the security boundary, and apps have no logins. Port 80 is
currently also reachable from the Pi's local network, including the pump
controls. The plan is for Caddy to listen only on localhost, with Tailscale
Serve as the only way in. That also makes HTTPS available. This hasn't been
tried on the Pi yet.

## Adding an app

Once the gateway exists:

1. Create `apps/<name>/`, following the rules above.
2. Add a service to `docker-compose.yml`.
3. Add a route to the Caddyfile and an entry to the homepage's app list.
4. Add `docs/apps/<name>/` and link it from [docs/README.md](README.md).
5. Add the app to `test` in the root `Makefile`.
