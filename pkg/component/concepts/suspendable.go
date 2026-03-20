package concepts

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

// Priority returns the priority of the suspension status.
// Higher values indicate a state that is further from the desired "Suspended" state.
// This is used for status aggregation when multiple resources are being suspended.
func (s SuspensionStatus) Priority() int {
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
