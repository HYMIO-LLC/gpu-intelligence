# Week 4 Day 1 — GPU Health Failure and Recovery E2E

Date: November 3, 2026
Project: Hymio GPU Intelligence
Environment: macOS ARM64, Docker, kind, Kubernetes v1.37

## Objective

Validate GPUHealthPolicy behavior during simulated
overtemperature, Prometheus query failures, and recovery.

All tests use synthetic GPU telemetry from the mock
DCGM exporter. No physical NVIDIA GPU was involved.

## Environment

Cluster: hymio-dev
Operator Namespace: operator-system
Operator Deployment: operator-controller-manager
Operator Image: hymio-operator:week3-day4

Monitoring Namespace: hymio-monitoring
Prometheus Service: hymio-prometheus
Mock Exporter: mock-dcgm-exporter

Matched Node: hymio-dev-control-plane
Test Policy: default/hymio-week4-overheat

## Scenario 1 — Overtemperature

Created a dedicated GPUHealthPolicy with a temperature
threshold of 60 degrees Celsius.

The mock GPU temperature was 68 degrees Cus.

Observed Kubernetes conditions:

- Ready=True / PolicyAccepted
- TelemetryAvailable=True / TemperatureExceeded
- GPUHealthy=False / TemperatureExceeded

The Kubernetes condition wait completed successfully.

Result: PASS

Evidence:
overtemperature-policy.yaml
overtemperature-status.yaml

## Scenario 2 — Prometheus Query Failure

Temporarily changed the Operator's PROMETHEUS_URL to:

http://127.0.0.1:1

The configuration deliberately caused telemetry queries
from the Operator Pod to fail.

The Operator Deployment rolled out successfully.

Observed Kubernetes conditions:

- Ready=True / PolicyAccepted
- TelemetryAvailable=False / IncompleteNodeTelemetry
- GPUHealthy=Unknown / IncompleteNodeTelemetry

The Kubernetes condition wait completed successfully.

Result: PASS

Evidence:
prometheus-failure-status.yaml

## Scenario 3 — Restore Prometheus Connectivity

Restored the original in-cluster Prometheus address:

http://hymio-prometheus.hymio-monitoring.svc.cluster.local:9090

The Operator Deployment ro out successfully.

The controller resumed collecting telemetry.

TelemetryAvailable=True was explicitly verified.

The temperature threshold was still 60 degrees Celsius
during this stage. A separate GPUHealthy status capture
was not taken before changing the threshold.

Result: Connectivity recovery verified.
GPUHealthy at this intermediate stage not independently
recorded.

## Scenario 4 — Healthy State Recovery

Applied a recovery policy with the temperature threshold
restored to 85 degrees Celsius.

The mock temperature remained 68 degrees Celsius.

Observed Kubernetes conditions:

- Ready=True / PolicyAccepted
- TelemetryAvailable=True / TemperatureWithinLimit
- GPUHealthy=True / TemperatureWithinLimit

The Kubernetes condition wait completed successfully.

Confirmed that the original PROMETHEUS_URL was restored.

Result: PASS

Evidence:
recovery-policy.yaml
recovery-status.yaml

## Safety and Isolation

- Used a dedicated test policy rather than changing the
  existing hymio-e2e policy.
- Did not mify physical GPUs or perform node remediation.
- Restored the production-like test Prometheus configuration.
- The temporary Prometheus URL failure affected telemetry
  queries for policies managed by this local Operator.
- Preserved existing historical E2E evidence and local
  temporary smoke-test files.

## Verification Summary

Overtemperature detection: PASS
Prometheus failure detection: PASS
Prometheus connectivity recovery: PASS
Healthy-state recovery: PASS

## Known Limitations

- No physical GPU hardware was used.
- Tests were conducted in a local kind cluster.
- Telemetry completeness relies on the current sample model.
- Per-condition Reason improvements are deferred to Day 2.
- Prometheus timeout and retry optimizations are deferred
  to Day 2.
- No automated remediation or node isolation was performed.

## Conclusion

Successfully validated GPUHealthPolicy overtemperature
detection, telemetry failure reporting, Prometheus
connectivity restoration, and healthy-state recovery
using real Kubernetes status reconciliation.
