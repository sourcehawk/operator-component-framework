package statefulset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a StatefulSet has reached its desired state.
//
// It reports Healthy when all of these are true:
//   - The statefulset controller has observed the current generation
//     (Status.ObservedGeneration >= ObjectMeta.Generation).
//   - Status.ReadyReplicas equals Spec.Replicas (1 when nil).
//   - The rollout is complete for Spec.UpdateStrategy, as described below.
//
// For RollingUpdate (or an empty strategy type) without a partition, Status.UpdatedReplicas must
// not be less than Spec.Replicas, and Status.CurrentRevision must equal Status.UpdateRevision.
// For RollingUpdate with a partition more than zero, Status.UpdatedReplicas must not be less than
// Spec.Replicas minus the partition. The revisions are not compared, because the replicas below
// the partition keep the current revision. For OnDelete, the rollout is always complete: the
// controller does not replace pods, so the handler cannot wait for them to move to the update
// revision.
//
// Otherwise it reports Creating when the apply created the StatefulSet. For other operations it
// reports Updating while the controller is behind the spec or the rollout is incomplete. While the
// ready count differs from the desired count, it reports Updating for ConvergingOperationUpdated
// and Scaling for all other operations.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomConvergeStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultConvergingStatusHandler(
	op concepts.ConvergingOperation, sts *appsv1.StatefulSet,
) (concepts.AliveStatusWithReason, error) {
	if status := concepts.StaleGenerationStatus(
		op, sts.Status.ObservedGeneration, sts.Generation, "statefulset",
	); status != nil {
		return *status, nil
	}

	desired := desiredReplicas(sts)

	if sts.Status.ReadyReplicas != desired {
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
			Reason: fmt.Sprintf("Waiting for replicas: %d/%d ready", sts.Status.ReadyReplicas, desired),
		}, nil
	}

	if reason, pending := pendingRollout(sts, desired); pending {
		status := concepts.AliveConvergingStatusUpdating
		if op == concepts.ConvergingOperationCreated {
			status = concepts.AliveConvergingStatusCreating
		}

		return concepts.AliveStatusWithReason{Status: status, Reason: reason}, nil
	}

	return concepts.AliveStatusWithReason{
		Status: concepts.AliveConvergingStatusHealthy,
		Reason: "All replicas are ready",
	}, nil
}

func desiredReplicas(sts *appsv1.StatefulSet) int32 {
	if sts.Spec.Replicas == nil {
		return 1
	}
	return *sts.Spec.Replicas
}

// pendingRollout reports why the rollout of the current pod template is not complete, or false
// when it is complete for the update strategy of the StatefulSet.
func pendingRollout(sts *appsv1.StatefulSet, desiredReplicas int32) (string, bool) {
	strategy := sts.Spec.UpdateStrategy
	if strategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		return "", false
	}

	status := sts.Status
	if strategy.RollingUpdate != nil && strategy.RollingUpdate.Partition != nil && *strategy.RollingUpdate.Partition > 0 {
		target := max(desiredReplicas-*strategy.RollingUpdate.Partition, 0)
		if status.UpdatedReplicas < target {
			return fmt.Sprintf(
				"Waiting for partitioned rollout: %d/%d replicas updated", status.UpdatedReplicas, target,
			), true
		}
		return "", false
	}

	if status.UpdatedReplicas < desiredReplicas {
		return fmt.Sprintf("Waiting for rollout: %d/%d replicas updated", status.UpdatedReplicas, desiredReplicas), true
	}

	// The statefulset controller moves CurrentRevision to UpdateRevision only when the rolling
	// update is complete, so the counts alone can look done one sync before the rollout is.
	if status.CurrentRevision != status.UpdateRevision {
		return fmt.Sprintf("Waiting for rollout to revision %s", status.UpdateRevision), true
	}

	return "", false
}

// DefaultGraceStatusHandler provides a default health assessment of the StatefulSet when it has not yet
// reached full readiness.
//
// It categorizes the current state into:
//   - GraceStatusHealthy: DefaultConvergingStatusHandler reports Healthy for the same StatefulSet.
//   - GraceStatusDown: Spec.Replicas is more than zero and no replicas are ready.
//   - GraceStatusDegraded: All other states. These include a statefulset controller that has not
//     observed the current generation, a ready count that differs from the desired count, and an
//     incomplete rollout.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(sts *appsv1.StatefulSet) (concepts.GraceStatusWithReason, error) {
	desired := desiredReplicas(sts)

	if sts.Status.ReadyReplicas == 0 && desired > 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: "No replicas are ready",
		}, nil
	}

	if sts.Status.ObservedGeneration < sts.Generation {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Waiting for statefulset controller to observe latest spec",
		}, nil
	}

	// Use != rather than < so that grace and convergence agree on replica state.
	// Grace must not return Healthy for a state that convergence considers
	// non-healthy (e.g. ReadyReplicas > desired during scale-down).
	if sts.Status.ReadyReplicas != desired {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "StatefulSet partially available",
		}, nil
	}

	if reason, pending := pendingRollout(sts, desired); pending {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: reason,
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusHealthy,
		Reason: "All replicas are ready",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the StatefulSet
// when the parent component is suspended.
//
// It always returns false, meaning the StatefulSet is kept in the cluster but scaled to zero replicas.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendDeletionDecision. It can be reused within custom handlers.
func DefaultDeleteOnSuspendHandler(_ *appsv1.StatefulSet) bool {
	return false
}

// DefaultSuspendMutationHandler provides the default mutation applied to a StatefulSet when the component is suspended.
//
// It scales the StatefulSet to zero replicas by setting Spec.Replicas to 0.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(mutator *Mutator) error {
	mutator.EnsureReplicas(0)
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// It reports whether the StatefulSet has successfully scaled down to zero replicas
// by checking if Status.Replicas is 0.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(sts *appsv1.StatefulSet) (concepts.SuspensionStatusWithReason, error) {
	if sts.Status.Replicas == 0 {
		return concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspended,
			Reason: "StatefulSet scaled to zero",
		}, nil
	}

	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspending,
		Reason: fmt.Sprintf("Waiting for replicas to scale down, %d replicas still running.", sts.Status.Replicas),
	}, nil
}
