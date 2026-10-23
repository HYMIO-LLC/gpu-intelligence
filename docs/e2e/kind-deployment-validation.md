# Hymio GPU Intelligence — kind Deployment Validation

## Environment

- Date: October 23, 2026
- Cluster: hymio-dev
- Kubernetes: v1.37.0
- Architecture: linux/arm64
- Runtime: Docker Desktop + kind
- Operator Image: hymio-operator:dev

## Objective

Validate GPUHealthPolicy reconciliation in a running Kubernetes
cluster, beyond envtest-based integration testing.

## Deployment

The Operator was built as a local Docker image, loaded into kind,
and deployed using Kubebuilder-generated Kustomize manifests.

Commands:

    make docker-build IMG=hymio-operator:dev BASE_IMAGE=golang:1.26.0
    kind load docker-image hymio-operator:dev --name hymio-dev
    make deploy IMG=hymio-operator:dev

## RBAC Issue Identified

The initial Node RBAC configuration granted get/list permissions
but omitted watch.

The controller-runtime cache attempted to watch Kernetes Nodes,
causing Forbidden errors.

Resolution:

- Added watch to the Node RBAC marker.
- Regenerated the ClusterRole.
- Applied updated Kubernetes manifests.
- Verified Node list/watch permissions.
- Restarted the Controller Deployment.

## End-to-End Scenarios

| Scenario | Expected | Observed |
|---|---|---|
| Operator Deployment | Successful rollout | PASS |
| CRD Installation | GPUHealthPolicy available | PASS |
| Controller Startup | Manager and workers started | PASS |
| Policy Creation | Ready=True | PASS |
| Initial Node Selection | matchedNodes=0 | PASS |
| Matching Node Label Added | matchedNodes=1 | PASS |
| Matching Node Label Removed | matchedNodes=0 | PASS |
| Node List Permission | Allowed | PASS |
| Node Watch Permission | Allowed | PASS |

Observed state transitions: 0 -> 1 -> 0.

## Limitations

- No physical NVIDIA GPU was used.
- Node labels simulated GPU node selection.
- GPU telemetry and temperature evaluation are not implemented.
- Ready=True means policy accepted, not GPU healthy.
- Reconciliation uses a two-minute periodic requeue;
  Node events are not directly mapped to Policy events.

## Outcome

The GPUHealthPolicy Controller was successfully deployed and
validated in a local Kubernetes cluster.

The E2E test identified and resolved a missing Node Watch
RBAC permission not exposed by earlier envtest tests.
