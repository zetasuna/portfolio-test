#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
[[ -f .env ]] || {
  echo "Missing .env. Run ./scripts/setup.sh first." >&2
  exit 1
}

PROJECT_DIR="$(pwd -P)"

docker compose \
  --env-file ${PROJECT_DIR}/.env \
  -f ${PROJECT_DIR}/deploy/jenkins/compose.yaml \
  up -d --build
docker compose ps
