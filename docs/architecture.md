# Architecture

`atelier` is one Raspberry Pi 5, reached over Tailscale, running several small
independent apps. A homepage is the way into all of them.

## Where things stand

| Part | Status |
|---|---|
| One repo with apps in `apps/` | Done |
| humidity app | Running at `/humidity/` |
| Gateway and homepage | Done |
| Tailnet-only access | Done |
| Each app's data in `data/<app>/` on the Pi | Done |
| Off-device backups | Built, waiting for its Cloudflare setup on the Pi |
| Glaze notebook | Planned |
| `shared/` code | Planned, with glaze |

## How it fits together

Glaze is planned; the rest is running.

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
├── gateway/              the Caddyfile, the homepage and its list of apps
├── backup/               the daily copy of every app's data to Cloudflare R2
├── docs/                 all documentation
├── AGENTS.md             principles for developing here
├── docker-compose.yml    everything that runs on the Pi
├── deploy.sh             builds, pushes and restarts
├── Makefile              checks for the whole repo
└── go.mod
```

Planned additions:

- `apps/glaze/`: the glaze notebook.
- `shared/`: see [Shared code](#shared-code).

## Rules every app follows

An app in this repo:

1. Listens on port 8080 and doesn't know its URL prefix on the server. The
   gateway strips it.
2. Builds its web app for `/<name>/`, including API calls, routes and the
   manifest.
3. Keeps all of its state in `/data`, with live databases at the top level.
   Everything else there is backed up off the Pi, so caches go elsewhere.
4. Writes a daily database snapshot to `/data/backups`, named `YYYY-MM-DD.db`.
5. Shuts down cleanly on SIGTERM.
6. Offers `GET /api/summary` for its homepage tile, answering
   `{"text": "one short line"}`.
7. Caps its logs in `docker-compose.yml`, to protect the SD card.
8. Uses the [shared code](#shared-code) instead of keeping its own version.

## Shared code

Apps run independently, but they shouldn't each solve the same problem their
own way or look like different products. Both folders are planned and arrive
with glaze, the first app to need them.

| Folder | What's in it | Why |
|---|---|---|
| `shared/ui/` | Design tokens, base styles and any Vue component used by more than one app | Every app and the homepage look the same. It starts from humidity's styles. |
| `shared/kit/` | Go code: photo processing, database snapshots, JSON error handling | Every app stores photos and reports errors the same way |

- Code moves here when a second app needs it, not before and not as a copy.
- Moving code here must leave the app it came from behaving the same.
- Anything specific to one app becomes an option or stays in that app. The
  dryness slider's "trim band" label is an option, because glaze's thickness
  slider is the same control without it.

## Data

Apps never read each other's databases. If one app needs another's data, it
calls that app's HTTP API.

Once a day the `backup` service copies each app's data to a private Cloudflare
R2 bucket. It finds everything in `data/<app>/` by itself, so a new app needs
no backup setup. Glaze recipes and test results can't be recreated, so backups
have to be running before the notebook holds real work.

| In the bucket | What | Kept |
|---|---|---|
| `snapshots/<app>/` | Each day's database snapshot | 90 days |
| `monthly/<app>/` | The snapshot from the 1st of each month | Forever |
| `files/<app>/` | Photos and any other files | Forever |
| `replaced/<date>/<app>/` | Earlier versions of files that were replaced | 90 days |

| Decision | Reason |
|---|---|
| Copies only, never mirrors | A mistake on the Pi can't delete the good copy. |
| Only snapshots of databases | A live SQLite file copied mid-write can be corrupt. |
| Retention and locks set in the bucket | The Pi can't shorten its own history. |
| No encryption | Recipes and pot photos aren't sensitive, and a lost password would make everything unrecoverable. |
| A healthchecks.io report after each success | A missing report raises the alarm, even if the whole Pi has died. |

All of this should fit R2's free 10 GB for about three and a half years. When
it gets close, shorten `snapshots/` and `replaced/` to 30 days in the bucket
rules.

## Access

The tailnet is the security boundary, and apps have no logins. The gateway
listens only on the Pi's localhost, and Tailscale Serve is the only way in, so
the local network can't reach the apps or the pump controls.

## Adding an app

1. Create `apps/<name>/`, following the rules above.
2. Add a service to `docker-compose.yml`, with no `ports` of its own.
3. Add a route to `gateway/Caddyfile` and `gateway/Caddyfile.dev`, and an entry
   to the homepage's app list.
4. Add the app's image to `deploy.sh`.
5. Add `docs/apps/<name>/` and link it from [docs/README.md](README.md).
6. Add the app to `dev` and `test` in the root `Makefile`, with dev ports no
   other app uses.
