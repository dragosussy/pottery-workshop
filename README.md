# atelier

Small apps for a pottery studio, running on a Raspberry Pi 5 and used from a
phone over Tailscale.

| App | What it does | Status |
|---|---|---|
| [humidity](docs/apps/humidity/README.md) | Holds the drying shelf's humidity and estimates when pieces will be leather hard | In use |
| Glaze notebook | Logs glaze tests, recipes, results and recipe adjustments | Planned |

## Documentation

Everything is in [docs/](docs/README.md). A good place to start is
[how the Pi and this repo are organised](docs/architecture.md).

AI agents working here follow [AGENTS.md](AGENTS.md).

## Common commands

```sh
make dev                     # everything on http://localhost:8000, simulated hardware
make test                    # every check for every app
make -C apps/humidity dev    # one app on its own
./deploy.sh humidity         # build, push and restart on the Pi (or: gateway)
```

Deploying is covered in [docs/deployment.md](docs/deployment.md).
