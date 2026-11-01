#!/bin/bash
# Builds the image for the Pi, pushes it and restarts the app there.
# Usage: ./deploy.sh dev   (reads PI_USER, PI_HOST and PI_DIR from .env.dev)
set -euo pipefail

ENV=${1:?usage: ./deploy.sh <dev|prod>}
source ".env.$ENV"
IMAGE=qqrtos/humiditycontrol:latest

docker build --platform linux/arm64 -t "$IMAGE" .
docker push "$IMAGE"
scp docker-compose.yml "$PI_USER@$PI_HOST:$PI_DIR/docker-compose.yml"
ssh "$PI_USER@$PI_HOST" "cd $PI_DIR && docker compose pull && docker compose up -d"
echo "Deployed to $ENV."
