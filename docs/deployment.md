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
5. Open the gateway to the tailnet, and only the tailnet. This setting
   survives reboots:

   ```sh
   sudo tailscale serve --bg --http 80 8080
   ```

## Set up your computer once

1. Create `.env` in the repo root with `PI_USER`, `PI_HOST` and `PI_DIR`.
2. Log in to Docker Hub with `docker login`.
3. Optional: run `ssh-copy-id $PI_USER@$PI_HOST` so deploys don't ask for the
   Pi's password.

## Deploy

Run from the repo root:

```sh
make test
./deploy.sh humidity    # or: gateway, backup, glaze
```

`deploy.sh` builds that arm64 image, pushes it, copies `docker-compose.yml` to
the Pi and restarts only that service. The homepage is then at `http://atelier`
from any device on the tailnet, and each app at `http://atelier/<name>/`.

## On the Pi

`PI_DIR` holds `docker-compose.yml` and `data/`. Each app has a folder,
`data/<name>/`, with its database, photos and backups. Don't delete it.

## Backups

How they work is in [Architecture](architecture.md#data).

### Set up once

1. In Cloudflare, create a private R2 bucket called `atelier-backups`.
2. Create an R2 API token with **Object Read & Write** on that bucket only.
   Save it in a password manager.
3. Add object lifecycle rules: delete `snapshots/` after 90 days and
   `replaced/` after 90 days.
4. Add bucket locks: `snapshots/` for 89 days and `monthly/` indefinitely.
5. In healthchecks.io, create a check with a period of 1 day and a grace time
   of 6 hours, and note its ping URL.
6. On the Pi, copy [backup.env.example](../backup/backup.env.example) to
   `PI_DIR/backup.env`, fill it in and run `chmod 600 backup.env`.
7. Run `./deploy.sh backup` and check the result with `docker logs backup` on
   the Pi. It runs straight away, then once a day.

### Restore

On a replacement Pi, set it up as above and put `docker-compose.yml` and
`backup.env` in `PI_DIR`. Then, from `PI_DIR`, for each app:

```sh
docker compose down
alias rc='docker run --rm --env-file backup.env -v "$PWD/data:/data" rclone/rclone:1.73.1'
rc lsf r2:atelier-backups/snapshots/humidity       # choose a date
rc copy r2:atelier-backups/files/humidity /data/humidity
rc copyto r2:atelier-backups/snapshots/humidity/YYYY-MM-DD.db /data/humidity/humidity.db
docker compose up -d
```

Older versions of a replaced photo are in `replaced/<date>/<app>/`. Practise a
restore before real glaze data depends on it, and about twice a year after
that.
