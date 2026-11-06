# Week 4 Day 4 — GPU Health Observability

Project: Hymio GPU Intelligence
Milestone: Week 4, Day 4
Validation Date: November 5, 2026 (PST)
Environment: Docker Desktop, kind, Kubernetes v1.37

## Objective

Expose GPUHealthPolicy health information as Prometheus
metrics and verify authenticated HTTPS scraping in Kubernetes.

## Implementation

Introduced three custom Prometheus Gauges:

- hymio_gpu_policy_healthy
  - 1: Healthy
  - 0: Unhealthy
  - -1: Unknown

- hymio_gpu_policy_telemetry_available
  - 1: Available
  - 0: Unavailable
  - -1: Unknown

- hymio_gpu_policy_matched_nodes
  - Number of Kubernetes Nodes matching the policy

Metric labels:
- namespace
- policy

Integrated the collectors with controller-runtime's
Prometheus Registry and the GPUHealthPolicy Reconciler.

Metrics are published from persisted Kubernetes Status.
Policy deletion removes associated metric sers.
Stale observed generations are not published as current.

## Unit and Integration Validation

- Metrics value tests: PASS
- Metrics deletion test: PASS
- Metrics transition test: PASS
- Reconcile and deletion integration test: PASS
- Stale generation invalidation test: PASS
- make build: PASS
- make test: PASS
- go vet: PASS

Controller coverage: 86.5%
Telemetry coverage: 90.9%

## Kubernetes Deployment

Operator image:
hymio-operator:week4-day4

Platform:
linux/arm64

The image was loaded into the hymio-dev kind cluster.
The Operator Deployment rolled out successfully.

## HTTPS Metrics Security

The Operator exposes HTTPS metrics on port 8443.

A dedicated ServiceAccount was created:
hymio-monitoring/hymio-prometheus-metrics

A dedicated ClusterRole and ClusterRoleBinding grant
GET access to the non-resource URL /metrics.

Authorization validation:
- Dedicated ServiceAccount: yes
- Monitoring default ServiceAccount: no

Authenticated HTTPS endpoint: HTTP 200.

## Prometheus Scrape Validation

A separate local Prometheus validator was deployed:

hymio-operator-metrics-prometheus

This preserved the existing Prometheus Deployment and
its mock DCGM telemetry collection.

Prometheus scrape interval: 15 seconds

Observed PromQL values for default/hymio-week4-overheat:

- up{job="hymio-operator"} = 1
- hymio_gpu_policy_healthy = 1
- hymio_gpu_policy_telemetry_available = 1
- hymio_gpu_policy_matched_nodes = 1

## Overtemperature Transition

Changed the policy temperature threshold from
85 degrees Celsius to 60 degrees Celsius.

Observed Kubernetes condition:
GPUHealthy=False / TemperatureExceeded

Observed Prometheus Gauge:
hymio_gpu_policy_healthy = 0

Result: PASS

## Recovery Transition

Restored the temperature threshold to 85 degrees Celsius.

Observed Kubernetes conditions:
- GPUHealthy=True / TemperatureWithinLimit
- TelemetryAvailable=True / TelemetryAvailable
- Ready=True / PolicyAccepted

Observed Prometheus Gauges:
- hymio_gpu_policy_healthy = 1
- hymio_gpu_policy_telemetry_available = 1
- up = 1

Result: PASS

## Environment After Testing

- Operator: 1/1 available
- Original Prometheus: 1/1 available
- Mock DCGM Exporter: 1/1 available
- Operator Metrics Prometheus: 1/1 available

The test policy was restored to its 85-degree threshold.

## Limitations

- Local kind validation with synthetic GPU telemetry
- No physical GPU validation
- Local metrics validator uses insecure TLS certificate
  verification for a self-signed development certificate
- No persistent Prometheus TSDB storage
- No production-grade CA management
- No multi-replica metrics consistency guarantee
- No custom Gauge E2E test for Prometheus outage
- No long-duration or high-load performance testing

## Result

GPU Health Observability MVP validated successfully.

The Operator exposes GPU health through authenticated
HTTPS metrics, and Prometheus successfully scrapes,
stores, and queries those metrics.

Healthy, overtemperature, and recovery transitions
were verified through Kubernetes Status and PromQL.
