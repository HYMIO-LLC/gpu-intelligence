#!/usr/bin/env bash
set -euo pipefail

CTX=kind-hymio-dev
NS=hymio-monitoring
NAME=hymio-operator-metrics-prometheus-config
DEPLOY=hymio-operator-metrics-prometheus

BASE=docs/e2e/week5/day2/prometheus.yml
TEST=docs/e2e/week5/day3/prometheus-nodata-e2e.yml
RULES=docs/e2e/week5/day3/gpu-health-alerts.yml

START_UTC=$(date -u +%Y-%m-%dT%H:%M:%SZ)
PF_LOG=$(mktemp)
PF_PID=""
CHANGED=0

apply_config() {
  local file="$1"

  kubectl --context "$CTX" create configmap "$NAME" \
    -n "$NS" \
    --from-file="prometheus.yml=$file" \
    --from-file="gpu-health-alerts.yml=$RULES" \
    --dry-run=client -o yaml \
    | kubectl --context "$CTX" apply -f -
}

wait_for_config() {
  local expected="$1"

  for i in $(seq 1 45); do
    if kubectl --context "$CTX" exec \
      -n "$NS" deployment/"$DEPLOY" \
      -c prometheus -- \
      grep -Fxq "  - $expected" /etc/prometheus/prometheus.yml \
      2>/dev/null; then
      return 0
    fi
    sleep 2
  done

  echo "ERROR: ConfigMap projection timed out."
  return 1
}

reload_prometheus() {
  kubectl --context "$CTX" exec \
    -n "$NS" deployment/"$DEPLOY" \
    -c prometheus -- \
    /bin/sh -c 'kill -HUP 1'
}

runtime_job() {
  curl -fsS \
    http://127.0.0.1:19090/api/v1/status/config \
    | python3 -c '
import json
import re
import sys

data = json.load(sys.stdin)
jobs = re.findall(
    r"^\s*-\s*job_name:\s*(\S+)\s*$",
    data["data"]["yaml"],
    re.MULTILINE,
)
print(jobs[0] if len(jobs) == 1 else "unexpected")
'
}

wait_runtime_job() {
  local expected="$1"
  local actual=""

  for i in $(seq 1 30); do
    actual=$(runtime_job) || return 1
    echo "Runtime job: $actual"

    if [ "$actual" = "$expected" ]; then
      return 0
    fi

    sleep 2
  done

  echo "ERROR: Runtime job did not become $expected"
  return 1
}

restore() {
  if [ "$CHANGED" -eq 1 ]; then
    echo "=== SAFETY: Restore Original Scrape Job ==="

    if apply_config "$BASE" &&
       wait_for_config 'job_name: hymio-operator' &&
       reload_prometheus &&
       wait_runtime_job 'hymio-operator'; then
      echo "Original Prometheus configuration restored."
      CHANGED=0
    else
      echo "ERROR: Automatic restoration failed."
      echo "Manual recovery is required."
      return 1
    fi
  fi
}

cleanup() {
  local status=$?
  trap - EXIT

  if ! restore; then
    status=1
  fi

  if [ -n "$PF_PID" ]; then
    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true
  fi

  rm -f "$PF_LOG"
  exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

query() {
  curl -fsSG \
    --data-urlencode "query=$1" \
    http://127.0.0.1:19090/api/v1/query \
    | python3 -c '
import json, sys
d=json.load(sys.stdin)
assert d["status"]=="success"
items=d["data"]["result"]
print(",".join(x["value"][1] for x in items) if items else "absent")
'
}

rule_state() {
  curl -fsSG \
    --data-urlencode 'type=alert' \
    http://127.0.0.1:19090/api/v1/rules \
    | python3 -c '
import json,sys
data=json.load(sys.stdin)
rules=[
    r for g in data["data"]["groups"]
    for r in g["rules"]
    if r["name"]=="GPUOperatorMetricsDown"
]
assert len(rules)==1
print(rules[0]["state"])
'
}

receiver_has() {
  local expected="$1"

  kubectl --context "$CTX" logs \
    deployment/hymio-alert-receiver \
    -n "$NS" --since-time="$START_UTC" \
    | python3 -c '
import json,sys

wanted=sys.argv[1]

for line in sys.stdin:
    try:
        event=json.loads(line)
    except json.JSONDecodeError:
        continue

    for alert in event.get("alerts",[]):
        if (alert.get("alertname")=="GPUOperatorMetricsDown"
            and alert.get("status")==wanted):
            print("Webhook", wanted, "PASS")
            sys.exit(0)

sys.exit(1)
' "$expected"
}

echo "=== Verify Original Config ==="

CURRENT=$(kubectl --context "$CTX" get configmap \
  "$NAME" -n "$NS" \
  -o jsonpath='{.data.prometheus\.yml}')

EXPECTED=$(cat "$BASE")

if [ "$CURRENT" != "$EXPECTED" ]; then
  echo "ERROR: Live Prometheus config differs from baseline."
  echo "Aborting without changing Kubernetes."
  exit 1
fi

echo "=== Start Prometheus Port Forward ==="

kubectl --context "$CTX" port-forward \
  -n "$NS" deployment/"$DEPLOY" \
  19090:9090 > "$PF_LOG" 2>&1 &

PF_PID=$!
sleep 3

echo "=== Baseline ==="

UP=$(query 'up{job="hymio-operator"}')
echo "Original up=$UP"

if [ "$UP" != "1" ]; then
  echo "ERROR: Expected healthy baseline."
  exit 1
fi

echo "=== Inject No Data ==="

CHANGED=1
apply_config "$TEST"
wait_for_config 'job_name: hymio-operator-nodata-e2e'
reload_prometheus
wait_runtime_job 'hymio-operator-nodata-e2e'

echo "=== Wait for Absent Series and Firing ==="

FIRED=0
ABSENT=0

for i in $(seq 1 150); do
  UP=$(query 'up{job="hymio-operator"}')
  MISSING=$(query 'absent(up{job="hymio-operator"})')
  STATE=$(rule_state)

  echo "Observation $i: original_up=$UP absent=$MISSING alert=$STATE"

  if [ "$UP" = "absent" ] && [ "$MISSING" = "1" ]; then
    ABSENT=1
  fi

  if [ "$ABSENT" -eq 1 ] && [ "$STATE" = "firing" ]; then
    FIRED=1
    break
  fi

  sleep 5
done

if [ "$FIRED" -ne 1 ]; then
  echo "FAIL: No Data did not produce Critical Firing."
  exit 1
fi

echo "=== Wait for Firing Webhook ==="

DELIVERED=0

for i in $(seq 1 24); do
  if receiver_has firing; then
    DELIVERED=1
    break
  fi
  sleep 5
done

if [ "$DELIVERED" -ne 1 ]; then
  echo "FAIL: No Data Firing webhook not received."
  exit 1
fi

echo "=== Restore Normal Scrape Configuration ==="

restore

echo "=== Wait for Scrape Recovery ==="

RECOVERED=0

for i in $(seq 1 30); do
  UP=$(query 'up{job="hymio-operator"}')
  echo "Recovery $i: up=$UP"

  if [ "$UP" = "1" ]; then
    RECOVERED=1
    break
  fi

  sleep 5
done

if [ "$RECOVERED" -ne 1 ]; then
  echo "FAIL: Normal scrape did not recover."
  exit 1
fi

echo "=== Wait for Resolved Webhook ==="

RESOLVED=0

for i in $(seq 1 30); do
  if receiver_has resolved; then
    RESOLVED=1
    break
  fi
  sleep 5
done

if [ "$RESOLVED" -ne 1 ]; then
  echo "FAIL: Resolved webhook not received."
  exit 1
fi

echo "=== Final Rule State ==="
echo "GPUOperatorMetricsDown=$(rule_state)"

echo "=== RESULT ==="
echo "No Data detected: PASS"
echo "Critical Firing: PASS"
echo "Webhook Firing: PASS"
echo "Scrape Recovery: PASS"
echo "Webhook Resolved: PASS"
echo "RESULT: END-TO-END PASS"
