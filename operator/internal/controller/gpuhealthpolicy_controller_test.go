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

var _ = Describe("GPUHealthPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		BeforeEach(func() {
			By("creating a valid GPUHealthPolicy resource")

			resource := &gpuv1alpha1.GPUHealthPolicy{
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

			err := k8sClient.Create(ctx, resource)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("cleaning up the GPUHealthPolicy resource")

			resource := &gpuv1alpha1.GPUHealthPolicy{}

			err := k8sClient.Get(
				ctx,
				typeNamespacedName,
				resource,
			)

			if apierrors.IsNotFound(err) {
				return
			}

			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, resource)
			Expect(err).To(Succeed())
		})

		It("should reconcile a valid resource without error", func() {
			By("reconciling the created GPUHealthPolicy")

			controllerReconciler := &GPUHealthPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(
				ctx,
				reconcile.Request{
					NamespacedName: typeNamespacedName,
				},
			)

			Expect(err).NotTo(HaveOccurred())
		})
	})
})
