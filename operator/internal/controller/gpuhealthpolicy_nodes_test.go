package controller

import (
	"context"
	"fmt"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("GPUHealthPolicy Node Selection", func() {
	It("counts matching Nodes and updates status idempotently", func() {
		testCtx := context.Background()
		testID := string(uuid.NewUUID())
		selectorKey := "hymio.dev/test-id"

		reconciler := &GPUHealthPolicyReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		policy := &gpuv1alpha1.GPUHealthPolicy{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "matched-nodes-test-",
				Namespace:    "default",
			},
			Spec: gpuv1alpha1.GPUHealthPolicySpec{
				NodeSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						selectorKey: testID,
					},
				},
				MaxTemperatureCelsius: 85,
				Action:                "AlertOnly",
			},
		}

		Expect(k8sClient.Create(testCtx, policy)).To(Succeed())

		DeferCleanup(func() {
			Expect(k8sClient.Delete(testCtx, policy)).To(Succeed())
		})

		key := types.NamespacedName{
			Name:      policy.Name,
			Namespace: policy.Namespace,
		}

		runReconcile := func() {
			_, err := reconciler.Reconcile(
				testCtx,
				reconcile.Request{NamespacedName: key},
			)
			Expect(err).NotTo(HaveOccurred())
		}

		getPolicy := func() *gpuv1alpha1.GPUHealthPolicy {
			current := &gpuv1alpha1.GPUHealthPolicy{}
			Expect(k8sClient.Get(testCtx, key, current)).To(Succeed())
			return current
		}

		createNode := func(accelerator string, id string) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "matched-node-test-",
					Labels: map[string]string{
						selectorKey:   id,
						"accelerator": accelerator,
					},
				},
			}

			Expect(k8sClient.Create(testCtx, node)).To(Succeed())

			DeferCleanup(func() {
				Expect(k8sClient.Delete(testCtx, node)).To(Succeed())
			})
		}

		By("returning zero when no Nodes match")
		runReconcile()
		Expect(getPolicy().Status.MatchedNodes).To(Equal(int32(0)))

		By("counting one matching Node")
		createNode("nvidia", testID)
		runReconcile()
		Expect(getPolicy().Status.MatchedNodes).To(Equal(int32(1)))

		By("counting two matching Nodes")
		createNode("amd", testID)
		runReconcile()
		Expect(getPolicy().Status.MatchedNodes).To(Equal(int32(2)))

		By("excluding Nodes with different labels")
		createNode("nvidia", string(uuid.NewUUID()))
		runReconcile()
		Expect(getPolicy().Status.MatchedNodes).To(Equal(int32(2)))

		By("supporting matchExpressions")
		current := getPolicy()
		current.Spec.NodeSelector = metav1.LabelSelector{
			MatchLabels: map[string]string{
				selectorKey: testID,
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "accelerator",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"nvidia"},
				},
			},
		}

		Expect(k8sClient.Update(testCtx, current)).To(Succeed())
		runReconcile()

		updated := getPolicy()
		Expect(updated.Status.MatchedNodes).To(Equal(int32(1)))
		Expect(updated.Status.ObservedGeneration).
			To(Equal(updated.Generation))

		By("avoiding unnecessary writes on repeated reconciliation")
		resourceVersion := updated.ResourceVersion

		runReconcile()

		finalPolicy := getPolicy()
		Expect(finalPolicy.ResourceVersion).
			To(Equal(resourceVersion))

		fmt.Fprintf(
			GinkgoWriter,
			"Node selector reconciliation passed for policy %s\n",
			policy.Name,
		)
	})
})
