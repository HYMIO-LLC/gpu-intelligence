package controller

import (
	"context"
	"testing"
	"time"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"
	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileTelemetryConditions(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		temp       float64
		wantHealth metav1.ConditionStatus
	}{
		{"healthy", 68, metav1.ConditionTrue},
		{"overheated", 95, metav1.ConditionFalse},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()

			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}

			policy := &gpuv1alpha1.GPUHealthPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "gpu-test",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: gpuv1alpha1.GPUHealthPolicySpec{
					NodeSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"gpu-test": "enabled",
						},
					},
					MaxTemperatureCelsius: 80,
					Action:                "AlertOnly",
				},
			}

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu-node-1",
					Labels: map[string]string{
						"gpu-test": "enabled",
					},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&gpuv1alpha1.GPUHealthPolicy{}).
				WithObjects(policy, node).
				Build()

			r := &GPUHealthPolicyReconciler{
				Client: k8sClient,
				Scheme: scheme,
				Telemetry: &fakeTelemetryClient{
					data: map[string][]telemetry.Sample{
						"gpu-node-1": {
							{
								NodeName: "gpu-node-1",
								GPUUUID:  "GPU-001",
								TemperatureCelsius: &telemetry.Reading{
									Value:      tc.temp,
									ObservedAt: now,
								},
							},
						},
					},
				},
				Now: func() time.Time { return now },
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "gpu-test",
				},
			}

			ctx := context.Background()

			for attempt := 0; attempt < 2; attempt++ {
				result, err := r.Reconcile(ctx, req)
				if err != nil {
					t.Fatal(err)
				}

				if result.RequeueAfter != 2*time.Minute {
					t.Fatalf("unexpected requeue: %+v", result)
				}
			}

			actual := &gpuv1alpha1.GPUHealthPolicy{}
			if err := k8sClient.Get(
				ctx, req.NamespacedName, actual,
			); err != nil {
				t.Fatal(err)
			}

			ready := meta.FindStatusCondition(
				actual.Status.Conditions, "Ready",
			)
			available := meta.FindStatusCondition(
				actual.Status.Conditions, "TelemetryAvailable",
			)
			healthy := meta.FindStatusCondition(
				actual.Status.Conditions, "GPUHealthy",
			)

			if ready == nil ||
				ready.Status != metav1.ConditionTrue ||
				ready.Reason != "PolicyAccepted" {
				t.Fatalf("Ready condition invalid: %+v", ready)
			}

			if available == nil ||
				available.Status != metav1.ConditionTrue {
				t.Fatalf("TelemetryAvailable invalid: %+v", available)
			}

			if healthy == nil ||
				healthy.Status != tc.wantHealth {
				t.Fatalf("GPUHealthy invalid: %+v", healthy)
			}

			if actual.Status.MatchedNodes != 1 ||
				actual.Status.ObservedGeneration != 1 {
				t.Fatalf("unexpected policy status: %+v", actual.Status)
			}
		})
	}
}
