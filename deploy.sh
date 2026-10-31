#!/bin/bash
set -e

ENV=$1

if [ -z "$ENV" ]; then
  echo "Usage: ./deploy.sh <dev|prod>"
  exit 1
fi

ENV_FILE=".env.$ENV"

if [ ! -f "$ENV_FILE" ]; then
  echo "Environment file $ENV_FILE not found!"
  exit 1
fi

source $ENV_FILE

IMAGE="qqrtos/humiditycontrol:latest"
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILT_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)

echo "Building image $VERSION ($COMMIT) for $ENV..."
docker build --platform linux/arm64 \
  --build-arg VERSION="$VERSION" \
  --build-arg COMMIT="$COMMIT" \
  --build-arg BUILT_AT="$BUILT_AT" \
  -t $IMAGE .

echo "Pushing image..."
docker push $IMAGE

echo "Uploading compose file..."
scp docker-compose.yml ${PI_USER}@${PI_HOST}:${PI_DIR}/docker-compose.yml

echo "Deploying to $ENV..."
ssh ${PI_USER}@${PI_HOST} "
  cd ${PI_DIR} &&
  docker compose pull &&
  docker compose up -d
"

echo "Deploy to $ENV complete."
