#!/bin/bash
# Builds the image for the Pi, pushes it and restarts the app there.
# Usage: ./deploy.sh   (reads PI_USER, PI_HOST and PI_DIR from .env)
set -euo pipefail

source .env
IMAGE=qqrtos/humiditycontrol:latest

docker build --platform linux/arm64 -f apps/humidity/Dockerfile -t "$IMAGE" .
docker push "$IMAGE"
# One SSH session (one password prompt): copy the compose file over stdin,
# then pull and restart.
ssh "$PI_USER@$PI_HOST" "cd $PI_DIR && cat > docker-compose.yml && docker compose pull && docker compose up -d" < docker-compose.yml
echo "Deployed to $PI_HOST."
