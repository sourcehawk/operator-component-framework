package statefulset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a StatefulSet has reached its desired state.
//
// It considers a StatefulSet ready when the statefulset controller has observed the current generation
// (Status.ObservedGeneration >= ObjectMeta.Generation) and Status.ReadyReplicas matches the
// Spec.Replicas (defaulting to 1 if nil). If the controller has not yet observed the latest spec,
// the handler reports Creating (when the resource was just created) or Updating (otherwise) to avoid
// falsely reporting health based on stale status fields.
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

	desiredReplicas := int32(1)
	if sts.Spec.Replicas != nil {
		desiredReplicas = *sts.Spec.Replicas
	}

	if sts.Status.ReadyReplicas == desiredReplicas {
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
		Reason: fmt.Sprintf("Waiting for replicas: %d/%d ready", sts.Status.ReadyReplicas, desiredReplicas),
	}, nil
}

// DefaultGraceStatusHandler provides a default health assessment of the StatefulSet when it has not yet
// reached full readiness.
//
// It categorizes the current state into:
//   - GraceStatusHealthy: ReadyReplicas meets or exceeds the desired replica count.
//   - GraceStatusDegraded: At least one replica is ready, but the desired count is not met.
//   - GraceStatusDown: No replicas are ready.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(sts *appsv1.StatefulSet) (concepts.GraceStatusWithReason, error) {
	desiredReplicas := int32(1)
	if sts.Spec.Replicas != nil {
		desiredReplicas = *sts.Spec.Replicas
	}

	if sts.Status.ReadyReplicas >= desiredReplicas {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusHealthy,
			Reason: "All replicas are ready",
		}, nil
	}

	if sts.Status.ReadyReplicas > 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "StatefulSet partially available",
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDown,
		Reason: "No replicas are ready",
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
