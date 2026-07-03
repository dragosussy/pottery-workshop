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

echo "Building image for $ENV..."
docker build -t $IMAGE .

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
