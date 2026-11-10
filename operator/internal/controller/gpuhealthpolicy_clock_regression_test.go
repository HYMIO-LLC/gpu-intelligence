package controller

import (
	"context"
	"testing"
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// advancingTelemetryClient simulates telemetry arriving after
// a network request has advanced the evaluation clock.
type advancingTelemetryClient struct {
	now *time.Time
}

func (c *advancingTelemetryClient) FetchNode(
	ctx context.Context,
	nodeName string,
) ([]telemetry.Sample, error) {
	*c.now = c.now.Add(100 * time.Millisecond)

	return []telemetry.Sample{
		{
			NodeName: nodeName,
			GPUUUID:  "GPU-CLOCK-001",
			TemperatureCelsius: &telemetry.Reading{
				Value:      68,
				ObservedAt: *c.now,
			},
		},
	}, nil
}

func TestTelemetryEvaluationUsesClockAfterFetch(t *testing.T) {
	now := time.Date(
		2026, 11, 9, 12, 0, 0, 0, time.UTC,
	)

	client := &advancingTelemetryClient{
		now: &now,
	}

	reconciler := &GPUHealthPolicyReconciler{
		Telemetry: client,
		Now: func() time.Time {
			return now
		},
	}

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gpu-node-1",
			},
		},
	}

	result := reconciler.evaluatePolicyTelemetry(
		context.Background(),
		nodes,
		85,
	)

	if result.Available != metav1.ConditionTrue {
		t.Fatalf(
			"TelemetryAvailable = %s, want True; reason=%s message=%s",
			result.Available,
			result.AvailableReason,
			result.Message,
		)
	}

	if result.Health != metav1.ConditionTrue {
		t.Fatalf(
			"GPUHealthy = %s, want True; reason=%s message=%s",
			result.Health,
			result.HealthReason,
			result.Message,
		)
	}

	if result.HealthReason != "TemperatureWithinLimit" {
		t.Fatalf(
			"HealthReason = %s, want TemperatureWithinLimit",
			result.HealthReason,
		)
	}
}
