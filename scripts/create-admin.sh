#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
[[ -f .env ]] || { echo "Run ./scripts/setup.sh first" >&2; exit 1; }
read -r -p 'Admin username (3-80 characters): ' ADMIN_USERNAME
read -r -s -p 'Admin password (at least 12 characters): ' ADMIN_PASSWORD
echo
read -r -s -p 'Repeat password: ' ADMIN_PASSWORD_CONFIRM
echo
[[ "$ADMIN_PASSWORD" == "$ADMIN_PASSWORD_CONFIRM" ]] || { echo 'Passwords did not match' >&2; exit 1; }
[[ ${#ADMIN_PASSWORD} -ge 12 ]] || { echo 'Password must be at least 12 characters' >&2; exit 1; }
export ADMIN_USERNAME ADMIN_PASSWORD
docker compose run --rm --no-deps -e ADMIN_USERNAME -e ADMIN_PASSWORD app1 create-admin
unset ADMIN_PASSWORD ADMIN_PASSWORD_CONFIRM
