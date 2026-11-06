# Week 4 Day 3 — GPU Telemetry Reliability E2E

Project: Hymio GPU Intelligence
Milestone: Week 4, Day 3
Verification Date: November 5, 2026
Environment: Docker Desktop, kind, Kubernetes v1.37

## Objective

Validate the Week 4 Day 2 reliability improvements
in a real Kubernetes control-plane environment.

## Deployment

Operator Image: hymio-operator:week4-day2
Cluster: hymio-dev
Namespace: operator-system
Test Policy: default/hymio-week4-overheat

The updated Operator image was built for linux/arm64,
loaded into kind, and successfully deployed.

Operator Deployment: 1/1 available
Prometheus Deployment: 1/1 available
Mock DCGM Exporter: 1/1 available

## Scenario 1 — Healthy Telemetry

Policy threshold: 85 degrees Celsius
Synthetic GPU temperature: approximately 68 degrees Celsius

Observed:
- Ready=True / PolicyAccepted
- TelemetryAvailable=True / TelemetryAvailable
- Galthy=True / TemperatureWithinLimit

Result: PASS

Evidence: healthy-status.yaml

## Scenario 2 — Overtemperature

Changed the policy threshold to 60 degrees Celsius.

Observed:
- Ready=True / PolicyAccepted
- TelemetryAvailable=True / TelemetryAvailable
- GPUHealthy=False / TemperatureExceeded

Result: PASS

Evidence: overtemperature-status.yaml

## Scenario 3 — Prometheus Failure

Changed the Operator PROMETHEUS_URL temporarily to:
http://127.0.0.1:1

The Operator successfully rolled out with the
unreachable endpoint.

Observed:
- Ready=True / PolicyAccepted
- TelemetryAvailable=False / IncompleteNodeTelemetry
- GPUHealthy=Unknown / IncompleteNodeTelemetry

Result: PASS

Evidence: prometheus-failure-status.yaml

## Scenario 4 — Recovery

Restored the Prometheus service endpoint and
changed the test policy threshold back to 85 degrees.

Observed:
- Ready=True / PolicyAccepted
- TelemetryAvailable=True / TelemetryAvailable
- GPUHealthy=True / TemperatureWithinLimit

The Operator rollout completed succly.

Result: PASS

Evidence: recovery-status.yaml

## Final Environment State

Operator: 1/1 available
Prometheus: 1/1 available
Mock exporter: 1/1 available

The original Prometheus endpoint was restored.

The test policy threshold was restored to 85 degrees.

## Validation Scope

This E2E suite validates:
- GPU temperature health evaluation
- Independent Kubernetes Condition Reasons
- Unavailable Prometheus telemetry handling
- Recovery following configuration restoration

Unit tests separately validate conflict retry behavior,
concurrent policy updates, and FetchNode timeouts.

Those internal behaviors were not directly fault-injected
or measured in this kind E2E suite.

## Limitations

- Synthetic GPU telemetry, not physical GPU hardware
- Local kind cluster, not a production environment
- No automatic node remediation
- No high-load or long-duration stability testing

## Result

Four E2E scenarios passed.

The Week 4 Day 2 Operator was successfully deployed
and validated in Kubernetes on November 5, 2026.
