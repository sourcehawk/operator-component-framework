package job

import (
	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes Job within a controller's
// reconciliation loop.
//
// It implements several component interfaces to integrate with the operator-component-framework:
//   - component.Resource: for basic identity and mutation behavior.
//   - component.Completable: for run-to-completion tracking.
//   - component.Suspendable: for controlled deactivation (suspend or delete).
//   - component.DataExtractable: for exporting information after successful reconciliation.
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
// Data extractors are provided with a deep copy of the current Job to
// prevent accidental mutations during the extraction process.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
