package replicaset

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes ReplicaSet within a controller's
// reconciliation loop.
//
// It implements several component interfaces to integrate with the operator-component-framework:
//   - component.Resource: for basic identity and mutation behavior.
//   - concepts.Alive: for health and readiness tracking.
//   - concepts.Suspendable: for graceful scale-down or temporary deactivation.
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting information after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to data extractors on read-only reconciliation.
//
// This resource handles the lifecycle of a ReplicaSet, including initial creation,
// updates via feature mutations, and status monitoring.
type Resource struct {
	base *generic.WorkloadResource[*appsv1.ReplicaSet, *Mutator]
}

// Identity returns a unique identifier for the ReplicaSet in the format
// "apps/v1/ReplicaSet/<namespace>/<name>".
//
// This identifier is used by the framework's internal tracking and recording
// mechanisms to distinguish this specific ReplicaSet from other resources
// managed by the same component.
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a copy of the underlying Kubernetes ReplicaSet object.
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

// Mutate transforms the current state of a Kubernetes ReplicaSet into the desired state.
//
// The mutation process follows a specific order:
//  1. Feature Mutations: All registered feature-based mutations are applied,
//     allowing for granular, version-gated changes to the ReplicaSet.
//  2. Suspension: If the resource is in a suspending state, the suspension
//     logic (e.g., scaling to zero) is applied.
//
// This method is invoked by the framework during the "Update" phase of
// reconciliation. It ensures that the in-memory object reflects all
// configuration and feature requirements before it is sent to the API server.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates if the ReplicaSet has successfully reached its desired state.
//
// By default, it uses DefaultConvergingStatusHandler, which checks if the number of ReadyReplicas
// matches the desired replica count.
//
// The return value includes a descriptive status (Ready, Creating, Updating, or Scaling)
// and a human-readable reason, which are used to update the component's conditions.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.AliveStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus provides a health assessment of the ReplicaSet when it has not yet
// reached full readiness.
//
// By default, it uses DefaultGraceStatusHandler, which categorizes the current state into:
//   - GraceStatusDegraded: At least one replica is ready, but the desired count is not met.
//   - GraceStatusDown: No replicas are ready.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// DeleteOnSuspend determines whether the ReplicaSet should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns false, meaning
// the ReplicaSet is kept in the cluster but scaled to zero replicas.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the deactivation of the ReplicaSet.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behavior uses DefaultSuspendMutationHandler to scale the ReplicaSet
// to zero replicas.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which reports whether the
// ReplicaSet has successfully scaled down to zero replicas or is still in the
// process of doing so.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes registered data extraction functions to harvest information
// from the reconciled ReplicaSet.
//
// Data extractors are provided with a deep copy of the current ReplicaSet to
// prevent accidental mutations during the extraction process.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}

// RecordObservation stores the supplied object as the resource's most recently
// observed cluster state. The framework invokes this on read-only resources
// after fetching them so that registered data extractors observe the live
// object rather than the inert base used to construct the resource.
func (r *Resource) RecordObservation(observed client.Object) error {
	return r.base.RecordObservation(observed)
}

// GuardStatus evaluates the resource's guard precondition.
// If no guard was registered, the resource is unconditionally unblocked.
func (r *Resource) GuardStatus() (concepts.GuardStatusWithReason, error) {
	return r.base.GuardStatus()
}

// Preview renders the ReplicaSet as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}
