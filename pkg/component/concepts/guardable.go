package concepts

// GuardStatus represents whether a resource's guard precondition has been met.
type GuardStatus string

const (
	// GuardStatusBlocked indicates that the resource's precondition is not yet satisfied.
	// The resource will not be applied and all resources registered after it are also skipped.
	GuardStatusBlocked GuardStatus = "Blocked"
	// GuardStatusUnblocked indicates that the resource's precondition is satisfied
	// and the resource can be applied normally.
	GuardStatusUnblocked GuardStatus = "Unblocked"
)

// GuardStatusWithReason pairs a guard evaluation result with a human-readable explanation.
type GuardStatusWithReason struct {
	// Status is the guard evaluation result.
	Status GuardStatus
	// Reason provides a human-readable explanation of why the guard is in this state.
	Reason string
}

// Guardable defines the contract for resources that have preconditions which must be
// satisfied before the resource can be applied to the cluster.
//
// When a guard evaluates to Blocked, the resource is not applied and all resources
// registered after it in the component are also skipped. The blocked reason is
// surfaced in the component's status condition.
//
// Guards are not evaluated during suspension. Suspension proceeds regardless of
// guard state to ensure the component can always be fully deactivated.
type Guardable interface {
	// GuardStatus evaluates the resource's precondition and returns whether
	// the resource is ready to be applied.
	GuardStatus() (GuardStatusWithReason, error)
}
