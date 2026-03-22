package replicaset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a ReplicaSet has reached its desired state.
//
// It considers a ReplicaSet ready when its Status.ReadyReplicas matches the Spec.Replicas (defaulting to 1 if nil).
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomConvergeStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultConvergingStatusHandler(
	op concepts.ConvergingOperation, rs *appsv1.ReplicaSet,
) (concepts.AliveStatusWithReason, error) {
	desiredReplicas := int32(1)
	if rs.Spec.Replicas != nil {
		desiredReplicas = *rs.Spec.Replicas
	}

	if rs.Status.ReadyReplicas == desiredReplicas {
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusHealthy,
			Reason: "All replicas are ready",
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

	return concepts.AliveStatusWithReason{
		Status: status,
		Reason: fmt.Sprintf("Waiting for replicas: %d/%d ready", rs.Status.ReadyReplicas, desiredReplicas),
	}, nil
}

// DefaultGraceStatusHandler provides a default health assessment of the ReplicaSet when it has not yet
// reached full readiness.
//
// It categorizes the current state into:
//   - GraceStatusDegraded: At least one replica is ready, but the desired count is not met.
//   - GraceStatusDown: No replicas are ready.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(rs *appsv1.ReplicaSet) (concepts.GraceStatusWithReason, error) {
	if rs.Status.ReadyReplicas > 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "ReplicaSet partially available",
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDown,
		Reason: "No replicas are ready",
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
