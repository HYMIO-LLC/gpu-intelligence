package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"
	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type countingTelemetryDay2 struct {
	calls  int
	sample telemetry.Sample
}

func (c *countingTelemetryDay2) FetchNode(
	ctx context.Context,
	nodeName string,
) ([]telemetry.Sample, error) {
	c.calls++
	return []telemetry.Sample{c.sample}, nil
}

func newDay2TestClient(t *testing.T) (
	client.Client,
	*runtime.Scheme,
	ctrl.Request,
) {
	t.Helper()

	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	policy := &gpuv1alpha1.GPUHealthPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "day2-reliability",
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

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "day2-reliability",
		},
	}

	return k8sClient, scheme, req
}

func newCountingTelemetryDay2(now time.Time) *countingTelemetryDay2 {
	return &countingTelemetryDay2{
		sample: telemetry.Sample{
			NodeName: "gpu-node-1",
			GPUUUID:  "GPU-001",
			TemperatureCelsius: &telemetry.Reading{
				Value:      68,
				ObservedAt: now,
			},
		},
	}
}

// Test 1: A Status conflict retries Kubernetes Patch,
// without making a second telemetry request.
func TestDay2ConflictDoesNotRefetchTelemetry(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

	base, scheme, req := newDay2TestClient(t)

	injected := &conflictClient{
		Client:             base,
		conflictsRemaining: 1,
	}

	telemetryClient := newCountingTelemetryDay2(now)

	r := &GPUHealthPolicyReconciler{
		Client:    injected,
		Scheme:    scheme,
		Telemetry: telemetryClient,
		Now:       func() time.Time { return now },
	}

	result, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	if telemetryClient.calls != 1 {
		t.Fatalf(
			"FetchNode calls = %d, want 1",
			telemetryClient.calls,
		)
	}

	if injected.patchAttempts != 2 {
		t.Fatalf(
			"Status Patch attempts = %d, want 2",
			injected.patchAttempts,
		)
	}

	if result.RequeueAfter != 2*time.Minute {
		t.Fatalf("unexpected Reconcile result: %+v", result)
	}

	actual := &gpuv1alpha1.GPUHealthPolicy{}
	if err := base.Get(ctx, req.NamespacedName, actual); err != nil {
		t.Fatal(err)
	}

	health := meta.FindStatusCondition(
		actual.Status.Conditions,
		"GPUHealthy",
	)

	if health == nil || health.Status != metav1.ConditionTrue {
		t.Fatalf("unexpected GPUHealthy: %+v", health)
	}

	t.Log("One telemetry fetch; two Status Patch attempts.")
}

// This client simulates a Spec update during the first Status Patch.
type specChangingClientDay2 struct {
	client.Client
	changed       bool
	patchAttempts int
}

type specChangingWriterDay2 struct {
	client.SubResourceWriter
	parent *specChangingClientDay2
}

func (c *specChangingClientDay2) Status() client.SubResourceWriter {
	return &specChangingWriterDay2{
		SubResourceWriter: c.Client.Status(),
		parent:            c,
	}
}

func (w *specChangingWriterDay2) Patch(
	ctx context.Context,
	obj client.Object,
	patch client.Patch,
	opts ...client.SubResourcePatchOption,
) error {
	w.parent.patchAttempts++

	if !w.parent.changed {
		w.parent.changed = true

		key := client.ObjectKeyFromObject(obj)
		current := &gpuv1alpha1.GPUHealthPolicy{}

		if err := w.parent.Client.Get(ctx, key, current); err != nil {
			return err
		}

		// Update the policy Spec before the retry sees it.
		current.Spec.MaxTemperatureCelsius = 60

		if err := w.parent.Client.Update(ctx, current); err != nil {
			return err
		}

		return apierrors.NewConflict(
			schema.GroupResource{
				Group:    "gpu.hymio.dev",
				Resource: "gpuhealthpolicies",
			},
			obj.GetName(),
			fmt.Errorf("injected concurrent Spec change"),
		)
	}

	return w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
}

// Test 2: An evaluation calculated with an old threshold
// must not be written into a newer policy Spec.
func TestDay2SpecChangeRejectsStaleEvaluation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

	base, scheme, req := newDay2TestClient(t)

	injected := &specChangingClientDay2{
		Client: base,
	}

	telemetryClient := newCountingTelemetryDay2(now)

	r := &GPUHealthPolicyReconciler{
		Client:    injected,
		Scheme:    scheme,
		Telemetry: telemetryClient,
		Now:       func() time.Time { return now },
	}

	result, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Requeue {
		t.Fatalf(
			"expected immediate Requeue after Spec change: %+v",
			result,
		)
	}

	if telemetryClient.calls != 1 {
		t.Fatalf(
			"FetchNode calls = %d, want 1",
			telemetryClient.calls,
		)
	}

	if injected.patchAttempts != 1 {
		t.Fatalf(
			"Status Patch attempts = %d, want 1",
			injected.patchAttempts,
		)
	}

	actual := &gpuv1alpha1.GPUHealthPolicy{}
	if err := base.Get(ctx, req.NamespacedName, actual); err != nil {
		t.Fatal(err)
	}

	if actual.Spec.MaxTemperatureCelsius != 60 {
		t.Fatalf(
			"threshold = %d, want 60",
			actual.Spec.MaxTemperatureCelsius,
		)
	}

	if len(actual.Status.Conditions) != 0 {
		t.Fatalf(
			"stale conditions were written: %+v",
			actual.Status.Conditions,
		)
	}

	t.Log("Concurrent Spec change detected; stale Status rejected.")
}
