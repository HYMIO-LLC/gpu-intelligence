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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("GPUHealthPolicy CRD Validation", func() {
	var (
		testCtx context.Context
		policy  *unstructured.Unstructured
	)

	BeforeEach(func() {
		testCtx = context.Background()

		policy = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gpu.hymio.dev/v1alpha1",
				"kind":       "GPUHealthPolicy",
				"metadata": map[string]interface{}{
					"namespace": "default",
					"name": fmt.Sprintf(
						"validation-%d",
						GinkgoRandomSeed()+int64(GinkgoParallelProcess()),
					),
				},
				"spec": map[string]interface{}{
					"nodeSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"accelerator": "nvidia",
						},
					},
					"maxTemperatureCelsius": int64(85),
					"action":                "AlertOnly",
				},
			},
		}

		policy.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "gpu.hymio.dev",
			Version: "v1alpha1",
			Kind:    "GPUHealthPolicy",
		})
	})

	AfterEach(func() {
		if policy == nil {
			return
		}

		err := k8sClient.Delete(testCtx, policy)
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("accepts a valid GPUHealthPolicy", func() {
		Expect(k8sClient.Create(testCtx, policy)).To(Succeed())
	})

	DescribeTable("rejects invalid policy specifications",
		func(modify func(map[string]interface{}), expectedField string) {
			spec := policy.Object["spec"].(map[string]interface{})
			modify(spec)

			err := k8sClient.Create(testCtx, policy)

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring(expectedField))
		},

		Entry("rejects temperature below minimum",
			func(spec map[string]interface{}) {
				spec["maxTemperatureCelsius"] = int64(0)
			},
			"spec.maxTemperatureCelsius",
		),

		Entry("rejects temperature above maximum",
			func(spec map[string]interface{}) {
				spec["maxTemperatureCelsius"] = int64(121)
			},
			"spec.maxTemperatureCelsius",
		),

		Entry("rejects unsupported action",
			func(spec map[string]interface{}) {
				spec["action"] = "Reboot"
			},
			"spec.action",
		),

		Entry("rejects missing action",
			func(spec map[string]interface{}) {
				delete(spec, "action")
			},
			"spec.action",
		),

		Entry("rejects missing temperature",
			func(spec map[string]interface{}) {
				delete(spec, "maxTemperatureCelsius")
			},
			"spec.maxTemperatureCelsius",
		),

		Entry("rejects missing node selector",
			func(spec map[string]interface{}) {
				delete(spec, "nodeSelector")
			},
			"spec.nodeSelector",
		),
	)

	It("accepts valid temperature boundaries", func() {
		spec := policy.Object["spec"].(map[string]interface{})

		spec["maxTemperatureCelsius"] = int64(1)
		Expect(k8sClient.Create(testCtx, policy)).To(Succeed())

		Expect(k8sClient.Delete(testCtx, policy)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &unstructured.Unstructured{}
			current.SetGroupVersionKind(policy.GroupVersionKind())

			err := k8sClient.Get(
				testCtx,
				metav1ObjectKey(policy),
				current,
			)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}).Should(Succeed())

		policy.SetResourceVersion("")
		policy.SetUID("")
		spec["maxTemperatureCelsius"] = int64(120)

		Expect(k8sClient.Create(testCtx, policy)).To(Succeed())
	})
})

func metav1ObjectKey(
	obj *unstructured.Unstructured,
) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}
