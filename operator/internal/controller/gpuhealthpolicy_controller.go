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
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"

	gpuv1alpha1 "github.com/HYMIO-LLC/gpu-intelligence/operator/api/v1alpha1"
	"github.com/HYMIO-LLC/gpu-intelligence/operator/internal/telemetry"

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
	Scheme    *runtime.Scheme
	Telemetry telemetry.Client
	Now       func() time.Time
}

// +kubebuilder:rbac:groups=gpu.hymio.dev,resources=gpuhealthpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gpu.hymio.dev,resources=gpuhealthpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gpu.hymio.dev,resources=gpuhealthpolicies/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile processes a GPUHealthPolicy and records that its configuration
// has been accepted.
//
// It does not inspect GPU health, query metrics, or modify Nodes.
func (r *GPUHealthPolicyReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Load the policy before any external telemetry queries.
	policy := &gpuv1alpha1.GPUHealthPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	selector, err := metav1.LabelSelectorAsSelector(
		&policy.Spec.NodeSelector,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	nodes := &corev1.NodeList{}
	if err := r.List(
		ctx,
		nodes,
		client.MatchingLabelsSelector{Selector: selector},
	); err != nil {
		return ctrl.Result{}, err
	}

	// Perform network queries only once for this reconciliation.
	var assessment *policyTelemetryResult

	if r.Telemetry != nil {
		result := r.evaluatePolicyTelemetry(
			ctx,
			nodes.Items,
			float64(policy.Spec.MaxTemperatureCelsius),
		)
		assessment = &result
	}

	// A changed policy must be evaluated again using the new Spec.
	errPolicyChanged := errors.New(
		"GPUHealthPolicy changed during telemetry evaluation",
	)

	deleted := false

	err = retry.RetryOnConflict(
		retry.DefaultRetry,
		func() error {
			latest := &gpuv1alpha1.GPUHealthPolicy{}

			if err := r.Get(
				ctx,
				req.NamespacedName,
				latest,
			); err != nil {
				if apierrors.IsNotFound(err) {
					deleted = true
					return nil
				}
				return err
			}

			// Generation alone may not change in fake clients.
			// Compare Spec as an additional safety check.
			if latest.Generation != policy.Generation ||
				!equality.Semantic.DeepEqual(
					latest.Spec,
					policy.Spec,
				) {
				return errPolicyChanged
			}

			original := latest.DeepCopy()

			latest.Status.MatchedNodes = int32(len(nodes.Items))
			latest.Status.ObservedGeneration = latest.Generation

			if assessment != nil {
				meta.SetStatusCondition(
					&latest.Status.Conditions,
					metav1.Condition{
						Type:               "TelemetryAvailable",
						Status:             assessment.Available,
						Reason:             assessment.AvailableReason,
						Message:            assessment.Message,
						ObservedGeneration: latest.Generation,
					},
				)

				meta.SetStatusCondition(
					&latest.Status.Conditions,
					metav1.Condition{
						Type:               "GPUHealthy",
						Status:             assessment.Health,
						Reason:             assessment.HealthReason,
						Message:            assessment.Message,
						ObservedGeneration: latest.Generation,
					},
				)
			}

			meta.SetStatusCondition(
				&latest.Status.Conditions,
				metav1.Condition{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					Reason:             "PolicyAccepted",
					Message:            "GPUHealthPolicy configuration accepted; GPU health is reported separately when telemetry is configured",
					ObservedGeneration: latest.Generation,
				},
			)

			if equality.Semantic.DeepEqual(
				original.Status,
				latest.Status,
			) {
				return nil
			}

			return r.Status().Patch(
				ctx,
				latest,
				client.MergeFromWithOptions(
					original,
					client.MergeFromWithOptimisticLock{},
				),
			)
		},
	)

	if errors.Is(err, errPolicyChanged) {
		log.Info(
			"Policy changed during evaluation; scheduling fresh reconciliation",
			"name", req.Name,
			"namespace", req.Namespace,
		)
		return ctrl.Result{Requeue: true}, nil
	}

	if err != nil {
		log.Error(
			err,
			"Failed to reconcile GPUHealthPolicy",
			"name", req.Name,
			"namespace", req.Namespace,
		)
		return ctrl.Result{}, err
	}

	if deleted {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{
		RequeueAfter: 2 * time.Minute,
	}, nil
}

// SetupWithManager registers the GPUHealthPolicy controller.
func (r *GPUHealthPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gpuv1alpha1.GPUHealthPolicy{}).
		Named("gpuhealthpolicy").
		Complete(r)
}
