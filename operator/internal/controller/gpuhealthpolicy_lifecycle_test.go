package controller

import (
	"context"
	"time"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("GPUHealthPolicy Node Lifecycle", func() {
	It("refreshes matched nodes after Node changes", func() {
		testCtx := context.Background()
		testID := string(uuid.NewUUID())
		labelKey := "hymio.dev/lifecycle-id"

		r := &GPUHealthPolicyReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		policy := &gpuv1alpha1.GPUHealthPolicy{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "lifecycle-policy-",
				Namespace:    "default",
			},
			Spec: gpuv1alpha1.GPUHealthPolicySpec{
				NodeSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						labelKey: testID,
					},
				},
				MaxTemperatureCelsius: 85,
				Action:                "AlertOnly",
			},
		}

		Expect(k8sClient.Create(testCtx, policy)).To(Succeed())
		DeferCleanup(func() {
			current := &gpuv1alpha1.GPUHealthPolicy{}
			err := k8sClient.Get(testCtx, types.NamespacedName{
				Name:      policy.Name,
				Namespace: policy.Namespace,
			}, current)

			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(testCtx, current)).To(Succeed())
		})

		key := types.NamespacedName{
			Name:      policy.Name,
			Namespace: policy.Namespace,
		}

		runReconcile := func() {
			result, err := r.Reconcile(
				testCtx,
				reconcile.Request{NamespacedName: key},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(2 * time.Minute))
		}

		getMatchedCount := func() int32 {
			current := &gpuv1alpha1.GPUHealthPolicy{}
			Expect(k8sClient.Get(testCtx, key, current)).To(Succeed())
			return current.Status.MatchedNodes
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "lifecycle-node-",
				Labels: map[string]string{
					labelKey: testID,
				},
			},
		}

		By("starting with zero matching Nodes")
		runReconcile()
		Expect(getMatchedCount()).To(Equal(int32(0)))

		By("detecting a newly created matching Node")
		Expect(k8sClient.Create(testCtx, node)).To(Succeed())
		DeferCleanup(func() {
			current := &corev1.Node{}
			err := k8sClient.Get(
				testCtx,
				types.NamespacedName{Name: node.Name},
				current,
			)
			if err == nil {
				Expect(k8sClient.Delete(testCtx, current)).To(Succeed())
			}
		})

		runReconcile()
		Expect(getMatchedCount()).To(Equal(int32(1)))

		By("detecting a Node label change")
		currentNode := &corev1.Node{}
		Expect(k8sClient.Get(
			testCtx,
			types.NamespacedName{Name: node.Name},
			currentNode,
		)).To(Succeed())

		currentNode.Labels[labelKey] = "different"
		Expect(k8sClient.Update(testCtx, currentNode)).To(Succeed())

		runReconcile()
		Expect(getMatchedCount()).To(Equal(int32(0)))

		By("detecting a matching label restored")
		Expect(k8sClient.Get(
			testCtx,
			types.NamespacedName{Name: node.Name},
			currentNode,
		)).To(Succeed())

		currentNode.Labels[labelKey] = testID
		Expect(k8sClient.Update(testCtx, currentNode)).To(Succeed())

		runReconcile()
		Expect(getMatchedCount()).To(Equal(int32(1)))

		By("detecting Node deletion")
		Expect(k8sClient.Delete(testCtx, currentNode)).To(Succeed())

		runReconcile()
		Expect(getMatchedCount()).To(Equal(int32(0)))

		By("not requeueing deleted policies")
		currentPolicy := &gpuv1alpha1.GPUHealthPolicy{}
		Expect(k8sClient.Get(testCtx, key, currentPolicy)).
			To(Succeed())
		Expect(k8sClient.Delete(testCtx, currentPolicy)).
			To(Succeed())

		result, err := r.Reconcile(
			testCtx,
			reconcile.Request{NamespacedName: key},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
	})
})
