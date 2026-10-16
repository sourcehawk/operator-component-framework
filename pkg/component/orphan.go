package component

import (
	"context"
	"errors"
	"fmt"
	"slices"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// orphanResources releases the given resources from the owner: for each live
// object it removes the owner's controller owner reference and persists the
// change, leaving the object in the cluster. It never applies desired state and
// never deletes.
//
// Ownership is removed with a fetch-and-update of the live object rather than a
// Server-Side Apply: a minimal apply would drop the spec fields the component's
// field manager owns and revert the object's content, whereas editing only
// metadata.ownerReferences on the fetched object preserves everything else.
//
// A missing object or an already-absent owner reference is a no-op. Errors are
// gathered so every resource is attempted; conflicts are retried.
func orphanResources(ctx context.Context, rec ReconcileContext, resources []Resource) error {
	var errs []error
	ownerUID := rec.Owner.GetUID()

	for _, resource := range resources {
		obj, err := resource.Object()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get resource %s's underlying object on orphan: %w", resource.Identity(), err))
			continue
		}
		key := client.ObjectKeyFromObject(obj)

		var (
			removed  bool
			orphaned client.Object
		)
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Each attempt reports only what it observed itself. Without this reset, an
			// attempt that conflicts after setting removed would leak that state into a
			// later attempt whose object has since been deleted, and the event would
			// claim an orphaning that never landed.
			removed, orphaned = false, nil

			live := obj.DeepCopyObject().(client.Object)
			if getErr := rec.Client.Get(ctx, key, live); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					return nil
				}
				return getErr
			}
			refs := live.GetOwnerReferences()
			kept := slices.DeleteFunc(slices.Clone(refs), referencesOwner(ownerUID))
			removed = len(kept) != len(refs)
			if !removed {
				return nil
			}
			live.SetOwnerReferences(kept)
			if updateErr := rec.Client.Update(ctx, live); updateErr != nil {
				return updateErr
			}
			orphaned = live
			return nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to orphan resource %s: %w", resource.Identity(), err))
			continue
		}
		if removed {
			rec.EventRecorder.Eventf(rec.Owner, orphaned, v1.EventTypeNormal, "ResourceOrphaned", "Orphan",
				"Resource %s orphaned: owner reference removed", resource.Identity())
		}
	}
	return errors.Join(errs...)
}

// ownedBy reports whether obj carries an owner reference to the owner with
// ownerUID, whether or not it is the controller reference.
func ownedBy(obj client.Object, ownerUID types.UID) bool {
	return slices.ContainsFunc(obj.GetOwnerReferences(), referencesOwner(ownerUID))
}

// referencesOwner matches an owner reference to the owner with ownerUID,
// whether or not it is the controller reference.
func referencesOwner(ownerUID types.UID) func(metav1.OwnerReference) bool {
	return func(ref metav1.OwnerReference) bool { return ref.UID == ownerUID }
}
