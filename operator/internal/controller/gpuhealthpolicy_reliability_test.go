package controller

import (
	"context"
	"errors"
	"fmt"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// failingNodeListClient simulates a Kubernetes Node List API failure.
type failingNodeListClient struct {
	client.Client
	err error
}

func (c *failingNodeListClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if _, ok := list.(*corev1.NodeList); ok {
		return c.err
	}
	return c.Client.List(ctx, list, opts...)
}

// conflictStatusWriter injects one conflict into Status Patch.
type conflictStatusWriter struct {
	client.SubResourceWriter
	conflictsRemaining *int
	patchAttempts      *int
}

func (w *conflictStatusWriter) Patch(
	ctx context.Context,
	obj client.Object,
	patch client.Patch,
	opts ...client.SubResourcePatchOption,
) error {
	*w.patchAttempts++

	if *w.conflictsRemaining > 0 {
		*w.conflictsRemaining--

		return apierrors.NewConflict(
			schema.GroupResource{
				Group:    "gpu.hymio.dev",
				Resource: "gpuhealthpolicies",
			},
			obj.GetName(),
			fmt.Errorf("injected resourceVersion conflict"),
		)
	}

	return w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
}

type conflictClient struct {
	client.Client
	conflictsRemaining int
	patchAttempts      int
}

func (c *conflictClient) Status() client.SubResourceWriter {
	return &conflictStatusWriter{
		SubResourceWriter:  c.Client.Status(),
		conflictsRemaining: &c.conflictsRemaining,
		patchAttempts:      &c.patchAttempts,
	}
}

var _ = Describe("GPUHealthPolicy Reliability", func() {
	var (
		testCtx context.Context
		policy  *gpuv1alpha1.GPUHealthPolicy
		key     types.NamespacedName
	)

	BeforeEach(func() {
		testCtx = context.Background()

		policy = &gpuv1alpha1.GPUHealthPolicy{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "reliability-test-",
				Namespace:    "default",
			},
			Spec: gpuv1alpha1.GPUHealthPolicySpec{
				NodeSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"hymio.dev/reliability-test": "enabled",
					},
				},
				MaxTemperatureCelsius: 85,
				Action:                "AlertOnly",
			},
		}

		Expect(k8sClient.Create(testCtx, policy)).To(Succeed())

		key = types.NamespacedName{
			Name:      policy.Name,
			Namespace: policy.Namespace,
		}

		DeferCleanup(func() {
			current := &gpuv1alpha1.GPUHealthPolicy{}
			err := k8sClient.Get(testCtx, key, current)

			if apierrors.IsNotFound(err) {
				return
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(testCtx, current)).To(Succeed())
		})
	})

	It("returns an error when the Node List API fails", func() {
		injectedErr := errors.New("injected Node List failure")

		failingClient := &failingNodeListClient{
			Client: k8sClient,
			err:    injectedErr,
		}

		r := &GPUHealthPolicyReconciler{
			Client: failingClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := r.Reconcile(
			testCtx,
			reconcile.Request{NamespacedName: key},
		)

		Expect(err).To(MatchError(injectedErr))

		current := &gpuv1alpha1.GPUHealthPolicy{}
		Expect(k8sClient.Get(testCtx, key, current)).To(Succeed())

		// A failed Node query must not produce a misleading status update.
		Expect(current.Status.Conditions).To(BeEmpty())
		Expect(current.Status.ObservedGeneration).To(BeZero())
	})

	It("retries a conflict during Status Patch", func() {
		injectedClient := &conflictClient{
			Client:             k8sClient,
			conflictsRemaining: 1,
		}

		r := &GPUHealthPolicyReconciler{
			Client: injectedClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := r.Reconcile(
			testCtx,
			reconcile.Request{NamespacedName: key},
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(injectedClient.conflictsRemaining).To(Equal(0))
		Expect(injectedClient.patchAttempts).To(Equal(2))

		current := &gpuv1alpha1.GPUHealthPolicy{}
		Expect(k8sClient.Get(testCtx, key, current)).To(Succeed())

		Expect(current.Status.ObservedGeneration).
			To(Equal(current.Generation))

		ready := findReadyCondition(current.Status.Conditions)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("PolicyAccepted"))
	})
})
