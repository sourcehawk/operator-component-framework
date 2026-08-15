package component

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			live := obj.DeepCopyObject().(client.Object)
			if getErr := rec.Client.Get(ctx, key, live); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					return nil
				}
				return getErr
			}
			refs := live.GetOwnerReferences()
			kept := make([]metav1.OwnerReference, 0, len(refs))
			removed = false
			for _, ref := range refs {
				if ref.UID == ownerUID {
					removed = true
					continue
				}
				kept = append(kept, ref)
			}
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
