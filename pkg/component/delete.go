package component

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// deleteResources removes the specified Kubernetes resources from the cluster.
// It is typically called during component reconciliation when certain resources
// are no longer needed or are marked for deletion by the component's state.
//
// The function iterates through all provided resources and attempts to delete each one.
// If a resource is not found in the cluster (apierrors.IsNotFound), it is treated as
// a success, as the desired state (deletion) is already achieved.
//
// To ensure as many resources as possible are processed, the function collects
// any errors encountered (e.g., failure to retrieve the underlying object or
// failure of the delete operation) and continues with the remaining resources.
//
// For each successful deletion, a "ResourceDeleted" event is recorded on the owner object.
//
// Returns a joined error containing all encountered errors, or nil if all deletions
// were successful or resulted in "Not Found" errors.
func deleteResources(
	ctx context.Context, rec ReconcileContext, resources []Resource,
) error {
	// we gather errors in order to delete as many resources as possible
	var errs []error

	for _, resource := range resources {
		object, err := resource.DesiredDefaultObject()
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"failed to get resource %s's underlying object on deletion: %w",
				resource.Identity(), err,
			))
			continue
		}

		if err := rec.Client.Delete(ctx, object); err != nil {
			if !apierrors.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("failed to delete resource %s: %w", resource.Identity(), err))
			}
			continue
		}

		rec.Recorder.Eventf(
			rec.Owner, v1.EventTypeNormal, "ResourceDeleted",
			fmt.Sprintf("Resource %s deleted due to resource deletion flag", resource.Identity()),
		)
	}

	return errors.Join(errs...)
}
