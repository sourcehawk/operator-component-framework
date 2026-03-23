package component

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type suspensionResults []concepts.SuspensionStatusWithReason

// summary returns the aggregate suspension status for all suspendable component resources.
// It uses a priority-based aggregation (PendingSuspension > Suspending > Suspended).
// The most "pending" status across all resources is selected as the component-level status.
func (s suspensionResults) summary() concepts.SuspensionStatusWithReason {
	var maxStatus concepts.SuspensionStatus
	var reasons []string

	for _, result := range s {
		if result.Status.Priority() > maxStatus.Priority() {
			maxStatus = result.Status
			reasons = []string{result.Reason}
		} else if result.Status.Priority() == maxStatus.Priority() && result.Reason != "" {
			reasons = append(reasons, result.Reason)
		}
	}

	if maxStatus == "" || maxStatus == concepts.SuspensionStatusSuspended {
		return concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspended,
			Reason: "All resources are suspended.",
		}
	}

	return concepts.SuspensionStatusWithReason{
		Status: maxStatus,
		Reason: strings.Join(reasons, "; "),
	}
}

// suspendResources orchestrates the suspension of all "creation" resources that implement Suspendable.
// It ensures that suspension mutations are applied and tracks the progress of each resource.
// If any resource fails to suspend, all encountered errors are joined and returned.
func suspendResources(
	ctx context.Context, rec ReconcileContext, resources []Resource, mapper meta.RESTMapper,
) ([]concepts.SuspensionStatusWithReason, error) {
	var results []concepts.SuspensionStatusWithReason
	var errs []error

	for _, resource := range resources {
		if suspendable, ok := resource.(concepts.Suspendable); ok {
			status, err := suspendResource(ctx, rec, resource, suspendable, mapper)
			if err != nil {
				// gather the errors to suspend as many resources as possible
				errs = append(errs, err)
			} else {
				results = append(results, status)
			}
		}
	}

	// if any error occurred, set an error condition and return without results
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// no errors occurred, return the suspension statuses
	return results, nil
}

// suspendResource handles the suspension process for a single resource:
//
// Stage 1: Short-circuit for absent delete-on-suspend resources
//   - If DeleteOnSuspend() is true and the object does not exist on the cluster,
//     returns SuspensionStatusSuspended immediately without creating the resource.
//     This prevents a create→delete churn loop on every reconcile while suspended.
//     The check runs before Suspend() to avoid queuing a mutation that will never be applied.
//
// Stage 2: Mutation
//   - Registers suspension-specific mutations via Suspend(), then applies them to the
//     resource's desired state.
//   - Persists these mutations to the cluster using createOrUpdateResources.
//     If the resource does not yet exist, it is created with suspension mutations already applied
//     (e.g., a Deployment is created with zero replicas). This is intentional: the resource is
//     immediately available in its suspended state, ready for when suspension ends.
//   - Checks if the resource has reached the Suspended state via SuspensionStatus().
//
// Stage 3: Optional Deletion
//   - If DeleteOnSuspend() is true AND the resource has reached SuspensionStatusSuspended,
//     the Kubernetes object is deleted from the cluster.
//   - Deletion is deferred until the Suspended state is reached to allow for graceful
//     shutdown or final state persistence (e.g., via finalizers or pre-stop hooks).
func suspendResource(
	ctx context.Context, rec ReconcileContext, resource Resource, suspendable concepts.Suspendable,
	mapper meta.RESTMapper,
) (concepts.SuspensionStatusWithReason, error) {
	// Get the object if possible
	object, err := resource.Object()
	if err != nil {
		return concepts.SuspensionStatusWithReason{}, fmt.Errorf("failed to get object on suspension: %w", err)
	}

	// Short-circuit: if the resource should be deleted on suspend and already doesn't exist,
	// skip CreateOrUpdate to avoid a create->delete churn loop on every reconcile.
	// This check runs before Suspend() to avoid queuing a mutation that will never be applied.
	if suspendable.DeleteOnSuspend() {
		existing, ok := object.DeepCopyObject().(client.Object)
		if !ok {
			return concepts.SuspensionStatusWithReason{}, fmt.Errorf(
				"failed to deep copy object of type %T", object,
			)
		}
		err := rec.Client.Get(ctx, client.ObjectKeyFromObject(object), existing)
		if apierrors.IsNotFound(err) {
			return concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: fmt.Sprintf("Resource %s already deleted.", resource.Identity()),
			}, nil
		}
		if err != nil {
			return concepts.SuspensionStatusWithReason{}, fmt.Errorf(
				"failed to check existence of resource %s on suspension: %w", resource.Identity(), err,
			)
		}
	}

	// Create suspension mutation on resource (if any)
	if err := suspendable.Suspend(); err != nil {
		return concepts.SuspensionStatusWithReason{}, fmt.Errorf("failed to suspend resource: %w", err)
	}

	// Apply suspension mutation (if any)
	_, err = createOrUpdateResources(ctx, rec, []Resource{resource}, mapper)
	if err != nil {
		return concepts.SuspensionStatusWithReason{}, fmt.Errorf(
			"failed to create or update resource %s on suspension: %w", resource.Identity(), err,
		)
	}

	// Get the suspension status of the resource
	suspension, err := suspendable.SuspensionStatus()
	if err != nil {
		return concepts.SuspensionStatusWithReason{}, fmt.Errorf(
			"failed to retrieve suspension status for resource %s: %w", resource.Identity(), err,
		)
	}

	// Do not proceed with potential deletion unless suspension state is in "completed" state
	if suspension.Status != concepts.SuspensionStatusSuspended {
		return suspension, nil
	}

	// Delete resource if it should be deleted
	if suspendable.DeleteOnSuspend() {
		if err := rec.Client.Delete(ctx, object); err != nil {
			if !apierrors.IsNotFound(err) {
				return suspension, fmt.Errorf("failed to delete resource: %w", err)
			}
		}

		rec.Recorder.Eventf(
			rec.Owner, v1.EventTypeNormal, "ResourceDeleted",
			fmt.Sprintf("Resource %s deleted on suspension", resource.Identity()),
		)

		return suspension, nil
	}

	return suspension, nil
}
