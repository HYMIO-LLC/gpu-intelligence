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
	Available       metav1.ConditionStatus
	Health          metav1.ConditionStatus
	AvailableReason string
	HealthReason    string
	Message         string
}

// evaluatePolicyTelemetry evaluates all matched Kubernetes Nodes.
// Confirmed overtemperature takes precedence over unknown health.
// Missing or failed queries never establish healthy status.
func (r *GPUHealthPolicyReconciler) evaluatePolicyTelemetry(
	ctx context.Context,
	nodes []corev1.Node,
	threshold float64,
) policyTelemetryResult {
	if len(nodes) == 0 {
		return policyTelemetryResult{
			Available:       metav1.ConditionFalse,
			Health:          metav1.ConditionUnknown,
			AvailableReason: "NoMatchedNodes",
			HealthReason:    "NoMatchedNodes",
			Message:         "No Kubernetes Nodes match this policy",
		}
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

	// Evaluate freshness after completing telemetry queries.
	// Prometheus instant-query timestamps may be later than fetch start.
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
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
		Available:       metav1.ConditionFalse,
		Health:          metav1.ConditionUnknown,
		AvailableReason: "IncompleteTelemetry",
		HealthReason:    evaluation.Reason,
		Message: fmt.Sprintf(
			"%s; %d/%d Node(s) without queryable GPU samples",
			evaluation.Message,
			incompleteNodes,
			len(nodes),
		),
	}

	if available {
		result.Available = metav1.ConditionTrue
		result.AvailableReason = "TelemetryAvailable"
	} else if incompleteNodes > 0 {
		result.AvailableReason = "IncompleteNodeTelemetry"
	}

	switch evaluation.Health {
	case telemetry.HealthUnhealthy:
		result.Health = metav1.ConditionFalse
		result.HealthReason = "TemperatureExceeded"

	case telemetry.HealthHealthy:
		if available {
			result.Health = metav1.ConditionTrue
			result.HealthReason = "TemperatureWithinLimit"
		}
	}

	if incompleteNodes > 0 &&
		result.Health == metav1.ConditionUnknown {
		result.HealthReason = "IncompleteNodeTelemetry"
	}

	return result
}
