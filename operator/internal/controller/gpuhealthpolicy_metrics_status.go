package controller

import (
	"context"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// metricConditionValue maps a Kubernetes condition to a Gauge.
func metricConditionValue(
	conditions []metav1.Condition,
	conditionType string,
) float64 {
	condition := meta.FindStatusCondition(conditions, conditionType)
	if condition == nil {
		return -1
	}

	switch condition.Status {
	case metav1.ConditionTrue:
		return 1
	case metav1.ConditionFalse:
		return 0
	default:
		return -1
	}
}

// publishPolicyMetrics reads the persisted Kubernetes Status before
// updating the metrics exposed by the Controller.
// No metrics are emitted for an unobserved policy generation.
func (r *GPUHealthPolicyReconciler) publishPolicyMetrics(
	ctx context.Context,
	key types.NamespacedName,
) error {
	if r.Metrics == nil {
		return nil
	}

	policy := &gpuv1alpha1.GPUHealthPolicy{}
	if err := r.Get(ctx, key, policy); err != nil {
		if apierrors.IsNotFound(err) {
			r.Metrics.Delete(key.Namespace, key.Name)
			return nil
		}
		return err
	}

	if policy.Status.ObservedGeneration != policy.Generation {
		// Do not expose a previous generation as current health.
		r.Metrics.Delete(key.Namespace, key.Name)
		return nil
	}

	r.Metrics.Set(
		key.Namespace,
		key.Name,
		metricConditionValue(policy.Status.Conditions, "GPUHealthy"),
		metricConditionValue(policy.Status.Conditions, "TelemetryAvailable"),
		policy.Status.MatchedNodes,
	)

	return nil
}
