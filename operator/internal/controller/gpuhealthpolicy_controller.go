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
	"time"

	corev1 "k8s.io/api/core/v1"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// GPUHealthPolicyReconciler reconciles a GPUHealthPolicy object.
type GPUHealthPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=gpu.hymio.dev,resources=gpuhealthpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gpu.hymio.dev,resources=gpuhealthpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gpu.hymio.dev,resources=gpuhealthpolicies/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list

// Reconcile processes a GPUHealthPolicy and records that its configuration
// has been accepted.
//
// It does not inspect GPU health, query metrics, or modify Nodes.
func (r *GPUHealthPolicyReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	policyFound := false

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		policy := &gpuv1alpha1.GPUHealthPolicy{}

		if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		// Convert the policy's Kubernetes LabelSelector.
		selector, err := metav1.LabelSelectorAsSelector(
			&policy.Spec.NodeSelector,
		)
		if err != nil {
			return err
		}

		// Query Nodes matching the label selector.
		nodes := &corev1.NodeList{}
		if err := r.List(
			ctx,
			nodes,
			client.MatchingLabelsSelector{Selector: selector},
		); err != nil {
			return err
		}

		policyFound = true

		original := policy.DeepCopy()

		policy.Status.MatchedNodes = int32(len(nodes.Items))
		policy.Status.ObservedGeneration = policy.Generation

		meta.SetStatusCondition(
			&policy.Status.Conditions,
			metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				Reason:             "PolicyAccepted",
				Message:            "GPUHealthPolicy configuration accepted; GPU health is not evaluated",
				ObservedGeneration: policy.Generation,
			},
		)

		// Avoid unnecessary API writes when nothing has changed.
		if equality.Semantic.DeepEqual(original.Status, policy.Status) {
			return nil
		}

		return r.Status().Patch(
			ctx,
			policy,
			client.MergeFromWithOptions(
				original,
				client.MergeFromWithOptimisticLock{},
			),
		)
	})

	if err != nil {
		log.Error(err, "Failed to reconcile GPUHealthPolicy",
			"name", req.Name,
			"namespace", req.Namespace,
		)
		return ctrl.Result{}, err
	}

	// Do not schedule periodic reconciliation for deleted policies.
	if !policyFound {
		return ctrl.Result{}, nil
	}

	// Periodically refresh matched Nodes even if no Policy event occurs.
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

// SetupWithManager registers the GPUHealthPolicy controller.
func (r *GPUHealthPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gpuv1alpha1.GPUHealthPolicy{}).
		Named("gpuhealthpolicy").
		Complete(r)
}
