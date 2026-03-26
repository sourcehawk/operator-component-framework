package ingress

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	networkingv1 "k8s.io/api/networking/v1"
)

// DefaultOperationalStatusHandler is the default logic for determining if an Ingress
// has reached its operational state.
//
// It considers an Ingress operational when at least one IP or hostname is assigned
// in Status.LoadBalancer.Ingress. Until then it reports Pending.
//
// This function is used as the default handler by the Resource if no custom handler
// is registered via Builder.WithCustomOperationalStatus.
func DefaultOperationalStatusHandler(
	_ concepts.ConvergingOperation, ing *networkingv1.Ingress,
) (concepts.OperationalStatusWithReason, error) {
	for _, lb := range ing.Status.LoadBalancer.Ingress {
		if lb.IP != "" || lb.Hostname != "" {
			return concepts.OperationalStatusWithReason{
				Status: concepts.OperationalStatusOperational,
				Reason: "Ingress has been assigned an address",
			}, nil
		}
	}

	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusPending,
		Reason: "Awaiting load balancer address assignment",
	}, nil
}

// DefaultGraceStatusHandler provides a default health assessment of the Ingress
// after the component's grace period has expired.
//
// It categorizes the current state into:
//   - GraceStatusHealthy: At least one IP or hostname is assigned in Status.LoadBalancer.Ingress.
//   - GraceStatusDegraded: No load balancer address has been assigned yet.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(ing *networkingv1.Ingress) (concepts.GraceStatusWithReason, error) {
	for _, lb := range ing.Status.LoadBalancer.Ingress {
		if lb.IP != "" || lb.Hostname != "" {
			return concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusHealthy,
				Reason: "Ingress has been assigned an address",
			}, nil
		}
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDegraded,
		Reason: "Awaiting load balancer address assignment",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete
// the Ingress when the parent component is suspended.
//
// It always returns false. Deleting an Ingress causes the ingress controller (e.g.
// nginx) to reload its configuration, which affects the entire cluster's routing —
// not just the suspended service. When a backend service is suspended, the Ingress
// returning 502/503 is the correct observable behaviour.
//
// Operators that want explicit deletion can set DeleteOnSuspend: true via
// Builder.WithCustomSuspendDeletionDecision.
func DefaultDeleteOnSuspendHandler(_ *networkingv1.Ingress) bool {
	return false
}

// DefaultSuspendMutationHandler provides the default mutation applied to an Ingress
// when the component is suspended.
//
// It is a no-op. The Ingress is left in place and the backend service returning
// 502/503 is the expected observable behaviour during suspension.
func DefaultSuspendMutationHandler(_ *Mutator) error {
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// Since the default suspension is a no-op (the Ingress is not deleted or modified),
// it immediately reports Suspended with a reason indicating the backend is unavailable.
func DefaultSuspensionStatusHandler(_ *networkingv1.Ingress) (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "Ingress suspended (backend unavailable)",
	}, nil
}
