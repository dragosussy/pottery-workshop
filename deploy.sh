#!/bin/bash
# Builds one image for the Pi, pushes it and restarts that one service there.
# Usage: ./deploy.sh gateway|backup|humidity   (reads PI_USER, PI_HOST and PI_DIR from .env)
set -euo pipefail

case "${1:-}" in
  gateway)  SERVICE=gateway          IMAGE=qqrtos/atelier-gateway:latest  DOCKERFILE=gateway/Dockerfile ;;
  backup)   SERVICE=backup           IMAGE=qqrtos/atelier-backup:latest   DOCKERFILE=backup/Dockerfile ;;
  humidity) SERVICE=humiditycontrol  IMAGE=qqrtos/humiditycontrol:latest  DOCKERFILE=apps/humidity/Dockerfile ;;
  *) echo "Usage: ./deploy.sh gateway|backup|humidity" >&2; exit 2 ;;
esac

source .env

docker build --platform linux/arm64 -f "$DOCKERFILE" -t "$IMAGE" .
docker push "$IMAGE"
# One SSH session (one password prompt): copy the compose file over stdin,
# then pull and restart.
ssh "$PI_USER@$PI_HOST" "cd $PI_DIR && cat > docker-compose.yml && docker compose pull $SERVICE && docker compose up -d $SERVICE" < docker-compose.yml
echo "Deployed $1 to $PI_HOST."
