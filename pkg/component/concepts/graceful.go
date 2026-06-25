package concepts

// GraceStatus represents the health of a resource after the allowed grace period has expired.
type GraceStatus string

const (
	// GraceStatusHealthy indicates the resource is fully healthy after the grace period.
	GraceStatusHealthy = GraceStatus(AliveConvergingStatusHealthy)
	// GraceStatusDegraded indicates the resource is partially functional or in an intermediate state
	// after the grace period has expired.
	GraceStatusDegraded GraceStatus = "Degraded"
	// GraceStatusDown indicates the resource is completely non-functional after the grace period.
	GraceStatusDown GraceStatus = "Down"
)

// Priority returns the priority of the grace status.
// Higher values indicate more severe health issues.
// This is used for status aggregation: "Down" takes precedence over "Degraded", which takes precedence over "Healthy".
func (s GraceStatus) Priority() int {
	switch s {
	case GraceStatusHealthy:
		return 1
	case GraceStatusDegraded:
		return 2
	case GraceStatusDown:
		return 3
	}
	return 0
}

// GraceStatusWithReason is the explanation of why the resource did or did not converge to healthy on grace expiry.
type GraceStatusWithReason struct {
	// Status is the status of the resource when the grace period expired.
	Status GraceStatus
	// Reason explains the reason why the resource is healthy, Down or Degraded at grace expiry.
	// Examples:
	//  - With Status=healthy: Deployment is healthy.
	//  - With Status=Degraded: Deployment has 2/3 healthy replicas.
	//  - With Status=Down: Deployment has 0/3 healthy replicas.
	Reason string
}

// Graceful defines the contract for resources which have time constrained convergence.
type Graceful interface {
	// GraceStatus returns the final health assessment after the component's grace period has expired.
	// The implementation should assume the grace period HAS expired and return its current state
	// (healthy, Degraded, or Down) without internal timing logic.
	GraceStatus() (GraceStatusWithReason, error)
}
