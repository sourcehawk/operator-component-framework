package deployment

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a Deployment has reached its desired state.
//
// It reports Healthy when all of these are true:
//   - The deployment controller has observed the current generation
//     (Status.ObservedGeneration >= ObjectMeta.Generation).
//   - Status.ReadyReplicas equals Spec.Replicas (1 when nil).
//   - The rollout is complete: Status.UpdatedReplicas is not less than Spec.Replicas, and
//     Status.Replicas is not more than Status.UpdatedReplicas, so no old replicas remain.
//     A paused Deployment (Spec.Paused) skips this check.
//
// Otherwise it reports Creating, Updating, or Scaling.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomConvergeStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultConvergingStatusHandler(
	op concepts.ConvergingOperation, deployment *appsv1.Deployment,
) (concepts.AliveStatusWithReason, error) {
	if status := concepts.StaleGenerationStatus(
		op, deployment.Status.ObservedGeneration, deployment.Generation, "deployment",
	); status != nil {
		return *status, nil
	}

	desired := desiredReplicas(deployment)

	if deployment.Status.ReadyReplicas != desired {
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
			Reason: fmt.Sprintf("Waiting for replicas: %d/%d ready", deployment.Status.ReadyReplicas, desired),
		}, nil
	}

	if reason, pending := pendingRollout(deployment, desired); pending {
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

func desiredReplicas(deployment *appsv1.Deployment) int32 {
	if deployment.Spec.Replicas == nil {
		return 1
	}
	return *deployment.Spec.Replicas
}

// pendingRollout reports why the rollout of the current pod template is not complete, or false
// when it is complete.
func pendingRollout(deployment *appsv1.Deployment, desiredReplicas int32) (string, bool) {
	// The deployment controller does not roll out a paused Deployment, so old replicas can
	// remain while it is paused.
	if deployment.Spec.Paused {
		return "", false
	}

	status := deployment.Status
	if status.UpdatedReplicas < desiredReplicas {
		return fmt.Sprintf("Waiting for rollout: %d/%d replicas updated", status.UpdatedReplicas, desiredReplicas), true
	}

	// With a surge, the controller can update every desired replica while old pods still run
	// and count toward ReadyReplicas. The rollout is not complete until those old pods are gone.
	if status.Replicas > status.UpdatedReplicas {
		return fmt.Sprintf(
			"Waiting for rollout: %d old replicas pending termination", status.Replicas-status.UpdatedReplicas,
		), true
	}

	return "", false
}

// DefaultGraceStatusHandler provides a default health assessment of the Deployment when it has not yet
// reached full readiness.
//
// It categorizes the current state into:
//   - GraceStatusHealthy: DefaultConvergingStatusHandler reports Healthy for the same Deployment.
//   - GraceStatusDown: No replicas are ready and the desired count (Spec.Replicas, 1 when nil) is
//     more than zero.
//   - GraceStatusDegraded: All other states. These include a deployment controller that has not
//     observed the current generation, a ready count that differs from the desired count, and an
//     incomplete rollout.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(deployment *appsv1.Deployment) (concepts.GraceStatusWithReason, error) {
	desired := desiredReplicas(deployment)

	if deployment.Status.ReadyReplicas == 0 && desired > 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: "No replicas are ready",
		}, nil
	}

	if deployment.Status.ObservedGeneration < deployment.Generation {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Waiting for deployment controller to observe latest spec",
		}, nil
	}

	// Use != rather than < so that grace and convergence agree on replica state.
	// Grace must not return Healthy for a state that convergence considers
	// non-healthy (e.g. ReadyReplicas > desired during scale-down).
	if deployment.Status.ReadyReplicas != desired {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Deployment partially available",
		}, nil
	}

	if reason, pending := pendingRollout(deployment, desired); pending {
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

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the Deployment
// when the parent component is suspended.
//
// It always returns false, meaning the Deployment is kept in the cluster but scaled to zero replicas.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendDeletionDecision. It can be reused within custom handlers.
func DefaultDeleteOnSuspendHandler(_ *appsv1.Deployment) bool {
	return false
}

// DefaultSuspendMutationHandler provides the default mutation applied to a Deployment when the component is suspended.
//
// It scales the Deployment to zero replicas by setting Spec.Replicas to 0.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(mutator *Mutator) error {
	mutator.EnsureReplicas(0)
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// It reports whether the Deployment has successfully scaled down to zero replicas
// by checking if Status.Replicas is 0.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(deployment *appsv1.Deployment) (concepts.SuspensionStatusWithReason, error) {
	if deployment.Status.Replicas == 0 {
		return concepts.SuspensionStatusWithReason{
			Status: concepts.SuspensionStatusSuspended,
			Reason: "Deployment scaled to zero",
		}, nil
	}

	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspending,
		Reason: fmt.Sprintf("Waiting for replicas to scale down, %d replicas still running.", deployment.Status.Replicas),
	}, nil
}
