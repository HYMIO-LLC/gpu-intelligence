package controller

import (
	"context"
	"testing"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPolicyMetricsReconcileAndDelete(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	policy := &gpuv1alpha1.GPUHealthPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "metrics-test",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gpuv1alpha1.GPUHealthPolicySpec{
			NodeSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"gpu-test": "enabled"},
			},
			MaxTemperatureCelsius: 85,
			Action:                "AlertOnly",
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "gpu-node-1",
			Labels: map[string]string{"gpu-test": "enabled"},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.GPUHealthPolicy{}).
		WithObjects(policy, node).
		Build()

	registry := prometheus.NewRegistry()
	metrics := NewPolicyMetrics(registry)

	reconciler := &GPUHealthPolicyReconciler{
		Client:  k8sClient,
		Scheme:  scheme,
		Metrics: metrics,
	}

	key := types.NamespacedName{
		Name:      "metrics-test",
		Namespace: "default",
	}
	req := ctrl.Request{NamespacedName: key}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	assertGauge := func(name string, got, want float64) {
		t.Helper()
		if got != want {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
	}

	assertGauge(
		"matched_nodes",
		testutil.ToFloat64(metrics.matchedNodes.WithLabelValues("default", "metrics-test")),
		1,
	)

	// Telemetry is intentionally not configured; unknown must not
	// be reported as healthy.
	assertGauge(
		"healthy",
		testutil.ToFloat64(metrics.healthy.WithLabelValues("default", "metrics-test")),
		-1,
	)

	assertGauge(
		"telemetry_available",
		testutil.ToFloat64(metrics.telemetryAvailable.WithLabelValues("default", "metrics-test")),
		-1,
	)

	if err := k8sClient.Delete(ctx, policy); err != nil {
		t.Fatal(err)
	}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}

	for _, family := range families {
		if len(family.GetMetric()) != 0 {
			t.Fatalf(
				"deleted policy still has metric series in %s",
				family.GetName(),
			)
		}
	}
}

func TestPolicyMetricsSkipStaleGeneration(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	policy := &gpuv1alpha1.GPUHealthPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "stale-policy",
			Namespace:  "default",
			Generation: 2,
		},
		Status: gpuv1alpha1.GPUHealthPolicyStatus{
			ObservedGeneration: 1,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.GPUHealthPolicy{}).
		WithObjects(policy).
		Build()

	registry := prometheus.NewRegistry()
	metrics := NewPolicyMetrics(registry)

	// Simulate an earlier successful generation before the Spec changes.
	metrics.Set("default", "stale-policy", 1, 1, 2)

	reconciler := &GPUHealthPolicyReconciler{
		Client:  k8sClient,
		Scheme:  scheme,
		Metrics: metrics,
	}

	err := reconciler.publishPolicyMetrics(
		ctx,
		types.NamespacedName{
			Namespace: "default",
			Name:      "stale-policy",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}

	for _, family := range families {
		if len(family.GetMetric()) != 0 {
			t.Fatalf("stale generation emitted metrics: %s", family.GetName())
		}
	}
}
