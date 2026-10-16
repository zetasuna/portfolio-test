#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [[ -e .env ]]; then
  echo ".env already exists; refusing to overwrite it." >&2
  exit 1
fi
command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }
POSTGRES_PASSWORD="$(openssl rand -hex 24)"
APP_DB_PASSWORD="$(openssl rand -hex 24)"
GRAFANA_PASSWORD="$(openssl rand -hex 24)"
PROJECT_DIR="$(pwd -P)"
DOCKER_GID="$(stat -c '%g' /var/run/docker.sock 2>/dev/null || echo 998)"
cp .env.example .env
python3 - "$POSTGRES_PASSWORD" "$APP_DB_PASSWORD" "$GRAFANA_PASSWORD" "$DOCKER_GID" "$PROJECT_DIR" <<'PY'
from pathlib import Path
import sys
p=Path('.env')
lines=p.read_text().splitlines()
out=[]
for line in lines:
    if line.startswith('POSTGRES_PASSWORD='):
        out.append('POSTGRES_PASSWORD='+sys.argv[1])
    elif line.startswith('APP_DB_PASSWORD='):
        out.append('APP_DB_PASSWORD='+sys.argv[2])
    elif line.startswith('GRAFANA_ADMIN_PASSWORD='):
        out.append('GRAFANA_ADMIN_PASSWORD='+sys.argv[3])
    elif line.startswith('DOCKER_GID='):
        out.append('DOCKER_GID='+sys.argv[4])
    elif line.startswith('PROJECT_DIR='):
        out.append('PROJECT_DIR='+sys.argv[5])
    else:
        out.append(line)
p.write_text('\n'.join(out)+'\n')
PY
chmod 600 .env
echo "Created .env with generated secrets and mode 0600."
echo "Review ALLOWED_HOSTS, PUBLIC_BASE_URL, COOKIE_SECURE and DOCKER_GID before exposing services."
echo "Next: docker compose config && docker compose up -d --build"
echo "Then create the first admin with: ./scripts/create-admin.sh"
