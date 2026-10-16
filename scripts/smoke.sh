#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
CURL="${CURL:-curl}"
for path in /healthz /readyz / /projects; do
  code="$($CURL --silent --show-error --output /dev/null --max-time 5 --write-out '%{http_code}' "$BASE_URL$path")"
  case "$code" in 200) printf 'PASS %s -> %s\n' "$path" "$code";; *) printf 'FAIL %s -> %s\n' "$path" "$code" >&2; exit 1;; esac
done
instances="$(for _ in $(seq 1 30); do $CURL --silent --show-error --max-time 5 -H 'Connection: close' -D - -o /dev/null "$BASE_URL/healthz" | awk 'BEGIN{IGNORECASE=1}/^X-App-Instance:/{gsub("\\r", ""); print $2}'; done | sort -u)"
for instance in app1 app2 app3; do
  if ! grep -qx "$instance" <<< "$instances"; then
    echo "FAIL load balancing did not observe $instance; seen: ${instances:-none}" >&2
    exit 1
  fi
done
echo "PASS load balancing observed app1, app2, app3"
