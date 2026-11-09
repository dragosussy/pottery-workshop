# Deployment

The image is built on your computer and pushed to Docker Hub. The Pi then
pulls it and restarts. Deploying is always done by the human developer, never
by an agent.

## Set up the Pi once

1. Install Docker.
2. Join the tailnet with the machine name `atelier`.
3. Fit the RTC battery, so the clock is right straight after boot.
4. Enable the interfaces each app needs. For humidity, see
   [Hardware](apps/humidity/hardware.md).

## Set up your computer once

1. Create `.env` in the repo root with `PI_USER`, `PI_HOST` and `PI_DIR`.
2. Log in to Docker Hub with `docker login`.
3. Optional: run `ssh-copy-id $PI_USER@$PI_HOST` so deploys don't ask for the
   Pi's password.

## Deploy

Run from the repo root:

```sh
make test
./deploy.sh
```

`deploy.sh` builds the arm64 image, pushes it, copies `docker-compose.yml` to
the Pi and restarts. The app is then at `http://atelier` from any device on
the tailnet.

Only humidity exists today, so `deploy.sh` takes no arguments. It will take
an app name once there is a second one.

## On the Pi

`PI_DIR` holds `docker-compose.yml` and `data/`. The `data/` folder holds the
database, photos and backups. Don't delete it.

Backups are daily, cover the database only, and stay on the Pi. See
[Architecture](architecture.md#data) for what's planned.
