package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type policyTelemetryResult struct {
	Available metav1.ConditionStatus
	Health    metav1.ConditionStatus
	Reason    string
	Message   string
}

// evaluatePolicyTelemetry evaluates all matched Kubernetes Nodes.
// A confirmed overtemperature reading takes precedence over unknown health.
// Failed or empty Node queries never establish healthy status.
func (r *GPUHealthPolicyReconciler) evaluatePolicyTelemetry(
	ctx context.Context,
	nodes []corev1.Node,
	threshold float64,
) policyTelemetryResult {
	if len(nodes) == 0 {
		return policyTelemetryResult{
			Available: metav1.ConditionFalse,
			Health:    metav1.ConditionUnknown,
			Reason:    "NoMatchedNodes",
			Message:   "No Kubernetes Nodes match this policy",
		}
	}

	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}

	var allSamples []telemetry.Sample
	incompleteNodes := 0

	for _, node := range nodes {
		samples, err := r.Telemetry.FetchNode(ctx, node.Name)
		if err != nil || len(samples) == 0 {
			incompleteNodes++
			continue
		}
		allSamples = append(allSamples, samples...)
	}

	evaluation := telemetry.EvaluateTemperatures(
		allSamples,
		threshold,
		now,
		telemetry.DefaultMaxAge,
	)

	available := evaluation.TelemetryAvailable &&
		incompleteNodes == 0

	result := policyTelemetryResult{
		Available: metav1.ConditionFalse,
		Health:    metav1.ConditionUnknown,
		Reason:    evaluation.Reason,
		Message: fmt.Sprintf(
			"%s; %d/%d Node(s) without queryable GPU samples",
			evaluation.Message,
			incompleteNodes,
			len(nodes),
		),
	}

	if available {
		result.Available = metav1.ConditionTrue
	}

	switch evaluation.Health {
	case telemetry.HealthUnhealthy:
		result.Health = metav1.ConditionFalse
	case telemetry.HealthHealthy:
		if available {
			result.Health = metav1.ConditionTrue
		}
	}

	if incompleteNodes > 0 &&
		result.Health == metav1.ConditionUnknown {
		result.Reason = "IncompleteNodeTelemetry"
	}

	return result
}
