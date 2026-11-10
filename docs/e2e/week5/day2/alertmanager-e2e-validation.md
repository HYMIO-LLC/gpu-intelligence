# Week 5 Day 2 — Alertmanager Integration E2E

**Project:** Hymio GPU Intelligence
**Date:** November 10, 2026 (PST)
**Environment:** Docker Desktop / kind / Kubernetes v1.37.0
**Baseline Commit:** 526b851

## Objective

Extend the GPU Health Alerting MVP with Alertmanager
and validate notification routing and recovery.

Target flow:

GPUHealthPolicy -> Operator -> Prometheus
-> Alertmanager -> Webhook Receiver

## Implementation

### 1. Alertmanager

Deployed Alertmanager v0.34.2 into the
hymio-monitoring namespace.

Resources:
- hymio-alertmanager ConfigMap
- hymio-alertmanager Deployment
- hymio-alertmanager Service

Configured:
- Grouping by alertname, namespace, and policy
- group_wait: 5s
- group_interval: 15s
- repeat_interval: 1h
- Webhook receiver with send_resolved: true

Validation:
- amtool check-config: PASS
- Deployment rollout: PASS
- Readiness endpoint: OK
- Runtime routing configuratio PASS

### 2. Prometheus Integration

Updated the dedicated Operator Metrics Prometheus
configuration with the Alertmanager target:

hymio-alertmanager.hymio-monitoring.svc.cluster.local:9093

Validated configuration using promtool v3.13.3.

Results:
- Prometheus configuration: PASS
- Alerting rules: 3 validated
- ConfigMap update: PASS
- SIGHUP configuration reload: PASS
- Alertmanager discovery: active
- Operator scrape: up=1

Existing GPU telemetry Prometheus was not modified.

### 3. Webhook Receiver

Implemented a Python HTTP receiver that records
Alertmanager notifications as structured JSON logs.

Endpoints:
- GET /healthz
- POST /alerts

Deployed into Kubernetes using a ConfigMap,
Deployment, and Service.

Validation:
- Python syntax check: PASS
- Deployment rollout: PASS
- Readiness: HTTP 200
- Direct POST smoke test: HTTP 200
- Structured alert logging: PASS

### 4. Alertmanager Routing Test

Injected HymioWebhookRoutingTest directly into
the Alertmanager API.

Verified Webhook Receiver received:
- Firing notification
- Resolved notification

This test validated Alertmanager to Receiver
delivery independently of Prometheus.

### 5. GPUOverheating End-to-End Test

Test resource:
default/hymio-week4-overheat

Initial temperature threshold: 85 C
Fault-injection threshold: 60 C
Restored threshold: 85 C

Observed:
1. GPUHealthy=False
2. Prometheus GPUOverheating inactive
3. Prometheus GPUOverheating pending
4. Prometheus GPUOverheating firing
5. Webhook Receiver received firing
6. GPUHealthy=True after threshold recovery
7. Webhook Receiver received resolved

Real alert fingerprint:
2e0c2aff83008834

Receiver firing time:
2026-11-10T22:42:57.125556Z

Receiver resolved time:
2026-11-10T22:43:27.129222Z

Both notifications used the same fingerprint.

Result: PASS

### 6. Final Cluster State

Final threshold: 85 C

GPUHealthPolicy:
- TelemetryAvailable=True
- GPUHealthy=True
- Ready=True

Monitoring Deployments:
- hymio-alert-receiver: 1/1
- hymio-alertmanager: 1/1
- hymio-operator-metrics-prometheus: 1/1
- hymio-prometheus: 1/1
- mock-dcgm-exporter: 1/1

All deployments available.

## Limitations and Follow-up

- Testing uses synthetic GPU telemetry
- Receiver is for local E2E testing
- No Slack or email notification integration
- No persistent Alertmanager storage
- No Alertmanager high-availability configuration
- No long-duration reliability test
- No Operator-down fault injection
- No production authentication or network isolation
- Python receiver image is tag-pinned, not digest-pinned

## Conclusion

Implemented and validated an end-to-end GPU
health alerting pipeline in local Kubernetes.

Prometheus successfully routed a real
GPUOverheating alert through Alertmanager to
the Webhook Receiver.

The Receiver recorded both Firing and Resolved
notifications for the same alert fingerprint.

Week 5 Day 2 technical E2E: PASS.
