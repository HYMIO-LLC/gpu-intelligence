# Week 6 — GPU-Aware Scheduling Foundation

## Goal

Build a standalone GPU scheduling decision engine.

The engine evaluates simulated GPU candidates and
recommends one GPU and its Kubernetes Node.

It does not bind Pods, reserve GPUs, or change Nodes.

## Existing Components

- telemetry.Sample: one GPU metric snapshot
- telemetry.Client: FetchNode(ctx, nodeName)
- EvaluateTemperatures: temperature health evaluation
- GPUHealthPolicy: existing alert-only health policy

## Current Telemetry Fields

- NodeName
- GPUUUID
- TemperatureCelsius
- UtilizationPercent
- MemoryUsedMiB
- ObservedAt (per metric)

Missing scheduling information:

- GPU total memory
- GPU allocatable capacity
- Existing workload reservations
- GPU allocation and ownership state

## Scheduling Pipeline

1. Receive a workload request.
2. Read GPU candidate snapshots.
3. Validate metric values and freshness.
4. Fier unsuitable GPUs.
5. Score eligible GPUs.
6. Select a deterministic winner.
7. Return a decision and rejection reasons.

## Phase 1 Filtering

Reject GPU candidates when:

- GPU identity is missing.
- Required telemetry is unavailable or invalid.
- Required telemetry is stale or future-dated.
- Temperature exceeds the configured limit.
- Utilization exceeds the configured limit.

## Phase 1 Scoring

Initial scoring will use:

- GPU utilization
- Temperature headroom

Scores must be deterministic.
Ties must have a stable ordering.

Weights and normalization rules will be defined
before implementation.

## Scope Limitations

- Single-GPU workload selection
- No actual Kubernetes Pod scheduling
- No resource reservation
- No concurrent scheduling guarantees
- No multi-GPU placement
- No memory-capacity filtering until the
  total/allocatable memory model is available

## Testing Strategy

Use Go unit tests with simulated GPU candidates.

Include healthy, overloaded, overheated, missing,
stale, invalid, and tied candidates.

Do not require a running Kubernetes cluster for
the core scheduling algorithm.

## Integration Strategy

Keep the scheduling package independent from
controller-runtime and the GPUHealthPolicy Reconciler.

Integrate telemetry and Kubernetes after the
decision engine passes unit tests.

## API Contract — Week 6 Day 1

### Request Validation

A request is invalid when:

- MaxTemperatureCelsius is non-finite or outside
  the accepted temperature range.
- MaxUtilizationPercent is non-finite or outside
  the range (0, 100].
- MaxTelemetryAge is zero or negative.

Invalid requests return an error.

### Candidate Filtering

Each GPU candidate is evaluated independently.

A candidate is rejected if required identity or
telemetry fields are missing, invalid, stale, or
future-dated.

The initial engine requires temperature and
utilization readings.

MemoryUsedMiB is not required until a memory
capacity model is implemented.

### Decision Semantics

Found=true means a GPU was selected.

Found=false means no eligible GPU was found.
This is not an invalid-request error.

When Found=false, SelectedNode, SelectedGPU,
and Score retain zero values.

Rejections contain candidate-specific reasons.

### Determinism

The same candidates, request, and evaluation time
must produce the same decision regardless of input
candidate ordering.

For equal scores, tie-break using:

1. NodeName ascending
2. GPUUUID ascending

### Safety and Scope

The engine returns recommendations only.

It does not allocate GPU resources, reserve
capacity, bind Pods, or mutate Kubernetes objects.
