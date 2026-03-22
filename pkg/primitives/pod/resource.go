package pod

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultFieldApplicator handles the immutable nature of pod specs.
//
// For new pods (empty ResourceVersion), the entire desired state is applied.
// For existing pods, only metadata (labels and annotations) is propagated
// because pod spec fields are largely immutable after creation.
func DefaultFieldApplicator(current, desired *corev1.Pod) error {
	if current.ResourceVersion == "" {
		*current = *desired.DeepCopy()
		return nil
	}
	// Pod spec is largely immutable; only propagate metadata changes
	current.Labels = desired.Labels
	current.Annotations = desired.Annotations
	return nil
}

// Resource is a high-level abstraction for managing a Kubernetes Pod within a controller's
// reconciliation loop.
//
// It implements several component interfaces to integrate with the operator-component-framework:
//   - component.Resource: for basic identity and mutation behavior.
//   - component.Alive: for health and readiness tracking.
//   - component.Suspendable: for deletion-based deactivation.
//   - component.DataExtractable: for exporting information after successful reconciliation.
//
// This resource handles the lifecycle of a Pod, including initial creation,
// updates via feature mutations, and status monitoring.
type Resource struct {
	base *generic.WorkloadResource[*corev1.Pod, *Mutator]
}

// Identity returns a unique identifier for the Pod in the format
// "v1/Pod/<namespace>/<name>".
//
// This identifier is used by the framework's internal tracking and recording
// mechanisms to distinguish this specific Pod from other resources
// managed by the same component.
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a copy of the underlying Kubernetes Pod object.
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

// Mutate transforms the current state of a Kubernetes Pod into the desired state.
//
// The mutation process follows a specific order:
//  1. Core State: The current object is reset to the desired base state, or
//     modified via a custom customFieldApplicator if one is configured.
//  2. Feature Mutations: All registered feature-based mutations are applied,
//     allowing for granular, version-gated changes to the Pod.
//  3. Suspension: If the resource is in a suspending state, the suspension
//     logic is applied.
//
// This method is invoked by the framework during the "Update" phase of
// reconciliation. It ensures that the in-memory object reflects all
// configuration and feature requirements before it is sent to the API server.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates if the Pod has successfully reached its desired state.
//
// By default, it uses DefaultConvergingStatusHandler, which checks the Pod's phase
// and container statuses to determine health.
//
// The return value includes a descriptive status (Healthy, Creating, Updating, or Failing)
// and a human-readable reason, which are used to update the component's conditions.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.AliveStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus provides a health assessment of the Pod when it has not yet
// reached full readiness.
//
// By default, it uses DefaultGraceStatusHandler, which categorizes the current state into:
//   - GraceStatusDegraded: Pod is Running but not all containers are Ready.
//   - GraceStatusDown: Pod phase is not Running.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// DeleteOnSuspend determines whether the Pod should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns true because
// pods cannot be paused — they must be deleted when suspended.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the deactivation of the Pod.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behavior uses DefaultSuspendMutationHandler, which is a no-op
// because pods are deleted on suspend rather than mutated.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which always reports
// Suspended because pod suspension is handled by deletion.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes registered data extraction functions to harvest information
// from the reconciled Pod.
//
// This is called by the framework after a successful reconciliation of the
// resource. It allows the component to export details (like pod IP, node name,
// or status fields) that might be needed by other resources or higher-level
// controllers.
//
// Data extractors are provided with a deep copy of the current Pod to
// prevent accidental mutations during the extraction process.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
