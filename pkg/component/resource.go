package component

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Resource is a generic interface for handling Kubernetes resources within a Component.
// Implementations of this interface wrap a specific Kubernetes object and define
// how its immutable and mutable fields are applied during reconciliation.
type Resource interface {
	// Mutate applies all fields on the resource.
	// These fields are updated every time the component is reconciled.
	Mutate() error
	// Object returns the underlying k8s resource object.
	Object() (client.Object, error)
	// Identity returns a unique identifier for the resource in the format <apiVersion>/<kind>/<name>.
	Identity() string
}

// SuspensionStatus is the status determined for the resource at suspension time.
// It represents the progress of a resource towards a fully suspended state.
type SuspensionStatus string

const (
	// SuspensionStatusPending indicates that the suspension is waiting for a precondition to be met.
	SuspensionStatusPending SuspensionStatus = "PendingSuspension"
	// SuspensionStatusSuspending indicates that suspension is in progress but not yet completed.
	// For example, a Deployment might be scaling down its replicas.
	SuspensionStatusSuspending SuspensionStatus = "Suspending"
	// SuspensionStatusSuspended indicates that the suspension has successfully completed.
	SuspensionStatusSuspended SuspensionStatus = "Suspended"
)

// level returns the priority of the suspension status.
// Higher values indicate a state that is further from the desired "Suspended" state.
// This is used for status aggregation when multiple resources are being suspended.
func (s SuspensionStatus) level() int {
	switch s {
	case SuspensionStatusSuspended:
		return 1
	case SuspensionStatusSuspending:
		return 2
	case SuspensionStatusPending:
		return 3
	}
	return 0
}

// SuspensionStatusWithReason is the explanation of why the resource is or is not Suspended at suspension checking time.
type SuspensionStatusWithReason struct {
	// Status is the status of the resource while converging towards Suspended (can also be Suspended)
	Status SuspensionStatus
	// Reason explains the reason why the Status is currently PendingSuspension, Suspending or Suspended.
	// Examples:
	//  - With Status=PendingSuspension (SuspensionStatusPending): Waiting for statefulset observed generation to match generation.
	//  - With Status=Suspending (SuspensionStatusSuspending): Replicas scaling down. 1/3 replicas running.
	//  - With Status=Suspended (SuspensionStatusSuspended): Replicas scaled down to 0.
	Reason string
}

// Suspendable defines the contract for resources that support controlled suspension.
// Suspension can be achieved through deletion, mutations (like scaling to zero), or both.
// Any resource not implementing this interface is considered non-suspendable and remains active.
type Suspendable interface {
	// DeleteOnSuspend returns true if the resource should be deleted after suspension is complete.
	// Note: Suspend() and SuspensionStatus() are still called even if this returns true.
	// The resource must reach SuspensionStatusSuspended before it is actually deleted.
	// This allows for necessary cleanup or state persistence (e.g., ensuring disks are retained)
	// before the Kubernetes object is removed.
	DeleteOnSuspend() bool
	// Suspend applies suspension mutations to the resource's desired state.
	// The suspension intent MUST be stored in the wrapper's internal state and applied
	// during a subsequent Mutate() call.
	// Suspend MUST NOT mutate the Kubernetes cluster state directly.
	Suspend() error
	// SuspensionStatus returns the current progress of the suspension.
	// It is called after Suspend() to track when the resource has reached the desired state.
	SuspensionStatus() (SuspensionStatusWithReason, error)
}

// ConvergingStatus represents the transitional state of a resource as it moves towards "Ready".
type ConvergingStatus string

const (
	// ConvergingStatusReady indicates the resource has reached its desired state and is fully operational.
	ConvergingStatusReady ConvergingStatus = "Ready"
	// ConvergingStatusCreating indicates the resource is being created for the first time.
	ConvergingStatusCreating ConvergingStatus = "Creating"
	// ConvergingStatusUpdating indicates an existing resource is being updated with new configuration.
	ConvergingStatusUpdating ConvergingStatus = "Updating"
	// ConvergingStatusScaling indicates the resource is scaling its capacity (e.g., adding/removing replicas).
	ConvergingStatusScaling ConvergingStatus = "Scaling"
)

// level returns the priority of the converging status.
// Higher values indicate a state that requires more attention or is "less ready" than others.
// This is used to aggregate the status of multiple resources into a single component condition.
func (s ConvergingStatus) level() int {
	switch s {
	case ConvergingStatusReady:
		return 1
	case ConvergingStatusCreating:
		return 2
	case ConvergingStatusUpdating:
		return 3
	case ConvergingStatusScaling:
		return 4
	}
	return 0
}

// ConvergingOperation represents the result of a CreateOrUpdate operation on a resource.
// It provides context to the Alive interface to help determine the ConvergingStatus.
type ConvergingOperation string

const (
	// ConvergingOperationCreated indicates that the resource was newly created.
	ConvergingOperationCreated ConvergingOperation = "Created"
	// ConvergingOperationUpdated indicates that an existing resource was updated.
	ConvergingOperationUpdated ConvergingOperation = "Updated"
	// ConvergingOperationNone indicates that no changes were made to the resource.
	ConvergingOperationNone ConvergingOperation = "None"
)

// convergingOperationFromOperationResult maps a controllerutil.OperationResult to a ConvergingStatus.
func convergingOperationFromOperationResult(result controllerutil.OperationResult) ConvergingOperation {
	switch result {
	case controllerutil.OperationResultCreated:
		return ConvergingOperationCreated
	case controllerutil.OperationResultUpdated:
		return ConvergingOperationUpdated
	}
	return ConvergingOperationNone
}

// ConvergingStatusWithReason is the explanation of why the resource is or is not Ready at converge time.
type ConvergingStatusWithReason struct {
	// Status is the status of the resource while converging towards Ready (can also be Ready).
	Status ConvergingStatus
	// Reason explains why the resource is currently Ready, Creating, Updating or Scaling.
	// Examples:
	//  - With Status=ConvergingStatusReady: Deployment is ready.
	//  - With Status=ConvergingStatusCreated (ConvergingOperationCreated): Deployment has 2/3 ready replicas.
	//  - With Status=ConvergingStatusUpdated (ConvergingOperationUpdated): Deployment has 2/3 ready replicas.
	//  - With Status=ConvergingStatusScaling (ConvergingOperationNone): Deployment has 0/3 ready replicas.
	Reason string
}

// GraceStatus represents the health of a resource after the allowed grace period has expired.
type GraceStatus string

const (
	// GraceStatusReady indicates the resource is fully healthy after the grace period.
	GraceStatusReady GraceStatus = "Ready"
	// GraceStatusDegraded indicates the resource is partially functional or in an intermediate state
	// after the grace period has expired.
	GraceStatusDegraded GraceStatus = "Degraded"
	// GraceStatusDown indicates the resource is completely non-functional after the grace period.
	GraceStatusDown GraceStatus = "Down"
)

// level returns the priority of the grace status.
// Higher values indicate more severe health issues.
// This is used for status aggregation: "Down" takes precedence over "Degraded", which takes precedence over "Ready".
func (s GraceStatus) level() int {
	switch s {
	case GraceStatusReady:
		return 1
	case GraceStatusDegraded:
		return 2
	case GraceStatusDown:
		return 3
	}
	return 0
}

// GraceStatusWithReason is the explanation of why the resource did or did not converge to Ready on grace expiry.
type GraceStatusWithReason struct {
	// Status is the status of the resource when the grace period expired.
	Status GraceStatus
	// Reason explains the reason why the resource is Ready, Down or Degraded at grace expiry.
	// Examples:
	//  - With Status=Ready: Deployment is ready.
	//  - With Status=Degraded: Deployment has 2/3 ready replicas.
	//  - With Status=Down: Deployment has 0/3 ready replicas.
	Reason string
}

// Alive defines the contract for resources that have observable health and readiness.
// Resources implementing this interface contribute to the component's aggregate status.
// If a resource does NOT implement Alive, it is considered "Ready" as long as it exists.
type Alive interface {
	// ConvergingStatus returns the resource's current progress towards "Ready".
	// The provided ConvergingOperation helps the resource decide if it's currently Creating or Updating.
	ConvergingStatus(op ConvergingOperation) (ConvergingStatusWithReason, error)
	// GraceStatus returns the final health assessment after the component's grace period has expired.
	// The implementation should assume the grace period HAS expired and return its current state
	// (Ready, Degraded, or Down) without internal timing logic.
	GraceStatus() (GraceStatusWithReason, error)
}

// DataExtractable defines the contract for resources that need to expose internal data
// after they have been created, updated, or fetched from the cluster.
//
// Implement this interface when a resource contains information (like generated credentials,
// endpoint URLs, or status fields) that needs to be pulled back into the operator's
// memory for use by other components or for updating the parent CRD's status.
//
// Data extraction is intended to be an observational/read-only operation on the resource.
// It is triggered automatically during reconciliation after all resources have been
// synchronized with the cluster but before the final component condition is calculated.
type DataExtractable interface {
	// ExtractData performs the data extraction from the resource's underlying Kubernetes object.
	// The implementation should store the extracted data in its own fields or shared state
	// where it can be accessed by the caller.
	ExtractData() error
}
