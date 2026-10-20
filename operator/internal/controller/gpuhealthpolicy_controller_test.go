/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("GPUHealthPolicy Status Reconciliation", func() {
	const (
		resourceName      = "status-reconciliation-test"
		resourceNamespace = "default"
	)

	testCtx := context.Background()

	key := types.NamespacedName{
		Name:      resourceName,
		Namespace: resourceNamespace,
	}

	var reconciler *GPUHealthPolicyReconciler

	BeforeEach(func() {
		reconciler = &GPUHealthPolicyReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		policy := &gpuv1alpha1.GPUHealthPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: resourceNamespace,
			},
			Spec: gpuv1alpha1.GPUHealthPolicySpec{
				NodeSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"accelerator": "nvidia",
					},
				},
				MaxTemperatureCelsius: 85,
				Action:                "AlertOnly",
			},
		}

		Expect(k8sClient.Create(testCtx, policy)).To(Succeed())
	})

	AfterEach(func() {
		policy := &gpuv1alpha1.GPUHealthPolicy{}

		err := k8sClient.Get(testCtx, key, policy)
		if apierrors.IsNotFound(err) {
			return
		}

		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(testCtx, policy)).To(Succeed())
	})

	reconcilePolicy := func() {
		_, err := reconciler.Reconcile(
			testCtx,
			reconcile.Request{NamespacedName: key},
		)
		Expect(err).NotTo(HaveOccurred())
	}

	getPolicy := func() *gpuv1alpha1.GPUHealthPolicy {
		policy := &gpuv1alpha1.GPUHealthPolicy{}
		Expect(k8sClient.Get(testCtx, key, policy)).To(Succeed())
		return policy
	}

	It("sets Ready=True and the observed generation", func() {
		reconcilePolicy()

		policy := getPolicy()

		Expect(policy.Status.ObservedGeneration).
			To(Equal(policy.Generation))

		condition := findReadyCondition(policy.Status.Conditions)

		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal("PolicyAccepted"))
		Expect(condition.ObservedGeneration).
			To(Equal(policy.Generation))
	})

	It("does not update status on repeated reconciliation", func() {
		reconcilePolicy()

		first := getPolicy()
		resourceVersion := first.ResourceVersion
		firstTransitionTime := findReadyCondition(
			first.Status.Conditions,
		).LastTransitionTime

		reconcilePolicy()

		second := getPolicy()

		Expect(second.ResourceVersion).To(Equal(resourceVersion))
		Expect(findReadyCondition(second.Status.Conditions).LastTransitionTime).
			To(Equal(firstTransitionTime))
	})

	It("updates observedGeneration after a spec change", func() {
		reconcilePolicy()

		policy := getPolicy()
		originalGeneration := policy.Generation

		policy.Spec.MaxTemperatureCelsius = 90
		Expect(k8sClient.Update(testCtx, policy)).To(Succeed())

		reconcilePolicy()

		updated := getPolicy()

		Expect(updated.Generation).
			To(BeNumerically(">", originalGeneration))
		Expect(updated.Status.ObservedGeneration).
			To(Equal(updated.Generation))

		condition := findReadyCondition(updated.Status.Conditions)
		Expect(condition).NotTo(BeNil())
		Expect(condition.ObservedGeneration).
			To(Equal(updated.Generation))
	})

	It("handles a deleted resource without error", func() {
		policy := getPolicy()

		Expect(k8sClient.Delete(testCtx, policy)).To(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(
				testCtx,
				key,
				&gpuv1alpha1.GPUHealthPolicy{},
			)
			return apierrors.IsNotFound(err)
		}).Should(BeTrue())

		reconcilePolicy()
	})
})

func findReadyCondition(
	conditions []metav1.Condition,
) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == "Ready" {
			return &conditions[i]
		}
	}
	return nil
}
