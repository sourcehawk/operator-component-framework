package service

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
)

// DefaultOperationalStatusHandler is the default logic for determining if a Service is operational.
//
// For LoadBalancer services, it reports OperationalStatusPending until at least one ingress
// entry (IP or hostname) is assigned to Status.LoadBalancer.Ingress, then reports
// OperationalStatusOperational.
//
// For all other service types (ClusterIP, NodePort, ExternalName, headless), it
// immediately reports OperationalStatusOperational.
//
// This function is used as the default handler by the Resource if no custom handler is
// registered via Builder.WithCustomOperationalStatus. It can be reused within custom
// handlers to augment the default behavior.
func DefaultOperationalStatusHandler(
	_ concepts.ConvergingOperation, svc *corev1.Service,
) (concepts.OperationalStatusWithReason, error) {
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		hasIngressAddress := false
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" || ing.Hostname != "" {
				hasIngressAddress = true
				break
			}
		}

		if !hasIngressAddress {
			return concepts.OperationalStatusWithReason{
				Status: concepts.OperationalStatusPending,
				Reason: "Awaiting load balancer IP/hostname assignment",
			}, nil
		}

		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusOperational,
			Reason: "Load balancer IP/hostname assigned",
		}, nil
	}

	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusOperational,
		Reason: "Service is operational",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the Service
// when the parent component is suspended.
//
// It always returns false, leaving the Service untouched during suspension. Services are
// typically stateless routing objects that are safe to leave in place. Override with
// Builder.WithCustomSuspendDeletionDecision to delete the Service on suspend.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendDeletionDecision. It can be reused within custom handlers.
func DefaultDeleteOnSuspendHandler(_ *corev1.Service) bool {
	return false
}

// DefaultSuspendMutationHandler provides the default mutation applied to a Service when the
// component is suspended.
//
// It is a no-op because the default behaviour leaves the Service unaffected by suspension.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(_ *Mutator) error {
	return nil
}

// DefaultSuspensionStatusHandler reports the progress of the suspension process.
//
// It always reports Suspended because the default behaviour is a no-op — the Service
// is left in place and requires no work to reach the suspended state.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(_ *corev1.Service) (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "Service unaffected by suspension",
	}, nil
}

// DefaultGraceStatusHandler provides the default health assessment of a Service when the
// component's grace period has expired.
//
// For LoadBalancer services, it reports Degraded if no ingress entries with an IP or
// hostname have been assigned to Status.LoadBalancer.Ingress, indicating that the
// external endpoint is not yet available. Once at least one ingress entry contains an
// IP or hostname, it reports Healthy.
//
// For all other service types (ClusterIP, NodePort, ExternalName, headless), it
// immediately reports Healthy because these types are functional as soon as they exist.
//
// This function is used as the default handler by the Resource if no custom handler is
// registered via Builder.WithCustomGraceStatus. It can be reused within custom handlers
// to augment the default behavior.
func DefaultGraceStatusHandler(svc *corev1.Service) (concepts.GraceStatusWithReason, error) {
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" || ing.Hostname != "" {
				return concepts.GraceStatusWithReason{
					Status: concepts.GraceStatusHealthy,
					Reason: "Load balancer IP/hostname assigned",
				}, nil
			}
		}

		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Awaiting load balancer IP/hostname assignment",
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusHealthy,
		Reason: "Service is healthy",
	}, nil
}
