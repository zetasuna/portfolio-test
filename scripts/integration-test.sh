#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
[[ -f .env ]] || { echo "Run ./scripts/setup.sh first" >&2; exit 1; }
# Load only the generated local .env; setup-generated secrets are URL-safe hex.
set -a
. ./.env
set +a
DB_NETWORK="${DB_NETWORK:-portfolio-crud_db_net}"
TEST_CONTAINER="portfolio-integration-test-$$"
TEST_DATABASE_URL="postgres://${APP_DB_USER}:${APP_DB_PASSWORD}@postgres:5432/${TEST_DB_NAME}?sslmode=disable"
cleanup() { docker rm -f "$TEST_CONTAINER" >/dev/null 2>&1 || true; }
trap cleanup EXIT
# Start on the default bridge for module downloads, then attach to the internal DB network.
# A container attached only to db_net cannot reach module proxies because that network is internal.
docker create --name "$TEST_CONTAINER" --network bridge \
  -v "$PWD:/src" -w /src \
  -e TEST_DATABASE_URL="$TEST_DATABASE_URL" -e MIGRATIONS_DIR=/src/migrations \
  golang:1.27.2-bookworm sh -ec 'go mod download && go test ./tests/integration -v' >/dev/null
docker network connect "$DB_NETWORK" "$TEST_CONTAINER"
docker start --attach "$TEST_CONTAINER"
