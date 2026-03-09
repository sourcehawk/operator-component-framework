package component

// Status represents the internal state of a component.
// It is used as the Reason in a Kubernetes condition and provides
// a standardized way to reflect the health and progress of a component.
type Status string

const (
	// Unknown indicates that the component has not been reconciled yet.
	Unknown Status = "Unknown"
	// Ready indicates that the component is fully provisioned
	// and operating as expected.
	Ready = Status(ConvergingStatusReady)
	// Creating indicates that the resource are being created.
	Creating = Status(ConvergingStatusCreating)
	// Scaling indicates that the resources are being scaled.
	Scaling = Status(ConvergingStatusScaling)
	// Updating indicates that the resources are being updated.
	Updating = Status(ConvergingStatusUpdating)
	// PendingSuspension indicates that the component is aware of the suspension request but has yet to begin suspension.
	PendingSuspension = Status(SuspensionStatusPending)
	// Suspending indicates that the component is converging towards a suspended state but is not yet fully suspended.
	Suspending = Status(SuspensionStatusSuspending)
	// Suspended indicates that the component is suspended.
	Suspended = Status(SuspensionStatusSuspended)
	// Degraded indicates that the component is degraded but operational.
	Degraded = Status(GraceStatusDegraded)
	// Down indicates that component is down and not operational.
	Down = Status(GraceStatusDown)
	// Error indicates that resource errors happened during reconciliation.
	Error Status = "Error"
)

// Level returns the aggregation priority of a component Status.
//
// The returned value is used when multiple component statuses must be
// collapsed into a single overall status (for example by a parent component
// or system-level status aggregator). The status with the highest Level()
// should be selected as the representative state.
//
// The ordering reflects the most meaningful explanation of the system's
// current state:
//
//   - Error, Down and Degraded represent failure states and therefore have
//     the highest priority.
//   - Suspension-related states (PendingSuspension, Suspending, Suspended)
//     represent an intentional lifecycle mode and take precedence over normal
//     convergence states.
//   - Convergence states (Creating, Updating, Scaling) represent normal
//     reconciliation progress toward Ready.
//   - Ready represents the steady-state healthy condition.
//
// Unknown or unrecognized statuses return 0 and therefore do not influence
// aggregation.
func (s Status) Level() int {
	switch s {
	case Ready:
		return 1
	case Creating:
		return 2
	case Updating:
		return 3
	case Scaling:
		return 4
	case Suspended:
		return 5
	case Suspending:
		return 6
	case PendingSuspension:
		return 7
	case Degraded:
		return 8
	case Down:
		return 9
	case Error:
		return 10
	}
	return 0
}

// progressing returns true if the reason is a progressing state.
func (s Status) progressing() bool {
	switch s {
	case Creating, Updating, Scaling:
		return true
	}
	return false
}
