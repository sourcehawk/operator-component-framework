package pvc

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultFieldApplicator replaces current with a deep copy of desired, preserving
// server-managed metadata (ResourceVersion, UID, Generation, etc.), shared-controller
// fields (OwnerReferences, Finalizers), and immutable PVC spec fields on existing
// resources.
//
// Kubernetes marks several PVC spec fields as immutable after creation: accessModes,
// storageClassName, volumeMode, and volumeName. If the current object has a non-empty
// ResourceVersion (indicating it already exists in the cluster), those fields are
// restored from the current object after the deep copy.
//
// Use a custom field applicator via Builder.WithCustomFieldApplicator if you need
// different merge semantics.
func DefaultFieldApplicator(current, desired *corev1.PersistentVolumeClaim) error {
	isExisting := current.ResourceVersion != ""
	original := current.DeepCopy()

	savedAccessModes := current.Spec.AccessModes
	savedStorageClassName := current.Spec.StorageClassName
	savedVolumeMode := current.Spec.VolumeMode
	savedVolumeName := current.Spec.VolumeName

	*current = *desired.DeepCopy()
	generic.PreserveServerManagedFields(current, original)

	if isExisting {
		current.Spec.AccessModes = savedAccessModes
		current.Spec.StorageClassName = savedStorageClassName
		current.Spec.VolumeMode = savedVolumeMode
		current.Spec.VolumeName = savedVolumeName
	}

	return nil
}

// Resource is a high-level abstraction for managing a Kubernetes PersistentVolumeClaim
// within a controller's reconciliation loop.
//
// It implements the following component interfaces:
//   - component.Resource: for basic identity and mutation behaviour.
//   - component.Operational: for tracking whether the PVC is bound and operational.
//   - component.Suspendable: for controlled suspension (e.g. retaining the PVC while suspending consumers).
//   - component.DataExtractable: for exporting values after successful reconciliation.
//
// PVC resources follow the Integration lifecycle: they are operationally significant
// (a PVC must be Bound to be useful) and support suspension semantics.
type Resource struct {
	base *generic.IntegrationResource[*corev1.PersistentVolumeClaim, *Mutator]
}

// Identity returns a unique identifier for the PVC in the format
// "v1/PersistentVolumeClaim/<namespace>/<name>".
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a deep copy of the underlying Kubernetes PersistentVolumeClaim object.
//
// The returned object implements client.Object, making it compatible with
// controller-runtime's Client for Create, Update, and Patch operations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes PersistentVolumeClaim into the
// desired state.
//
// The mutation process follows this order:
//  1. Field application: the current object is updated to reflect the desired base state,
//     using either DefaultFieldApplicator or a custom applicator if one is configured.
//  2. Field application flavors: any registered flavors are applied in registration order.
//  3. Feature mutations: all registered feature-gated mutations are applied in order.
//  4. Suspension: if the resource is in a suspending state, the suspension logic is applied.
//
// This method is invoked by the framework during the Update phase of reconciliation.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates whether the PVC has reached its desired operational state.
//
// By default, it uses DefaultOperationalStatusHandler, which checks the PVC's phase
// to determine if it is Bound (operational), Pending, or Lost (failing).
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// DeleteOnSuspend determines whether the PVC should be deleted from the cluster
// when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns false to preserve
// the PVC and its underlying storage during suspension.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the deactivation of the PVC.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behaviour uses DefaultSuspendMutationHandler, which is a no-op
// since PVCs do not require modification when suspended — the consuming workload
// is responsible for scaling down.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which always reports
// Suspended since PVCs themselves have no runtime state to wind down.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes all registered data extractor functions against a deep copy
// of the reconciled PVC.
//
// This is called by the framework after successful reconciliation, allowing the
// component to read generated or updated values from the PVC.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
