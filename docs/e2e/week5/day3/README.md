# Week 5 Day 3 — Alert Reliability and Failure Recovery

## Objective

Validate the reliability of GPU Operator alerting when:

1. The Operator metrics endpoint becomes unavailable.
2. The Prometheus scrape target disappears entirely.
3. The monitoring system recovers after restoring normal operation.

## Environment

- Kubernetes: kind-hymio-dev
- Prometheus: v3.13.3
- Alertmanager: v0.34.2
- Operator: operator-controller-manager
- Alert: GPUOperatorMetricsDown
- Severity: critical

## Alert Rule Improvement

Original expression:

```promql
up{job="hymio-operator"} == 0
```

Updated expression:

```promql
(up{job="hymio-operator"} == 0)
or absent(up{job="hymio-operator"})
```

The updated rule detects failed scrapes and the complete
absence of the configured Operator scrape time series.

The alert retains a one-minute `for` duration.

## Validation 1: Operator Down / Recovery

Fault injection:
- Scaled Operator deploymentrom 1 to 0 replicas.
- Observed alert transition: inactive -> pending -> firing.
- Confirmed Alertmanager delivered a Firing webhook.
- Restored deployment to 1 replica.
- Confirmed Prometheus up returned to 1.
- Confirmed Alertmanager delivered a Resolved webhook.

Result: PASS.

Webhook evidence:
- Firing: 2026-11-11T21:38:38Z
- Resolved: 2026-11-11T21:39:08Z
- Fingerprint: 693d8a445f9aabfc

## Validation 2: Prometheus Rule Unit Tests

Used promtool to validate:

1. Healthy scrape target: no alert.
2. Failed scrape: alert fires after the hold period.
3. Missing scrape series: alert fires through absent().

Results:
- Rule validation: SUCCESS, 3 rules found.
- Rule unit tests: SUCCESS.

## Validation 3: No Data End-to-End

Fault injection:
- Temporarily renamed the Prometheus scrape job to
  hymio-operator-nodata-e2e.
- Verified the new job through Prometheus Runtime API.
- Observed the original up series disappear.
- Confirmed absent(up{job="hymio-operator"}) returned 1.
- Observed inactive -> pending -> firing.
- Confirmed Firing webhook delivery.
- Restored the original scrape job.
- Verified Runtime configuration and up=1.
- Confirmed Resolved webhook delivery.

Final automated test result: END-TO-END PASS.
Exit code: 0.

Webhook evidence:
- Firing: 2026-11-12T09:04:09Z
- Resolved: 2026-11-12T09:04:39Z
- Fingerprint: b20fc289e6d7596e

## Failure Investigation

The initial No Data test did not pass.

Issues identified:
- The observation period was too short for the original
  time series to disappear.
- Configuration checks used prefix matching.
- ConfigMap file checks did not verify the Prometheus
  runtime configuration.
- A generated regular expression required correction.

The test script was updated to:
- Wait longer for time-series disappearance.
- Match configured job names exactly.
- Verify active runtime job names using Prometheus API.
- Confirm runtime recovery before reporting success.

The corrected test passed.

## Final Cluster State

- Operator deployment: 1/1 Ready.
- All five monitoring deployments: 1/1 Ready.
- Original scrape job restored: hymio-operator.
- Operator metrics scrape: up=1.
- GPUOperatorMetricsDown: inactive.

## Artifacts

- gpu-health-alerts.yml
- gpu-health-alerts.test.yml
- prometheus-configmap.yaml
- prometheus-nodata-e2e.yml
- test-nodata-e2e.sh

## Limitations

- Validated in a local single-node kind environment.
- Operator scrape monitoring uses one static target.
- Alertmanager and receiver are not configured for HA.
- No persistent alert notification storage was tested.
