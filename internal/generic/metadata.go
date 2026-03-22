package generic

import "sigs.k8s.io/controller-runtime/pkg/client"

// PreserveServerManagedFields restores server-managed and shared-controller
// metadata fields from src onto dst.
//
// Call this after replacing an object via deep copy to ensure fields populated
// by the API server or other controllers are not lost. The preserved fields are:
//
// Server-managed (read-only): ResourceVersion, UID, Generation,
// CreationTimestamp, DeletionTimestamp, DeletionGracePeriodSeconds,
// ManagedFields, SelfLink.
//
// Shared across controllers: OwnerReferences, Finalizers.
func PreserveServerManagedFields(dst, src client.Object) {
	// Server-managed read-only fields
	dst.SetResourceVersion(src.GetResourceVersion())
	dst.SetUID(src.GetUID())
	dst.SetGeneration(src.GetGeneration())
	dst.SetCreationTimestamp(src.GetCreationTimestamp())
	dst.SetDeletionTimestamp(src.GetDeletionTimestamp())
	dst.SetDeletionGracePeriodSeconds(src.GetDeletionGracePeriodSeconds())
	dst.SetManagedFields(src.GetManagedFields())
	dst.SetSelfLink(src.GetSelfLink())

	// Shared across controllers
	dst.SetOwnerReferences(src.GetOwnerReferences())
	dst.SetFinalizers(src.GetFinalizers())
}
