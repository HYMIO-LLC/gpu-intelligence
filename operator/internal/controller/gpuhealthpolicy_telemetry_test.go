package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeTelemetryClient struct {
	data   map[string][]telemetry.Sample
	errors map[string]error
}

func (f *fakeTelemetryClient) FetchNode(
	ctx context.Context,
	nodeName string,
) ([]telemetry.Sample, error) {
	if err := f.errors[nodeName]; err != nil {
		return nil, err
	}
	return f.data[nodeName], nil
}

func TestPolicyTelemetryEvaluation(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

	sample := func(node, uuid string, temp float64) telemetry.Sample {
		return telemetry.Sample{
			NodeName: node,
			GPUUUID:  uuid,
			TemperatureCelsius: &telemetry.Reading{
				Value:      temp,
				ObservedAt: now,
			},
		}
	}

	node := func(name string) corev1.Node {
		return corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
	}

	tests := []struct {
		name          string
		nodes         []corev1.Node
		data          map[string][]telemetry.Sample
		errors        map[string]error
		wantAvailable metav1.ConditionStatus
		wantHealth    metav1.ConditionStatus
	}{
		{
			name:  "single healthy node",
			nodes: []corev1.Node{node("node-1")},
			data: map[string][]telemetry.Sample{
				"node-1": {sample("node-1", "GPU-001", 68)},
			},
			wantAvailable: metav1.ConditionTrue,
			wantHealth:    metav1.ConditionTrue,
		},
		{
			name:  "single overheated node",
			nodes: []corev1.Node{node("node-1")},
			data: map[string][]telemetry.Sample{
				"node-1": {sample("node-1", "GPU-001", 95)},
			},
			wantAvailable: metav1.ConditionTrue,
			wantHealth:    metav1.ConditionFalse,
		},
		{
			name:  "two healthy nodes",
			nodes: []corev1.Node{node("node-1"), node("node-2")},
			data: map[string][]telemetry.Sample{
				"node-1": {sample("node-1", "GPU-001", 68)},
				"node-2": {sample("node-2", "GPU-002", 75)},
			},
			wantAvailable: metav1.ConditionTrue,
			wantHealth:    metav1.ConditionTrue,
		},
		{
			name:  "healthy node and failed node",
			nodes: []corev1.Node{node("node-1"), node("node-2")},
			data: map[string][]telemetry.Sample{
				"node-1": {sample("node-1", "GPU-001", 68)},
			},
			errors: map[string]error{
				"node-2": errors.New("Prometheus unavailable"),
			},
			wantAvailable: metav1.ConditionFalse,
			wantHealth:    metav1.ConditionUnknown,
		},
		{
			name:  "overheated node and failed node",
			nodes: []corev1.Node{node("node-1"), node("node-2")},
			data: map[string][]telemetry.Sample{
				"node-1": {sample("node-1", "GPU-001", 95)},
			},
			errors: map[string]error{
				"node-2": errors.New("Prometheus unavailable"),
			},
			wantAvailable: metav1.ConditionFalse,
			wantHealth:    metav1.ConditionFalse,
		},
		{
			name:          "no matched nodes",
			nodes:         nil,
			wantAvailable: metav1.ConditionFalse,
			wantHealth:    metav1.ConditionUnknown,
		},
		{
			name:          "node with empty telemetry",
			nodes:         []corev1.Node{node("node-1")},
			wantAvailable: metav1.ConditionFalse,
			wantHealth:    metav1.ConditionUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &GPUHealthPolicyReconciler{
				Telemetry: &fakeTelemetryClient{
					data:   tt.data,
					errors: tt.errors,
				},
				Now: func() time.Time { return now },
			}

			got := r.evaluatePolicyTelemetry(
				context.Background(),
				tt.nodes,
				80,
			)

			if got.Available != tt.wantAvailable {
				t.Errorf(
					"TelemetryAvailable = %s, want %s",
					got.Available, tt.wantAvailable,
				)
			}

			if got.Health != tt.wantHealth {
				t.Errorf(
					"GPUHealthy = %s, want %s",
					got.Health, tt.wantHealth,
				)
			}
		})
	}
}
