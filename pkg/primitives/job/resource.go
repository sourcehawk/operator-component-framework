package job

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes Job within a controller's
// reconciliation loop.
//
// It implements several component interfaces to integrate with the operator-component-framework:
//   - component.Resource: for basic identity and mutation behavior.
//   - concepts.Completable: for run-to-completion tracking.
//   - concepts.Suspendable: for controlled deactivation (suspend or delete).
//   - concepts.Guardable: for conditional reconciliation based on a guard precondition.
//   - concepts.DataExtractable: for exporting information after successful reconciliation.
//   - concepts.ObservationRecorder: for surfacing live cluster state to declared data extractions on read-only reconciliation.
//
// This resource handles the lifecycle of a Job, including initial creation,
// updates via feature mutations, and completion status monitoring.
type Resource struct {
	base *generic.TaskResource[*batchv1.Job, *Mutator]
}

// Identity returns a unique identifier for the Job in the format
// "batch/v1/Job/<namespace>/<name>".
//
// This identifier is used by the framework's internal tracking and recording
// mechanisms to distinguish this specific Job from other resources
// managed by the same component.
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// MetricsIdentifier returns the identifier set with
// Builder.WithMetricsIdentifier, or an empty string when none was set, in which
// case the framework labels the resource with its lowercased kind. It satisfies
// concepts.MetricsIdentifiable.
func (r *Resource) MetricsIdentifier() string {
	return r.base.MetricsIdentifier()
}

// Object returns a copy of the underlying Kubernetes Job object.
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

// Mutate transforms the current state of a Kubernetes Job into the desired state.
//
// The mutation process follows a specific order:
//  1. Core State: The desired base state is applied to the current object.
//  2. Feature Mutations: All registered feature-based mutations are applied,
//     allowing for granular, version-gated changes to the Job.
//  3. Suspension: If the resource is in a suspending state, the suspension
//     logic (e.g., setting suspend=true) is applied.
//
// This method is invoked by the framework during the "Update" phase of
// reconciliation. It ensures that the in-memory object reflects all
// configuration and feature requirements before it is sent to the API server.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates if the Job has completed, is still running, or has failed.
//
// By default, it uses DefaultConvergingStatusHandler, which checks the Job's status
// conditions for Complete or Failed.
//
// The return value includes a descriptive status (concepts.CompletionStatusCompleted,
// concepts.CompletionStatusRunning, concepts.CompletionStatusPending, or
// concepts.CompletionStatusFailing) and a human-readable reason, which are used to
// update the component's conditions.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.CompletionStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// DeleteOnSuspend determines whether the Job should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns true, meaning
// the Job is deleted during suspension. Jobs cannot be meaningfully scaled to zero
// like Deployments, so deletion is the standard approach.
//
// A custom decision handler can be registered via the Builder to change this
// behavior based on the current state of the Job.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the deactivation of the Job.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behavior uses DefaultSuspendMutationHandler to set the Job's
// Suspend field to true, which prevents new pods from being created.
//
// This is typically called by the framework when a component's .spec.suspended
// field is set to true.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which reports whether the
// Job has been successfully suspended by checking if the Suspend field is true
// and there are no active pods. The framework uses this to determine when the
// component has reached a fully suspended state.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes registered data extraction functions to harvest information
// from the reconciled Job.
//
// This is called by the framework after a successful reconciliation of the
// resource. It allows the component to export details (like completion status
// or generated names) that might be needed by other resources or higher-level
// controllers.
//
// Declared data extractions are provided with a deep copy of the current Job to
// prevent accidental mutations during the extraction process.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}

// ProducedData returns the cells this Job declares extractions into.
// It satisfies concepts.DataProducer for component topology validation and
// introspection.
func (r *Resource) ProducedData() []concepts.DataCell {
	return r.base.ProducedData()
}

// ConsumedData returns the Job's declared data reads. It satisfies
// concepts.DataConsumer for component topology validation and introspection.
func (r *Resource) ConsumedData() []concepts.DataConsumption {
	return r.base.ConsumedData()
}

// RecordObservation stores the supplied object as the resource's most recently
// observed cluster state. The framework invokes this on read-only resources
// after fetching them so that declared data extractions observe the live
// object rather than the inert base used to construct the resource.
func (r *Resource) RecordObservation(observed client.Object) error {
	return r.base.RecordObservation(observed)
}

// GuardStatus evaluates the resource's guard precondition.
// If no guard was registered, the resource is unconditionally unblocked.
func (r *Resource) GuardStatus() (concepts.GuardStatusWithReason, error) {
	return r.base.GuardStatus()
}

// Preview renders the Job as a client.Object with feature mutations applied,
// without modifying the resource's internal state. It satisfies the component's
// Previewable capability so the component can assemble a cluster-free preview.
//
// Suspension mutations are not applied; the preview reflects content state only.
// Callers needing the concrete type can type-assert the returned object.
func (r *Resource) Preview() (client.Object, error) {
	return r.base.Preview()
}

// RegisteredMutations returns the deduplicated Names of every mutation registered
// on the Job, independent of version. It satisfies concepts.MutationInspector
// so the resource can be introspected for version-matrix golden generation.
func (r *Resource) RegisteredMutations() []string {
	return r.base.RegisteredMutations()
}

// FiringSet returns the Names of registered mutations whose gate is enabled for the
// version the Job was built at. It satisfies concepts.MutationInspector.
func (r *Resource) FiringSet() ([]string, error) {
	return r.base.FiringSet()
}

var _ concepts.MutationInspector = (*Resource)(nil)
var _ concepts.DataProducer = (*Resource)(nil)
var _ concepts.DataConsumer = (*Resource)(nil)
var _ concepts.MetricsIdentifiable = (*Resource)(nil)
