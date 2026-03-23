package generic

import (
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

// PreserveStatus copies the Status field from src to dst using reflection.
//
// Most Kubernetes resource types have a Status subresource that is managed by
// the API server or status-writing controllers, not by spec-level reconciliation.
// Field applicators that replace the entire object (e.g. *current = *desired)
// inadvertently overwrite the live status. Call this function after such a
// replacement to restore the status from the original live object.
//
// If either object's underlying struct does not have a field named "Status",
// the call is a no-op.
func PreserveStatus[T client.Object](dst, src T) {
	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()

	dstStatus := dstVal.FieldByName("Status")
	srcStatus := srcVal.FieldByName("Status")

	if !dstStatus.IsValid() || !srcStatus.IsValid() {
		return
	}

	if !dstStatus.CanSet() {
		return
	}

	dstStatus.Set(srcStatus)
}
