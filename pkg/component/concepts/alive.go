// Package concepts defines the core concepts for the operator component framework.
package concepts

// AliveConvergingStatus represents the transitional state of an alive resource as it moves towards "healthy".
type AliveConvergingStatus string

const (
	// AliveConvergingStatusHealthy indicates the resource has reached its desired state and is fully operational.
	AliveConvergingStatusHealthy AliveConvergingStatus = "Healthy"
	// AliveConvergingStatusCreating indicates the resource is being created for the first time.
	AliveConvergingStatusCreating AliveConvergingStatus = "Creating"
	// AliveConvergingStatusUpdating indicates an existing resource is being updated with new configuration.
	AliveConvergingStatusUpdating AliveConvergingStatus = "Updating"
	// AliveConvergingStatusScaling indicates the resource is scaling its capacity (e.g., adding/removing replicas).
	AliveConvergingStatusScaling AliveConvergingStatus = "Scaling"
	// AliveConvergingStatusFailing indicates the resource is failing to converge to healthy.
	AliveConvergingStatusFailing AliveConvergingStatus = "Failing"
)

// AliveStatusWithReason is the explanation of why the resource is or is not healthy at health checking time.
type AliveStatusWithReason struct {
	// Status is the status of the resource while converging towards healthy (can also be healthy).
	Status AliveConvergingStatus
	// Reason explains why the resource is currently healthy, Creating, Updating or Scaling.
	// Examples:
	//  - With Status=healthy: Deployment is healthy.
	//  - With Status=Created (ConvergingOperationCreated): Deployment has 2/3 healthy replicas.
	//  - With Status=Updated (ConvergingOperationUpdated): Deployment has 2/3 healthy replicas.
	//  - With Status=Scaling (ConvergingOperationNone): Deployment has 0/3 healthy replicas.
	//  - With Status=failing: failing to pull image
	Reason string
}

// Alive defines the contract for resources that have observable health and readiness.
// Resources implementing this interface contribute to the component's aggregate status.
// If a resource does NOT implement Alive, it is considered "Ready" as long as it exists.
type Alive interface {
	// ConvergingStatus returns the resource's current progress towards "Ready".
	// The provided ConvergingOperation helps the resource decide if it's currently Creating or Updating.
	ConvergingStatus(op ConvergingOperation) (AliveStatusWithReason, error)
}
