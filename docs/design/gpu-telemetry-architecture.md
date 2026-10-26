# GPU Telemetry Architecture

## 1. Objective

Define a source-independent GPU telemetry domain model and
client interface for Hymio GPU Intelligence.

The initial implementation supports telemetry modeling and
validation. It does not yet collect real GPU metrics.

## 2. Planned Data Flow

Mock DCGM Exporter
    -> Prometheus Scraper
    -> Prometheus Query API
    -> Go Telemetry Client
    -> GPU Health Evaluation
    -> GPUHealthPolicy Status

A real NVIDIA DCGM Exporter may replace the mock source
in a future GPU-enabled deployment.

## 3. Domain Model

Package: `operator/internal/telemetry`

### Reading

- Value: float64
- ObservedAt: time.Time

A nil Reading indicates unavailable telemetry.
A Reading with Value=0 represents an observed zero value.

### Sample

Each Sample represents one GPU on one Kubernetes Node.

Identity:
- NodeName
- GPUUUID

Metrics:
- TemperatureCelsius
- UtilizationPercent
- MemoryUsedMiB

## 4. Validation Rules

- NodeName and GPUUUID must be nonempty.
- Observed metric values must be finite.
- Observed metrics require nonzero timestamps.
- Utilization must be within 0 to 100 percent.
- Memory usage must be nonnegative.
- Temperature must not be below absolute zero.
- Missing individual metrics are permitted.

Validation establishes structural and numeric validity.
It does not establish telemetry freshness or GPU health.

## 5. Telemetry Client

The Client interface provides:

FetchNode(ctx context.Context, nodeName string) ([]Sample, error)

The implementation will be independent of the Controller.

Future implementations may retrieve metrics from Prometheus
or a deterministic fake provider used by tests.

## 6. Error and Availability Semantics

The following cases must remain distinguishable:

- Successful query with zero matching GPU samples
- Successful query with missing individual metrics
- Failed telemetry query
- Stale telemetry sample
- Invalid telemetry value
- Node or GPU identity mapping failure

No data must not be interpreted as a healthy GPU.

## 7. Kubernetes Integration

The existing GPUHealthPolicy Ready condition indicates
policy acceptance, not measured GPU health.

Future telemetry assessment must use separate conditions
and preserve the current Ready semantics.

Status updates should remain idempotent and follow the
existing reconciliation and optimistic-locking practices.

## 8. Hardware Limitations

The development environment uses macOS ARM64 and kind.

The initial implementation uses mock GPU measurements.
It does not validate NVIDIA drivers, physical GPUs,
or live NVIDIA DCGM hardware telemetry.

## 9. Week 3 Roadmap

Day 1: Telemetry domain model, interface, tests, design.
Day 2: Mock exporter and Prometheus scraping.
Day 3: Prometheus query client and failure handling.
Day 4: Policy evaluation and condition integration.
Day 5: Local kind E2E tests and documentation.
