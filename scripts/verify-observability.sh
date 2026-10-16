#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
[[ -f .env ]] || { echo "Missing .env" >&2; exit 1; }
# Host CLI is only exposed for localhost services. The one-shot curl container attaches to
# the private Compose network; it is not a persistent service/container in the architecture.
source .env
NETWORK="portfolio-crud_obs_net"
CURL_IMAGE="curlimages/curl:8.12.1"
run_curl() { docker run --rm --network "$NETWORK" "$CURL_IMAGE" --fail --silent --show-error "$@"; }

echo '[1/5] Generating traffic and a trace'
curl --fail --silent --show-error "http://127.0.0.1:${APP_HOST_PORT:-8080}/healthz" >/dev/null
curl --fail --silent --show-error "http://127.0.0.1:${APP_HOST_PORT:-8080}/projects" >/dev/null
sleep 6

echo '[2/5] Checking Prometheus targets'
TARGETS="$(run_curl 'http://prometheus:9090/api/v1/query?query=up%7Bjob%3D%22portfolio-app%22%7D')"
python3 - "$TARGETS" <<'PY'
import json,sys
obj=json.loads(sys.argv[1]); result=obj.get('data',{}).get('result',[])
instances={row.get('metric',{}).get('app_instance'):row.get('value',[None,None])[1] for row in result}
for name in ('app1','app2','app3'):
    if instances.get(name)!='1': raise SystemExit(f'FAIL {name} scrape state={instances.get(name)}')
print('PASS Prometheus scrapes all three app instances')
PY

echo '[3/5] Checking provisioned Grafana datasources'
DATA="$(curl --fail --silent --show-error -u "${GRAFANA_ADMIN_USER:-admin}:${GRAFANA_ADMIN_PASSWORD}" "http://127.0.0.1:${GRAFANA_HOST_PORT:-3000}/api/datasources")"
python3 - "$DATA" <<'PY'
import json,sys
names={x.get('name') for x in json.loads(sys.argv[1])}
required={'Prometheus','Loki','Tempo'}
missing=required-names
if missing: raise SystemExit(f'FAIL missing Grafana data sources: {sorted(missing)}')
print('PASS Grafana has Prometheus, Loki and Tempo datasources')
PY

echo '[4/5] Checking logs in Loki'
LOG_START="$(date -u -d '30 minutes ago' +%s%N)"
LOG_END="$(date -u +%s%N)"
LOGS="$(run_curl --get --data-urlencode 'query={service="portfolio-app"}' --data-urlencode 'limit=5' --data-urlencode 'direction=backward' --data-urlencode "start=$LOG_START" --data-urlencode "end=$LOG_END" 'http://loki:3100/loki/api/v1/query_range')"
python3 - "$LOGS" <<'PY'
import json,sys
obj=json.loads(sys.argv[1]); streams=obj.get('data',{}).get('result',[])
if not streams: raise SystemExit('FAIL Loki returned no portfolio-app log streams; generate traffic and inspect Alloy logs')
print(f'PASS Loki returned {len(streams)} application log stream(s)')
PY

echo '[5/5] Checking traces in Tempo'
TRACE_START="$(date -u -d '30 minutes ago' +%s)"
TRACE_END="$(date -u +%s)"
TRACES="$(run_curl --get --data-urlencode "start=$TRACE_START" --data-urlencode "end=$TRACE_END" --data-urlencode 'limit=20' 'http://tempo:3200/api/search')"
python3 - "$TRACES" <<'PY'
import json,sys
obj=json.loads(sys.argv[1]); traces=obj.get('traces', obj.get('data',{}).get('traces',[]))
if not traces: raise SystemExit('FAIL Tempo search returned no traces; inspect app and collector logs, and wait for export')
print(f'PASS Tempo returned {len(traces)} trace(s)')
PY
