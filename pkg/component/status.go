package component

import "github.com/sourcehawk/operator-component-framework/pkg/component/concepts"

// ConvergingStatus represents the transitional state of a resource as it moves towards healthiness.
type convergingStatus string

const (
	convergingStatusAliveHealthy  = convergingStatus(concepts.AliveConvergingStatusHealthy)
	convergingStatusAliveCreating = convergingStatus(concepts.AliveConvergingStatusCreating)
	convergingStatusAliveUpdating = convergingStatus(concepts.AliveConvergingStatusUpdating)
	convergingStatusAliveScaling  = convergingStatus(concepts.AliveConvergingStatusScaling)
	convergingStatusAliveFailing  = convergingStatus(concepts.AliveConvergingStatusFailing)

	convergingStatusOperationalOperational = convergingStatus(concepts.OperationalStatusOperational)
	convergingStatusOperationalPending     = convergingStatus(concepts.OperationalStatusPending)
	convergingStatusOperationalFailing     = convergingStatus(concepts.OperationalStatusFailing)

	convergingStatusCompletableCompleted = convergingStatus(concepts.CompletionStatusCompleted)
	convergingStatusCompletablePending   = convergingStatus(concepts.CompletionStatusPending)
	convergingStatusCompletableRunning   = convergingStatus(concepts.CompletionStatusRunning)
	convergingStatusCompletableFailed    = convergingStatus(concepts.CompletionStatusFailing)
)

// convergingStatusWithReason is the explanation of why the resource is in its current convergence state.
type convergingStatusWithReason struct {
	Status convergingStatus
	Reason string
}

// severity returns the severity of the converging status.
// Higher values indicate a state that requires more attention or is "less healthy" than others.
// This is used to aggregate the status of multiple resources into a single component condition.
func (s convergingStatus) severity() int {
	switch s {
	case convergingStatusAliveHealthy, convergingStatusOperationalOperational, convergingStatusCompletableCompleted:
		return 1
	case convergingStatusAliveCreating, convergingStatusOperationalPending, convergingStatusCompletablePending:
		return 2
	case convergingStatusAliveUpdating:
		return 3
	case convergingStatusAliveScaling, convergingStatusCompletableRunning:
		return 4
	case convergingStatusAliveFailing, convergingStatusOperationalFailing, convergingStatusCompletableFailed:
		return 5
	}
	return 0
}

// priority returns the priority of the converging status.
// Higher values indicate that the status proceeds other statuses in component health display.
func (s convergingStatus) priority() int {
	switch s {
	case convergingStatusCompletableCompleted:
		return 1
	case convergingStatusOperationalOperational:
		return 2
	case convergingStatusAliveHealthy:
		return 3
	case convergingStatusCompletablePending:
		return 4
	case convergingStatusOperationalPending:
		return 5
	case convergingStatusAliveCreating:
		return 6
	case convergingStatusAliveUpdating:
		return 7
	case convergingStatusCompletableRunning:
		return 8
	case convergingStatusAliveScaling:
		return 9
	case convergingStatusCompletableFailed:
		return 10
	case convergingStatusOperationalFailing:
		return 11
	case convergingStatusAliveFailing:
		return 12
	}
	return 0
}

// healthy returns true if the converging status signals healthiness of the resource.
func (s convergingStatus) healthy() bool {
	switch s {
	case convergingStatusAliveHealthy, convergingStatusOperationalOperational, convergingStatusCompletableCompleted:
		return true
	default:
		return false
	}
}

func getConvergingStatus(resource Resource, op concepts.ConvergingOperation) (*convergingStatusWithReason, error) {
	if alive, ok := resource.(concepts.Alive); ok {
		status, err := alive.ConvergingStatus(op)
		if err != nil {
			return nil, err
		}

		return &convergingStatusWithReason{
			Status: convergingStatus(status.Status),
			Reason: status.Reason,
		}, nil
	}

	if operational, ok := resource.(concepts.Operational); ok {
		status, err := operational.ConvergingStatus(op)
		if err != nil {
			return nil, err
		}

		return &convergingStatusWithReason{
			Status: convergingStatus(status.Status),
			Reason: status.Reason,
		}, nil
	}

	if completable, ok := resource.(concepts.Completable); ok {
		status, err := completable.ConvergingStatus(op)
		if err != nil {
			return nil, err
		}

		return &convergingStatusWithReason{
			Status: convergingStatus(status.Status),
			Reason: status.Reason,
		}, nil
	}

	return nil, nil
}

// Status represents the internal state of a component.
// It is used as the Reason in a Kubernetes condition and provides
// a standardized way to reflect the health and progress of a component.
type Status string

const (
	// Unknown indicates that the component has not been reconciled yet.
	Unknown Status = "Unknown"
	// Error indicates that resource errors happened during reconciliation.
	Error Status = "Error"

	// Healthy indicates that the component is fully provisioned and operating as expected.
	Healthy = Status(concepts.AliveConvergingStatusHealthy)
	// AliveCreating indicates that the resource are being created.
	AliveCreating = Status(concepts.AliveConvergingStatusCreating)
	// AliveScaling indicates that the resources are being scaled.
	AliveScaling = Status(concepts.AliveConvergingStatusScaling)
	// AliveUpdating indicates that the resources are being updated.
	AliveUpdating = Status(concepts.AliveConvergingStatusUpdating)
	// AliveFailing indicates that the resources are failing to converge.
	AliveFailing = Status(concepts.AliveConvergingStatusFailing)

	// Operational indicates that the resources are operational.
	Operational = Status(concepts.OperationalStatusOperational)
	// OperationPending indicates that operational resources are pending and not yet operational.
	OperationPending = Status(concepts.OperationalStatusPending)
	// OperationFailing indicates that operational resources are failing to become operational.
	OperationFailing = Status(concepts.OperationalStatusFailing)

	// Completed indicates that completable resources successfully completed their task.
	Completed = Status(concepts.CompletionStatusCompleted)
	// CompletionPending indicates that completable resources are still pending.
	CompletionPending = Status(concepts.CompletionStatusPending)
	// CompletionRunning indicates that completable tasks are still running.
	CompletionRunning = Status(concepts.CompletionStatusRunning)
	// CompletionFailing indicates that completable tasks are failing.
	CompletionFailing = Status(concepts.CompletionStatusFailing)

	// PendingSuspension indicates that the component is aware of the suspension request but has yet to begin suspension.
	PendingSuspension = Status(concepts.SuspensionStatusPending)
	// Suspending indicates that the component is converging towards a suspended state but is not yet fully suspended.
	Suspending = Status(concepts.SuspensionStatusSuspending)
	// Suspended indicates that the component is suspended.
	Suspended = Status(concepts.SuspensionStatusSuspended)

	// Degraded indicates that the component is degraded but operational.
	Degraded = Status(concepts.GraceStatusDegraded)
	// Down indicates that component is down and not operational.
	Down = Status(concepts.GraceStatusDown)
)

// Priority returns the aggregation priority of a component Status.
//
// The returned value is used when multiple component statuses must be
// collapsed into a single overall status (for example by a parent component
// or system-level status aggregator). The status with the highest Priority()
// should be selected as the representative state.
//
// The ordering reflects the most meaningful explanation of the system's
// current state:
//
// Unknown or unrecognized statuses return 0 and therefore do not influence
// aggregation.
func (s Status) Priority() int {
	switch s {
	case Completed:
		return 1
	case Operational:
		return 2
	case Healthy:
		return 3
	case CompletionPending:
		return 4
	case OperationPending:
		return 5
	case AliveCreating:
		return 6
	case AliveUpdating:
		return 7
	case CompletionRunning:
		return 8
	case AliveScaling:
		return 9
	case CompletionFailing:
		return 10
	case OperationFailing:
		return 11
	case AliveFailing:
		return 12
	case Suspended:
		return 13
	case Suspending:
		return 14
	case PendingSuspension:
		return 15
	case Degraded:
		return 16
	case Down:
		return 17
	case Error:
		return 18
	}

	return 0
}

// Healthy returns true if the status signals healthiness.
func (s Status) Healthy() bool {
	return convergingStatus(s).healthy()
}

func (s Status) sticky() bool {
	switch s {
	case AliveCreating, AliveUpdating, AliveScaling:
		return true
	default:
		return false
	}
}
