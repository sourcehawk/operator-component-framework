package statefulset

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultFieldApplicator replaces current with a deep copy of desired while
// preserving server-managed metadata (ResourceVersion, UID, Generation, etc.),
// shared-controller fields (OwnerReferences, Finalizers), and the Status
// subresource from the original current object.
//
// It also preserves the existing VolumeClaimTemplates from the current object
// when it already exists on the server. spec.volumeClaimTemplates is immutable
// after creation in Kubernetes; attempting to update it will be rejected by the
// API server.
func DefaultFieldApplicator(current, desired *appsv1.StatefulSet) error {
	original := current.DeepCopy()

	vcts := current.Spec.VolumeClaimTemplates
	*current = *desired.DeepCopy()
	generic.PreserveServerManagedFields(current, original)
	generic.PreserveStatus(current, original)

	if original.ResourceVersion != "" {
		current.Spec.VolumeClaimTemplates = vcts
	}
	return nil
}

// Resource is a high-level abstraction for managing a Kubernetes StatefulSet within a controller's
// reconciliation loop.
//
// It implements several component interfaces to integrate with the operator-component-framework:
//   - component.Resource: for basic identity and mutation behavior.
//   - component.Alive: for health and readiness tracking.
//   - component.Suspendable: for graceful scale-down or temporary deactivation.
//   - component.DataExtractable: for exporting information after successful reconciliation.
//
// This resource handles the lifecycle of a StatefulSet, including initial creation,
// updates via feature mutations, and status monitoring.
type Resource struct {
	base *generic.WorkloadResource[*appsv1.StatefulSet, *Mutator]
}

// Identity returns a unique identifier for the StatefulSet in the format
// "apps/v1/StatefulSet/<namespace>/<name>".
//
// This identifier is used by the framework's internal tracking and recording
// mechanisms to distinguish this specific StatefulSet from other resources
// managed by the same component.
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a copy of the underlying Kubernetes StatefulSet object.
//
// The returned object implements the client.Object interface, making it
// fully compatible with controller-runtime's Client for operations like
// Get, Create, Update, and Patch.
//
// This method is called by the framework to obtain the current state
// of the resource before applying mutations.
func (r *Resource) Object() (client.Object, error) {
	return r.base.Object()
}

// Mutate transforms the current state of a Kubernetes StatefulSet into the desired state.
//
// The mutation process follows a specific order:
//  1. Core State: The current object is reset to the desired base state, or
//     modified via a custom field applicator if one is configured.
//  2. Feature Mutations: All registered feature-based mutations are applied,
//     allowing for granular, version-gated changes to the StatefulSet.
//  3. Suspension: If the resource is in a suspending state, the suspension
//     logic (e.g., scaling to zero) is applied.
//
// This method is invoked by the framework during the "Update" phase of
// reconciliation. It ensures that the in-memory object reflects all
// configuration and feature requirements before it is sent to the API server.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates if the StatefulSet has successfully reached its desired state.
//
// By default, it uses DefaultConvergingStatusHandler, which checks if the number of ReadyReplicas
// matches the desired replica count.
//
// The return value includes a descriptive status (Healthy, Creating, Updating, or Scaling)
// and a human-readable reason, which are used to update the component's conditions.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.AliveStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus provides a health assessment of the StatefulSet when it has not yet
// reached full readiness.
//
// By default, it uses DefaultGraceStatusHandler, which categorizes the current state into:
//   - GraceStatusDegraded: At least one replica is ready, but the desired count is not met.
//   - GraceStatusDown: No replicas are ready.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// DeleteOnSuspend determines whether the StatefulSet should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns false, meaning
// the StatefulSet is kept in the cluster but scaled to zero replicas.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the deactivation of the StatefulSet.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behavior uses DefaultSuspendMutationHandler to scale the StatefulSet
// to zero replicas, which effectively stops the application while keeping the
// Kubernetes resource intact.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which reports whether the
// StatefulSet has successfully scaled down to zero replicas or is still in the
// process of doing so.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes registered data extraction functions to harvest information
// from the reconciled StatefulSet.
//
// This is called by the framework after a successful reconciliation of the
// resource. It allows the component to export details that might be needed by
// other resources or higher-level controllers.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
