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

func TestPolicyTelemetryConditionReasons(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

	makeSample := func(temp float64) telemetry.Sample {
		return telemetry.Sample{
			NodeName: "node-1",
			GPUUUID:  "GPU-001",
			TemperatureCelsius: &telemetry.Reading{
				Value:      temp,
				ObservedAt: now,
			},
		}
	}

	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}},
	}

	tests := []struct {
		name                string
		nodes               []corev1.Node
		data                []telemetry.Sample
		queryError          error
		wantAvailable       metav1.ConditionStatus
		wantHealth          metav1.ConditionStatus
		wantAvailableReason string
		wantHealthReason    string
	}{
		{
			name:                "healthy",
			nodes:               nodes,
			data:                []telemetry.Sample{makeSample(68)},
			wantAvailable:       metav1.ConditionTrue,
			wantHealth:          metav1.ConditionTrue,
			wantAvailableReason: "TelemetryAvailable",
			wantHealthReason:    "TemperatureWithinLimit",
		},
		{
			name:                "overheated",
			nodes:               nodes,
			data:                []telemetry.Sample{makeSample(95)},
			wantAvailable:       metav1.ConditionTrue,
			wantHealth:          metav1.ConditionFalse,
			wantAvailableReason: "TelemetryAvailable",
			wantHealthReason:    "TemperatureExceeded",
		},
		{
			name:                "missing telemetry",
			nodes:               nodes,
			wantAvailable:       metav1.ConditionFalse,
			wantHealth:          metav1.ConditionUnknown,
			wantAvailableReason: "IncompleteNodeTelemetry",
			wantHealthReason:    "IncompleteNodeTelemetry",
		},
		{
			name:                "Prometheus query failure",
			nodes:               nodes,
			queryError:          errors.New("connection refused"),
			wantAvailable:       metav1.ConditionFalse,
			wantHealth:          metav1.ConditionUnknown,
			wantAvailableReason: "IncompleteNodeTelemetry",
			wantHealthReason:    "IncompleteNodeTelemetry",
		},
		{
			name:                "no matched nodes",
			nodes:               nil,
			wantAvailable:       metav1.ConditionFalse,
			wantHealth:          metav1.ConditionUnknown,
			wantAvailableReason: "NoMatchedNodes",
			wantHealthReason:    "NoMatchedNodes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := &fakeTelemetryClient{
				data: map[string][]telemetry.Sample{
					"node-1": tt.data,
				},
				errors: map[string]error{},
			}
			if tt.queryError != nil {
				fakeClient.errors["node-1"] = tt.queryError
			}

			r := &GPUHealthPolicyReconciler{
				Telemetry: fakeClient,
				Now:       func() time.Time { return now },
			}

			got := r.evaluatePolicyTelemetry(
				context.Background(),
				tt.nodes,
				80,
			)

			if got.Available != tt.wantAvailable {
				t.Errorf(
					"available = %s, want %s",
					got.Available, tt.wantAvailable,
				)
			}

			if got.Health != tt.wantHealth {
				t.Errorf(
					"health = %s, want %s",
					got.Health, tt.wantHealth,
				)
			}

			if got.AvailableReason != tt.wantAvailableReason {
				t.Errorf(
					"available reason = %s, want %s",
					got.AvailableReason, tt.wantAvailableReason,
				)
			}

			if got.HealthReason != tt.wantHealthReason {
				t.Errorf(
					"health reason = %s, want %s",
					got.HealthReason, tt.wantHealthReason,
				)
			}
		})
	}
}
