package concepts

// OperationalStatus represents the operational state of a resource.
type OperationalStatus string

const (
	// OperationalStatusOperational indicates the resource is fully operational.
	OperationalStatusOperational OperationalStatus = "Operational"
	// OperationalStatusPending indicates the resource is waiting to become operational.
	OperationalStatusPending OperationalStatus = "OperationPending"
	// OperationalStatusFailing indicates the resource is not operational.
	OperationalStatusFailing OperationalStatus = "OperationFailing"
)

type OperationalStatusWithReason struct {
	Status OperationalStatus
	// Reason explains why the resource is currently 'Operational', 'Pending' or 'failing'.
	// Examples:
	//  - With Status=Operational: Service load balancer IP address assigned.
	//  - With Status=Pending: Awaiting load balancer IP assignment.
	//  - With Status=failing: Missing cloud provider annotation for load balancer assignment.
	Reason string
}

// Operational describes resources who are not necessarily alive but depend on assignments or other external
// dependencies to be considered operational. This can be services, ingresses, cronjob schedules, or any other
// resource who may be perceived as static but also depend on cluster-external factors.
type Operational interface {
	ConvergingStatus(op ConvergingOperation) (OperationalStatusWithReason, error)
}
