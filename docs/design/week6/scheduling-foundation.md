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
4. Filter unsuitable GPUs.
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

Weights and normalization rules are defined in
"Implementation — Week 6 Day 3" below.

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

## Implementation — Week 6 Day 2 (Filtering)

Code: `operator/internal/scheduling/filter.go`.

`Request.Validate()` enforces the request contract above.
Errors wrap `ErrInvalidRequest`. The temperature limit range
is `[1, 120]` °C, matching `GPUHealthPolicy`.

`Filter(candidates, request, now)` returns `Eligible` and
`Rejections`. Each rejected candidate gets exactly one reason:
the first failed check, in this order.

| # | Check | Reason |
|---|---|---|
| 1 | NodeName and GPUUUID are non-empty | `InvalidIdentity` |
| 2 | (NodeName, GPUUUID) appears once | `DuplicateGPU` |
| 3 | Temperature and utilization readings present | `MissingTelemetry` |
| 4 | `telemetry.Sample.Validate()` passes | `InvalidTelemetry` |
| 5 | No required reading has `ObservedAt` after `now` | `FutureTelemetry` |
| 6 | No required reading is older than `MaxTelemetryAge` | `StaleTelemetry` |
| 7 | Temperature <= `MaxTemperatureCelsius` | `TemperatureExceeded` |
| 8 | Utilization <= `MaxUtilizationPercent` | `UtilizationExceeded` |

Decisions:

- Values equal to a limit pass. Age equal to
  `MaxTelemetryAge` passes.
- Duplicates: every copy of a repeated identity is
  rejected. Conflicting snapshots cannot be trusted, and
  picking one copy would depend on input order. The same
  GPUUUID on different Nodes is treated as distinct.
- Step 4 reuses `Sample.Validate()`, so an invalid optional
  `MemoryUsedMiB` reading also rejects the candidate.
  A missing memory reading does not.
- `MemoryUsedMiB` is never interpreted as free capacity.
- Empty input returns an empty result, not an error.
- Both output slices are sorted by NodeName, then GPUUUID.
  The input slice is not modified.

## Implementation — Week 6 Day 3 (Scoring and Selection)

Code: `operator/internal/scheduling/scoring.go`, `engine.go`.

```
score    = 0.7 × (100 − utilizationPercent)
         + 0.3 × headroom
headroom = clamp(100 × (maxTemp − temp) / maxTemp, 0, 100)
```

Both terms are clamped to `[0, 100]`, so a score is always
finite and within `[0, 100]`. Higher is better.

Example values with `MaxTemperatureCelsius = 80`:

| Temp °C | Util % | Score |
|---|---|---|
| 0 | 0 | 100.000 |
| 40 | 20 | 71.000 |
| 45 | 30 | 62.125 |
| 60 | 50 | 42.500 |
| 65 | 60 | 33.625 |
| 80 | 50 | 35.000 |
| 80 | 100 | 0.000 |

This is an initial heuristic, not a validated model of
available compute.

`DefaultEngine.Select()` runs Filter, scores the eligible
candidates and keeps the highest score:

- Scores are compared exactly, with no epsilon, which keeps
  the ordering transitive. Equal scores are broken by
  NodeName, then GPUUUID, ascending.
- `Rejections` lists only candidates removed by Filter.
  Eligible GPUs that lose on score are not listed.
- An invalid request returns a zero `Decision` and an error.
- The engine is stateless and safe for concurrent use.

Reproducible demo scenarios live in
`operator/internal/scheduling/example_test.go`:

```sh
cd operator
go test ./internal/scheduling -run Example -v
```

## Simulation — Week 6 Day 4

Code: `operator/internal/scheduling/simulation_test.go`.

These tests run Filter and Select together on in-memory
multi-node clusters. They need no kind cluster or Prometheus,
and they are independent of the number of real nodes.

Fixture cluster (4 nodes, 16 GPUs, 17 entries), evaluated
with 80 °C / 90 % / 2m at a fixed time:

| Node | GPU | Temp °C | Util % | Age | Result |
|---|---|---|---|---|---|
| gpu-node-1 | GPU-0 | 72 | 88 | 15s | eligible, 11.400 |
| gpu-node-1 | GPU-1 | 76 | 90 | 15s | eligible, 8.500 (util at limit) |
| gpu-node-1 | GPU-2 | 83 | 60 | 15s | `TemperatureExceeded` |
| gpu-node-1 | GPU-3 | 70 | 97 | 15s | `UtilizationExceeded` |
| gpu-node-2 | GPU-0 | 55 | 35 | 30s | eligible, 54.875 |
| gpu-node-2 | GPU-1 | 48 | 20 | 30s | eligible, 68.000 |
| gpu-node-2 | GPU-2 | — | 0 | 15s | `MissingTelemetry` |
| gpu-node-2 | GPU-3 | 41 | 15 | 5m | `StaleTelemetry` |
| gpu-node-3 | GPU-0 | 35 | 0 | 3m | `StaleTelemetry` |
| gpu-node-3 | GPU-1 | 40 | 20 | 10s | **selected, 71.000** |
| gpu-node-3 | GPU-2 | 30 | 5 | −30s | `FutureTelemetry` |
| gpu-node-3 | GPU-3 | 35 | NaN | 15s | `InvalidTelemetry` |
| gpu-node-4 | GPU-0 | 40 | 20 | 2m | eligible, 71.000 (loses tie-break) |
| gpu-node-4 | GPU-1 | 80 | 20 | 15s | eligible, 56.000 (temp at limit) |
| gpu-node-4 | GPU-2 ×2 | 25 | 0 | 15s / 1m | `DuplicateGPU` (both copies) |
| gpu-node-4 | (empty) | 30 | 0 | 15s | `InvalidIdentity` |

The idlest and coolest GPUs are all rejected. This checks
that bad data never wins on raw values.

Coverage:

| Test | What it proves |
|---|---|
| `TestSimulationClusterDecision` | Exact eligible set, per-GPU scores, winner and sorted rejections for the fixture |
| `TestSimulationScenarios` | Winner overheats; evaluation 1s later makes the tie partner stale; duplicate resolved; stricter/relaxed limits |
| `TestSimulationNoEligibleGPU` | Empty cluster, heat wave, telemetry outage, Prometheus lag, only GPU duplicated: `Found=false`, zero selection, every candidate rejected |
| `TestSimulationInputOrderIndependent` | Reverse plus 500 seeded shuffles per cluster give an identical Decision; input slice unchanged; three-way exact tie |
| `TestSimulationDuplicateGPUIdentity` | Identical, conflicting (fresh/stale, idle/overloaded) and triplicate copies are all rejected; same UUID on two nodes stays distinct; empty identity is `InvalidIdentity`, not `DuplicateGPU` |
| `TestSimulationAgainstReferenceModel` | 500 seeded random clusters (1–64 GPUs, ~10 % duplicated, coarse grid for frequent ties). The generator knows each GPU's expected reason, and a brute-force reference picks the winner without calling Filter or Select |

Every simulated Decision is also checked against invariants:
each candidate is eligible or rejected exactly once,
`Found` is true exactly when something is eligible, the
winner is never rejected, and no eligible GPU beats it on
score or tie-break.

Open question for review: the duplicate key is
(NodeName, GPUUUID). Physical GPU UUIDs are globally unique,
so the same UUID on two nodes most likely means bad data.
It is currently treated as two GPUs, and a test pins that
behavior. Rejecting it would be a Filter contract change.

## Verification — Week 6 Day 2–4

```sh
cd operator
gofmt -l internal/scheduling                 # no output
go test ./internal/scheduling -count=1 -cover   # 100% statements
go test -race ./internal/scheduling -count=1
go test ./internal/scheduling -run Simulation -v   # Day 4 only
make test                                    # full suite
```
