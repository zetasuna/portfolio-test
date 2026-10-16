#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
[[ -f .env ]] || { echo "Missing .env. Run ./scripts/setup.sh first." >&2; exit 1; }
docker compose config >/dev/null
docker compose up -d --build
docker compose ps
