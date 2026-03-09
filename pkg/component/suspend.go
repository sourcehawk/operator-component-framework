package component

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type suspensionResults []SuspensionStatusWithReason

// summary returns the aggregate suspension status for all suspendable component resources.
// It uses a priority-based aggregation (PendingSuspension > Suspending > Suspended).
// The most "pending" status across all resources is selected as the component-level status.
func (s suspensionResults) summary() SuspensionStatusWithReason {
	var maxStatus SuspensionStatus
	var reasons []string

	for _, result := range s {
		if result.Status.level() > maxStatus.level() {
			maxStatus = result.Status
			reasons = []string{result.Reason}
		} else if result.Status.level() == maxStatus.level() && result.Reason != "" {
			reasons = append(reasons, result.Reason)
		}
	}

	if maxStatus == "" || maxStatus == SuspensionStatusSuspended {
		return SuspensionStatusWithReason{
			Status: SuspensionStatusSuspended,
			Reason: "All resources are suspended.",
		}
	}

	return SuspensionStatusWithReason{
		Status: maxStatus,
		Reason: strings.Join(reasons, "; "),
	}
}

// suspendResources orchestrates the suspension of all "creation" resources that implement Suspendable.
// It ensures that suspension mutations are applied and tracks the progress of each resource.
// If any resource fails to suspend, all encountered errors are joined and returned.
func suspendResources(
	ctx context.Context, rec ReconcileContext, resources []Resource,
) ([]SuspensionStatusWithReason, error) {
	var results []SuspensionStatusWithReason
	var errs []error

	for _, resource := range resources {
		if suspendable, ok := resource.(Suspendable); ok {
			status, err := suspendResource(ctx, rec, resource, suspendable)
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

// suspendResource handles the two-stage suspension process for a single resource:
//
// Stage 1: Mutation
//   - Applies suspension-specific mutations to the resource's desired state via Suspend().
//   - Persists these mutations to the cluster using createOrUpdateResources.
//   - Checks if the resource has reached the Suspended state via SuspensionStatus().
//
// Stage 2: Optional Deletion
//   - If DeleteOnSuspend() is true AND the resource has reached SuspensionStatusSuspended,
//     the Kubernetes object is deleted from the cluster.
//   - Deletion is deferred until the Suspended state is reached to allow for graceful
//     shutdown or final state persistence (e.g., via finalizers or pre-stop hooks).
func suspendResource(
	ctx context.Context, rec ReconcileContext, resource Resource, suspendable Suspendable,
) (SuspensionStatusWithReason, error) {
	// Create suspension mutation on resource (if any)
	if err := suspendable.Suspend(); err != nil {
		return SuspensionStatusWithReason{}, fmt.Errorf("failed to suspend resource: %w", err)
	}

	// Get the object if possible
	object, err := resource.DesiredDefaultObject()
	if err != nil {
		return SuspensionStatusWithReason{}, fmt.Errorf("failed to get object on suspension: %w", err)
	}

	// Apply suspension mutation (if any)
	_, err = createOrUpdateResources(ctx, rec, []Resource{resource})
	if err != nil {
		return SuspensionStatusWithReason{}, fmt.Errorf(
			"failed to create or update resource %s on suspension: %w", resource.Identity(), err,
		)
	}

	// Get the suspension status of the resource
	suspension, err := suspendable.SuspensionStatus()
	if err != nil {
		return SuspensionStatusWithReason{}, fmt.Errorf(
			"failed to retrieve suspension status for resource %s: %w", resource.Identity(), err,
		)
	}

	// Do not proceed with potential deletion unless suspension state is in "completed" state
	if suspension.Status != SuspensionStatusSuspended {
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
