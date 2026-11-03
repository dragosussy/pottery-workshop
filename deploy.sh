#!/bin/bash
# Builds the image for the Pi, pushes it and restarts the app there.
# Usage: ./deploy.sh dev   (reads PI_USER, PI_HOST and PI_DIR from .env.dev)
set -euo pipefail

ENV=${1:?usage: ./deploy.sh <dev|prod>}
source ".env.$ENV"
IMAGE=qqrtos/humiditycontrol:latest

docker build --platform linux/arm64 -t "$IMAGE" .
docker push "$IMAGE"
# One SSH session (one password prompt): copy the compose file over stdin,
# then pull and restart.
ssh "$PI_USER@$PI_HOST" "cd $PI_DIR && cat > docker-compose.yml && docker compose pull && docker compose up -d" < docker-compose.yml
echo "Deployed to $ENV."
