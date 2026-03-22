package daemonset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a DaemonSet has reached its desired state.
//
// It considers a DaemonSet ready when:
//   - Status.NumberReady >= Status.DesiredNumberScheduled and DesiredNumberScheduled > 0, or
//   - DesiredNumberScheduled is zero and the controller has observed the current generation
//     (no matching nodes is a valid converged state).
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomConvergeStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultConvergingStatusHandler(
	op concepts.ConvergingOperation, ds *appsv1.DaemonSet,
) (concepts.AliveStatusWithReason, error) {
	desired := ds.Status.DesiredNumberScheduled

	if desired > 0 && ds.Status.NumberReady >= desired {
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusHealthy,
			Reason: "All pods are ready",
		}, nil
	}

	if desired == 0 && ds.Status.ObservedGeneration >= ds.Generation {
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusHealthy,
			Reason: "No nodes match the DaemonSet node selector",
		}, nil
	}

	var status concepts.AliveConvergingStatus
	switch op {
	case concepts.ConvergingOperationCreated:
		status = concepts.AliveConvergingStatusCreating
	case concepts.ConvergingOperationUpdated:
		status = concepts.AliveConvergingStatusUpdating
	default:
		status = concepts.AliveConvergingStatusScaling
	}

	var reason string
	if ds.Status.ObservedGeneration < ds.Generation {
		reason = "Waiting for controller to observe latest generation"
	} else {
		reason = fmt.Sprintf("Waiting for pods: %d/%d ready", ds.Status.NumberReady, desired)
	}

	return concepts.AliveStatusWithReason{
		Status: status,
		Reason: reason,
	}, nil
}

// DefaultGraceStatusHandler provides a default health assessment of the DaemonSet when it has not yet
// reached full readiness.
//
// It categorizes the current state into:
//   - GraceStatusUp: DesiredNumberScheduled is zero — no nodes match the selector; this is a
//     valid configuration state, not a failure.
//   - GraceStatusDegraded: DesiredNumberScheduled > 0 and at least one pod is ready, but below desired.
//   - GraceStatusDown: DesiredNumberScheduled > 0 and no pods are ready.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(ds *appsv1.DaemonSet) (concepts.GraceStatusWithReason, error) {
	if ds.Status.DesiredNumberScheduled == 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusHealthy,
			Reason: "No nodes match the DaemonSet node selector",
		}, nil
	}

	if ds.Status.NumberReady >= 1 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "DaemonSet partially available",
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDown,
		Reason: "No pods are ready",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the DaemonSet
// when the parent component is suspended.
//
// It always returns true because DaemonSets have no replicas field and cannot be
// scaled to zero in-place.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendDeletionDecision. It can be reused within custom handlers.
func DefaultDeleteOnSuspendHandler(_ *appsv1.DaemonSet) bool {
	return true
}

// DefaultSuspendMutationHandler provides the default mutation applied to a DaemonSet when the component is suspended.
//
// It is a no-op because DaemonSets are deleted on suspension rather than mutated.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(_ *Mutator) error {
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// It always reports Suspended with a reason indicating that the DaemonSet is deleted
// on suspension, because there is no in-place scale-down mechanism for DaemonSets.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(_ *appsv1.DaemonSet) (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "DaemonSet deleted on suspend",
	}, nil
}
