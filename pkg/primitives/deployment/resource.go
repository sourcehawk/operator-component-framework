package deployment

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource is a high-level abstraction for managing a Kubernetes Deployment within a controller's
// reconciliation loop.
//
// It implements several component interfaces to integrate with the operator-component-framework:
//   - component.Resource: for basic identity and mutation behavior.
//   - component.Alive: for health and readiness tracking.
//   - component.Suspendable: for graceful scale-down or temporary deactivation.
//   - component.DataExtractable: for exporting information after successful reconciliation.
//
// This resource handles the lifecycle of a Deployment, including initial creation,
// updates via feature mutations, and status monitoring.
type Resource struct {
	base *generic.WorkloadResource[*appsv1.Deployment, *Mutator]
}

// Identity returns a unique identifier for the Deployment in the format
// "apps/v1/Deployment/<namespace>/<name>".
//
// This identifier is used by the framework's internal tracking and recording
// mechanisms to distinguish this specific Deployment from other resources
// managed by the same component.
func (r *Resource) Identity() string {
	return r.base.Identity()
}

// Object returns a copy of the underlying Kubernetes Deployment object.
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

// Mutate transforms the current state of a Kubernetes Deployment into the desired state.
//
// The mutation process follows a specific order:
//  1. Core State: The desired base state is applied to the current object.
//  2. Feature Mutations: All registered feature-based mutations are applied,
//     allowing for granular, version-gated changes to the Deployment.
//  3. Suspension: If the resource is in a suspending state, the suspension
//     logic (e.g., scaling to zero) is applied.
//
// This method is invoked by the framework during the "Update" phase of
// reconciliation. It ensures that the in-memory object reflects all
// configuration and feature requirements before it is sent to the API server.
func (r *Resource) Mutate(current client.Object) error {
	return r.base.Mutate(current)
}

// ConvergingStatus evaluates if the Deployment has successfully reached its desired state.
//
// By default, it uses DefaultConvergingStatusHandler, which checks if the number of ReadyReplicas
// matches the desired replica count.
//
// The return value includes a descriptive status (Ready, Creating, Updating, or Scaling)
// and a human-readable reason, which are used to update the component's conditions.
//
// When to use:
// This is used by the framework after an Apply operation to determine if the
// reconciliation of this specific resource is complete or if further waiting is required.
func (r *Resource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.AliveStatusWithReason, error) {
	return r.base.ConvergingStatus(op)
}

// GraceStatus provides a health assessment of the Deployment when it has not yet
// reached full readiness.
//
// By default, it uses DefaultGraceStatusHandler, which categorizes the current state into:
//   - GraceStatusDegraded: At least one replica is ready, but the desired count is not met.
//   - GraceStatusDown: No replicas are ready.
//
// This information is surfaced through the component's health reporting, allowing
// operators to understand the severity of a rollout delay or failure.
func (r *Resource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	return r.base.GraceStatus()
}

// DeleteOnSuspend determines whether the Deployment should be deleted from the
// cluster when the parent component is suspended.
//
// By default, it uses DefaultDeleteOnSuspendHandler, which returns false, meaning
// the Deployment is kept in the cluster but scaled to zero replicas. This preserves
// the resource definition and history while stopping the workload.
//
// A custom decision handler can be registered via the Builder to change this
// behavior based on the current state of the Deployment.
func (r *Resource) DeleteOnSuspend() bool {
	return r.base.DeleteOnSuspend()
}

// Suspend triggers the deactivation of the Deployment.
//
// It registers a mutation that will be executed during the next Mutate call.
// The default behavior uses DefaultSuspendMutationHandler to scale the Deployment
// to zero replicas, which effectively stops the application while keeping the
// Kubernetes resource intact.
//
// This is typically called by the framework when a component's .spec.suspended
// field is set to true.
func (r *Resource) Suspend() error {
	return r.base.Suspend()
}

// SuspensionStatus monitors the progress of the suspension process.
//
// By default, it uses DefaultSuspensionStatusHandler, which reports whether the
// Deployment has successfully scaled down to zero replicas or is still in the
// process of doing so. The framework uses this to determine when the component
// has reached a fully suspended state.
func (r *Resource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return r.base.SuspensionStatus()
}

// ExtractData executes registered data extraction functions to harvest information
// from the reconciled Deployment.
//
// This is called by the framework after a successful reconciliation of the
// resource. It allows the component to export details (like a generated name,
// assigned IP, or status fields) that might be needed by other resources or
// higher-level controllers.
//
// Data extractors are provided with a deep copy of the current Deployment to
// prevent accidental mutations during the extraction process.
func (r *Resource) ExtractData() error {
	return r.base.ExtractData()
}
