#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
docker compose config >/dev/null
printf '\n=== Service status ===\n'
docker compose ps
printf '\n=== HTTP smoke tests ===\n'
./scripts/smoke.sh
printf '\n=== Observability acceptance checks ===\n'
./scripts/verify-observability.sh
