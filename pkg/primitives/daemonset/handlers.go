package daemonset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a DaemonSet has reached its desired state.
//
// It reports Healthy when all of these are true:
//   - The DaemonSet controller has observed the current generation
//     (Status.ObservedGeneration >= ObjectMeta.Generation).
//   - Status.NumberReady equals Status.DesiredNumberScheduled.
//   - The rollout is complete: Status.UpdatedNumberScheduled is not less than
//     Status.DesiredNumberScheduled. A DaemonSet with the OnDelete update strategy skips this check,
//     so Healthy does not mean that its pods run the current template.
//
// A DesiredNumberScheduled of zero means that no nodes match the node selector. Such a DaemonSet is
// Healthy once its controller has observed the current generation.
//
// Otherwise it reports Creating, Updating, or Scaling.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomConvergeStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultConvergingStatusHandler(
	op concepts.ConvergingOperation, ds *appsv1.DaemonSet,
) (concepts.AliveStatusWithReason, error) {
	if status := concepts.StaleGenerationStatus(
		op, ds.Status.ObservedGeneration, ds.Generation, "DaemonSet",
	); status != nil {
		return *status, nil
	}

	desired := ds.Status.DesiredNumberScheduled

	if desired == 0 {
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusHealthy,
			Reason: "No nodes match the DaemonSet node selector",
		}, nil
	}

	if ds.Status.NumberReady != desired {
		var status concepts.AliveConvergingStatus
		switch op {
		case concepts.ConvergingOperationCreated:
			status = concepts.AliveConvergingStatusCreating
		case concepts.ConvergingOperationUpdated:
			status = concepts.AliveConvergingStatusUpdating
		default:
			status = concepts.AliveConvergingStatusScaling
		}

		return concepts.AliveStatusWithReason{
			Status: status,
			Reason: fmt.Sprintf("Waiting for pods: %d/%d ready", ds.Status.NumberReady, desired),
		}, nil
	}

	if reason, pending := pendingRollout(ds); pending {
		status := concepts.AliveConvergingStatusUpdating
		if op == concepts.ConvergingOperationCreated {
			status = concepts.AliveConvergingStatusCreating
		}

		return concepts.AliveStatusWithReason{Status: status, Reason: reason}, nil
	}

	return concepts.AliveStatusWithReason{
		Status: concepts.AliveConvergingStatusHealthy,
		Reason: "All pods are ready",
	}, nil
}

// pendingRollout reports why the rollout of the current pod template is not complete, or false
// when it is complete for the update strategy of the DaemonSet.
func pendingRollout(ds *appsv1.DaemonSet) (string, bool) {
	// The DaemonSet controller does not replace pods under OnDelete, so they move to the current
	// template only when something outside the controller deletes them.
	if ds.Spec.UpdateStrategy.Type == appsv1.OnDeleteDaemonSetStrategyType {
		return "", false
	}

	// With maxSurge the controller keeps the old pod on a node until the new pod is ready, and it
	// counts only the oldest pod of each node, so NumberReady can match while new pods fail.
	status := ds.Status
	if status.UpdatedNumberScheduled < status.DesiredNumberScheduled {
		return fmt.Sprintf(
			"Waiting for rollout: %d/%d pods updated", status.UpdatedNumberScheduled, status.DesiredNumberScheduled,
		), true
	}

	return "", false
}

// DefaultGraceStatusHandler provides a default health assessment of the DaemonSet when it has not yet
// reached full readiness.
//
// It categorizes the current state into:
//   - GraceStatusHealthy: DefaultConvergingStatusHandler reports Healthy for the same DaemonSet.
//   - GraceStatusDown: DesiredNumberScheduled is more than zero and no pods are ready.
//   - GraceStatusDegraded: All other states.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(ds *appsv1.DaemonSet) (concepts.GraceStatusWithReason, error) {
	desired := ds.Status.DesiredNumberScheduled

	if ds.Status.NumberReady == 0 && desired > 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: "No pods are ready",
		}, nil
	}

	if ds.Status.ObservedGeneration < ds.Generation {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Waiting for DaemonSet controller to observe latest spec",
		}, nil
	}

	if desired == 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusHealthy,
			Reason: "No nodes match the DaemonSet node selector",
		}, nil
	}

	if ds.Status.NumberReady != desired {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "DaemonSet partially available",
		}, nil
	}

	if reason, pending := pendingRollout(ds); pending {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: reason,
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusHealthy,
		Reason: "All pods are ready",
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
