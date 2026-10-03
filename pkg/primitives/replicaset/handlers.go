package replicaset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a ReplicaSet has reached its desired state.
//
// It reports Healthy when all of these are true:
//   - The replicaset controller has observed the current generation
//     (Status.ObservedGeneration >= ObjectMeta.Generation).
//   - Status.ReadyReplicas equals Spec.Replicas (1 when nil).
//   - The scale-down is complete: Status.Replicas is not more than Spec.Replicas.
//
// Otherwise it reports Creating, Updating, or Scaling.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomConvergeStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultConvergingStatusHandler(
	op concepts.ConvergingOperation, rs *appsv1.ReplicaSet,
) (concepts.AliveStatusWithReason, error) {
	if status := concepts.StaleGenerationStatus(
		op, rs.Status.ObservedGeneration, rs.Generation, "replicaset",
	); status != nil {
		return *status, nil
	}

	desired := desiredReplicas(rs)

	if rs.Status.ReadyReplicas != desired {
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
			Reason: fmt.Sprintf("Waiting for replicas: %d/%d ready", rs.Status.ReadyReplicas, desired),
		}, nil
	}

	if reason, pending := pendingScaleDown(rs, desired); pending {
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

func desiredReplicas(rs *appsv1.ReplicaSet) int32 {
	if rs.Spec.Replicas == nil {
		return 1
	}
	return *rs.Spec.Replicas
}

// pendingScaleDown reports why replicas above the desired count remain, or false when none remain.
// A replica that is not ready can wait for removal while ReadyReplicas already equals the desired
// count.
func pendingScaleDown(rs *appsv1.ReplicaSet, desiredReplicas int32) (string, bool) {
	if rs.Status.Replicas > desiredReplicas {
		return fmt.Sprintf("Waiting for scale-down: %d/%d replicas", rs.Status.Replicas, desiredReplicas), true
	}

	return "", false
}

// DefaultGraceStatusHandler provides a default health assessment of the ReplicaSet when it has not yet
// reached full readiness.
//
// It categorizes the current state into:
//   - GraceStatusHealthy: DefaultConvergingStatusHandler reports Healthy for the same ReplicaSet.
//   - GraceStatusDown: No replicas are ready and the desired count (Spec.Replicas, 1 when nil) is
//     more than zero.
//   - GraceStatusDegraded: All other states.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(rs *appsv1.ReplicaSet) (concepts.GraceStatusWithReason, error) {
	desired := desiredReplicas(rs)

	if rs.Status.ReadyReplicas == 0 && desired > 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: "No replicas are ready",
		}, nil
	}

	if rs.Status.ObservedGeneration < rs.Generation {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Waiting for replicaset controller to observe latest spec",
		}, nil
	}

	if rs.Status.ReadyReplicas != desired {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "ReplicaSet partially available",
		}, nil
	}

	if reason, pending := pendingScaleDown(rs, desired); pending {
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

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the ReplicaSet
// when the parent component is suspended.
//
// It always returns false, meaning the ReplicaSet is kept in the cluster but scaled to zero replicas.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendDeletionDecision. It can be reused within custom handlers.
func DefaultDeleteOnSuspendHandler(_ *appsv1.ReplicaSet) bool {
	return false
}

// DefaultSuspendMutationHandler provides the default mutation applied to a ReplicaSet when the component is suspended.
//
// It scales the ReplicaSet to zero replicas by setting Spec.Replicas to 0.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(mutator *Mutator) error {
	mutator.EnsureReplicas(0)
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// It reports whether the ReplicaSet has successfully scaled down to zero replicas
// by checking if Status.Replicas is 0.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(rs *appsv1.ReplicaSet) (concepts.SuspensionStatusWithReason, error) {
	if rs.Status.Replicas == 0 {
		return concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspended,
			Reason: "ReplicaSet scaled to zero",
		}, nil
	}

	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspending,
		Reason: fmt.Sprintf("Waiting for replicas to scale down, %d replicas still running.", rs.Status.Replicas),
	}, nil
}
