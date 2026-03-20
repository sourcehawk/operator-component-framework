package concepts

// CompletionStatus represents the execution state of a resource that runs to completion (e.g., Jobs, Tasks).
type CompletionStatus string

const (
	// CompletionStatusCompleted indicates the resource has finished its execution successfully.
	CompletionStatusCompleted CompletionStatus = "Completed"
	// CompletionStatusRunning indicates the resource is currently running.
	CompletionStatusRunning CompletionStatus = "TaskRunning"
	// CompletionStatusPending indicates the resource is waiting to start.
	CompletionStatusPending CompletionStatus = "TaskPending"
	// CompletionStatusFailing indicates the resource has finished its execution with an error.
	CompletionStatusFailing CompletionStatus = "TaskFailing"
)

// CompletionStatusWithReason is the explanation of why the resource is in its current execution state.
type CompletionStatusWithReason struct {
	// Status is the current execution state of the resource.
	Status CompletionStatus
	// Reason explains why the resource is in the current status.
	// Examples:
	//  - With Status=Completed: Job has finished successfully.
	//  - With Status=TaskRunning: Job has 1/1 active pods.
	//  - With Status=TaskPending: Job is waiting for pods to be scheduled.
	//  - With Status=TaskFailing: Job failed with exit code 1.
	Reason string
}

// Completable defines the contract for resources that run to completion rather than being long-running.
// Resources implementing this interface contribute to the component's aggregate completion status.
type Completable interface {
	// ConvergingStatus returns the resource's current completion state.
	// The provided ConvergingOperation helps the resource decide its current status.
	ConvergingStatus(op ConvergingOperation) (CompletionStatusWithReason, error)
}
