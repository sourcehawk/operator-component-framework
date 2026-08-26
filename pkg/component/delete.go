package component

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// deleteConfig holds configuration for a deleteResources call.
type deleteConfig struct {
	reason string
}

// deleteOption is a functional option for deleteResources.
type deleteOption func(*deleteConfig)

// withDeletionReason sets the human-readable reason included in the deletion event.
func withDeletionReason(reason string) deleteOption {
	return func(cfg *deleteConfig) {
		cfg.reason = reason
	}
}

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
// An entry registered with BlockOnForeignController is left in place when the
// live object is controlled by another owner: the object is not this
// component's to delete, whichever path (a deletion flag, a disabled feature
// gate, suspension) asked for it. The skip is logged with the controlling owner.
//
// For each successful deletion, a "ResourceDeleted" event is recorded on the owner object.
// The opts parameter allows callers to customize the event message via functional options.
//
// Returns a joined error containing all encountered errors, or nil if all deletions
// were successful or resulted in "Not Found" errors.
func deleteResources(
	ctx context.Context, rec ReconcileContext, entries []reconcileEntry, opts ...deleteOption,
) error {
	cfg := deleteConfig{reason: "resource deletion flag"}
	for _, opt := range opts {
		opt(&cfg)
	}
	// we gather errors in order to delete as many resources as possible
	var errs []error

	for _, entry := range entries {
		resource := entry.Resource

		object, err := resource.Object()
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"failed to get resource %s's underlying object on deletion: %w",
				resource.Identity(), err,
			))
			continue
		}

		deleted, err := deleteEntry(ctx, rec, entry, object)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !deleted {
			continue
		}

		rec.EventRecorder.Eventf(
			rec.Owner, object, v1.EventTypeNormal, "ResourceDeleted", "Delete",
			"Resource %s deleted due to %s", resource.Identity(), cfg.reason,
		)
	}

	return errors.Join(errs...)
}

// deleteEntry deletes object, the desired object of entry's resource, and
// reports whether a deletion happened. An object that is already gone is not
// an error and reports false.
//
// For an entry registered with BlockOnForeignController the delete is bound to
// what was observed: the live object is read first (through the API reader
// when set), an absent object counts as deleted, an object another owner
// controls is left in place and logged, and an object observed as safe is
// deleted with its UID and resourceVersion as preconditions. An owner that
// claims the object between the read and the delete therefore makes the delete
// conflict instead of removing that owner's object; the error is returned so
// the next reconcile observes the object again. Unlike a lost apply race, a
// lost delete race is not repaired by the next reconcile, which is why the
// delete carries the precondition and the apply does not.
func deleteEntry(
	ctx context.Context, rec ReconcileContext, entry reconcileEntry, object client.Object,
) (bool, error) {
	resource := entry.Resource

	if !entry.Options.BlockOnForeignController {
		if err := rec.Client.Delete(ctx, object); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("failed to delete resource %s: %w", resource.Identity(), err)
		}
		return true, nil
	}

	live, controller, err := observeController(ctx, rec, resource)
	if err != nil {
		return false, err
	}
	if live == nil {
		return false, nil
	}
	if controller != nil {
		log.FromContext(ctx).Info(
			"skipping deletion of a resource another owner controls",
			"resource", resource.Identity(), "controller", controller.Kind+" "+controller.Name,
		)
		return false, nil
	}

	uid, resourceVersion := live.GetUID(), live.GetResourceVersion()
	err = rec.Client.Delete(ctx, live, client.Preconditions{UID: &uid, ResourceVersion: &resourceVersion})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf(
			"failed to delete resource %s as observed: %w", resource.Identity(), err,
		)
	}
	return true, nil
}
