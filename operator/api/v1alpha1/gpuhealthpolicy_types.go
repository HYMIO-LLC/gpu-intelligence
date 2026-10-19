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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GPUHealthPolicySpec defines the desired configuration of a GPUHealthPolicy.
type GPUHealthPolicySpec struct {
	// NodeSelector identifies the Kubernetes Nodes to which this
	// GPU health policy applies.
	//
	// An empty selector matches all Nodes. Users should specify
	// appropriate labels to target GPU Nodes.
	//
	// +kubebuilder:validation:Required
	NodeSelector metav1.LabelSelector `json:"nodeSelector"`

	// MaxTemperatureCelsius defines the configured maximum GPU
	// temperature threshold in degrees Celsius.
	//
	// This field is reserved for future GPU health evaluation.
	// The current API does not perform temperature monitoring.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=120
	// +kubebuilder:validation:Required
	MaxTemperatureCelsius int32 `json:"maxTemperatureCelsius"`

	// Action specifies the configured policy action.
	//
	// In v0.1, only AlertOnly is supported.
	// No automatic node remediation is performed.
	//
	// +kubebuilder:validation:Enum=AlertOnly
	// +kubebuilder:validation:Required
	Action string `json:"action"`
}

// GPUHealthPolicyStatus defines the observed state of a GPUHealthPolicy.
type GPUHealthPolicyStatus struct {
	// ObservedGeneration is the most recent generation
	// processed by the GPUHealthPolicy controller.
	//
	// It does not indicate that GPU health has been evaluated.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// MatchedNodes is the number of Kubernetes Nodes matching
	// the policy's node selector during the last reconciliation.
	//
	// It does not represent the number of healthy GPU Nodes.
	//
	// +optional
	MatchedNodes int32 `json:"matchedNodes,omitempty"`

	// Conditions represent the current processing state of
	// the GPUHealthPolicy resource.
	//
	// A Ready condition indicates whether the policy has been
	// accepted and processed by the controller.
	// It does not indicate actual GPU health.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GPUHealthPolicy is the Schema for the gpuhealthpolicies API.
type GPUHealthPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// Metadata contains standard Kubernetes resource metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// Spec defines the desired configuration of the GPUHealthPolicy.
	// +required
	Spec GPUHealthPolicySpec `json:"spec"`

	// Status defines the observed state of the GPUHealthPolicy.
	// +optional
	Status GPUHealthPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// GPUHealthPolicyList contains a list of GPUHealthPolicy resources.
type GPUHealthPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

	Items []GPUHealthPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(
			SchemeGroupVersion,
			&GPUHealthPolicy{},
			&GPUHealthPolicyList{},
		)
		return nil
	})
}
