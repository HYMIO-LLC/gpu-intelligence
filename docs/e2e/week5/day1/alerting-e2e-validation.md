# Week 5 Day 1 — GPU Health Alerting E2E

Project: Hymio GPU Intelligence
Date: November 9, 2026 (PST)
Environment: Docker Desktop, kind, Kubernetes v1.37

## Objective

Introduce Prometheus alerting rules for GPU health,
telemetry availability, and Operator metrics scraping.

Verify alert state transitions and resolve the
intermittent GPU telemetry freshness issue.

## Alerting Rules

Three Prometheus alerts were implemented:

### GPUOverheating

Expression:
hymio_gpu_policy_healthy == 0

Duration: 30 seconds
Severity: warning

### GPUTelemetryUnavailable

Expression:
hymio_gpu_policy_telemetry_available == 0

Duration: 1 minute
Severity: warning

### GPUOperatorMetricsDown

Expression:
up{job="hymio-operator"} == 0

Duration: 1 minute
Severity: critical

## Configuration Validation

Prometheus version: v3.13.3

Validated using promtool:
- Alerting rules: PASS
- Prometheus configuration: PASS
- Rules found3

Updated the existing standalone Operator Metrics
Prometheus ConfigMap.

Reloaded Prometheus configuration using SIGHUP,
without intentionally restarting the Deployment.

Verified all rules through /api/v1/rules.

## GPU Overtemperature E2E

Test policy:
default/hymio-week4-overheat

Normal temperature threshold: 85 C
Overtemperature test threshold: 60 C

Observed Kubernetes condition:
GPUHealthy=False / TemperatureExceeded

Observed alert lifecycle:
- inactive
- pending
- firing

Result: PASS

## Telemetry Recovery Investigation

After restoring the threshold to 85 C, the
Controller initially reported IncompleteTelemetry.

The original Prometheus successfully scraped
the mock DCGM exporter.

Observed:
- up = 1
- GPU temperature = 68 C
- Temperature sample age approximately 1.67 seconds
- Default telemetry freshness window = 2 minutes

Investigation identified a timing issue in which
the Controller captured evaluation time before
fetching telemetry.

Prometheus query timestamps could be later than
the previously captured evaluation time, causing
fresh observations to be rejected as future data.

## Controller Fix

Modified gpuhealthpolicy_telemetry.go to obtain
the evaluation clock after telemetry fetching.

Added regression test:

TestTelemetryEvaluationUsesClockAfterFetch

Validated:
- Targeted regression test: PASS
- Controller test suite: PASS
- Complete make build: PASS
- Complete make test: PASS
- Go vet: PASS

## Kubernetes Deployment

Image:
hymio-operator:week5-day1

Platform:
linux/arm64

Successfully deployed to the hymio-dev kind cluster.

Observed:
- TelemetryAvailable=True
- GPUHealthy=True
- Ready=True
- Generation=8
- ObservedGeneration=8

## Prometheus Recovery Validation

For default/hymio-week4-overheat:

- hymio_gpu_policy_healthy = 1
- hymio_gpu_policy_telemetry_available = 1
- up{job="hymio-operator"} = 1

Final rule states:
- GPUOverheating: inactive / ok
- GPUTelemetryUnavailable: inactive / ok
- GPUOperatorMetricsDown: inactive / ok

## Repeated Reconciliation Checks

Triggered five annotation updates and checked
GPUHealthPolicy conditions after each update.

All five checks returned:
- TelemetryAvailable=True
- GPUHealthy=True

Result: PASS

## Limitations

- Tests use synthetic GPU telemetry
- No physical GPU hardware validation
- No long-duration soak testing
- No multi-replica Controller consistency test
- Alertmanager notification routing not yet configured
- Operator-down alert has not been fault-injected
- An absent scrape target is not covered by up == 0
- Repeated status checks do not prove every individual
  reconciliation completed before observation

## Result

GPU Health Alerting MVP validated in a local
Kubernetes environment.

Prometheus alert rules loaded and evaluated
successfully. GPU overheating triggered Pending
and Firing states.

Following the telemetry evaluation clock fix,
the GPU health metrics returned to Healthy and
the alert rules returned to Inactive.
