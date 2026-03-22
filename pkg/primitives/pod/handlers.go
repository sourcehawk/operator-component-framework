package pod

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
)

// DefaultConvergingStatusHandler is the default logic for determining if a Pod has reached its desired state.
//
// It considers a Pod:
//   - Healthy: when Status.Phase is Running AND all container statuses report Ready.
//   - Failing: when any container is in CrashLoopBackOff or has terminated with an error.
//   - Updating: when the converging operation indicates the pod is being recreated.
//   - Creating: when Status.Phase is Pending and no restart failures are detected.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomConvergeStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultConvergingStatusHandler(
	op concepts.ConvergingOperation, pod *corev1.Pod,
) (concepts.AliveStatusWithReason, error) {
	// Check for failing containers first
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			return concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusFailing,
				Reason: "Container " + cs.Name + " is in CrashLoopBackOff",
			}, nil
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusFailing,
				Reason: "Container " + cs.Name + " terminated with error",
			}, nil
		}
	}

	// Check if pod is running and all containers are ready
	if pod.Status.Phase == corev1.PodRunning {
		allReady := true
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				allReady = false
				break
			}
		}
		if allReady {
			return concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusHealthy,
				Reason: "Pod is running and all containers are ready",
			}, nil
		}
	}

	// Handle terminal phases explicitly.
	switch pod.Status.Phase {
	case corev1.PodFailed:
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusFailing,
			Reason: "Pod has failed",
		}, nil
	case corev1.PodSucceeded:
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusHealthy,
			Reason: "Pod has completed successfully",
		}, nil
	}

	// Pod is Running with not-all-ready containers, or Pending.
	// Determine status based on converging operation.
	switch op {
	case concepts.ConvergingOperationUpdated:
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusUpdating,
			Reason: "Pod is being recreated",
		}, nil
	case concepts.ConvergingOperationCreated:
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusCreating,
			Reason: "Pod is starting",
		}, nil
	default:
		if pod.Status.Phase == corev1.PodRunning {
			return concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusCreating,
				Reason: "Pod running but not all containers ready",
			}, nil
		}
		return concepts.AliveStatusWithReason{
			Status: concepts.AliveConvergingStatusCreating,
			Reason: "Pod is pending",
		}, nil
	}
}

// DefaultGraceStatusHandler provides a default health assessment of the Pod when it has not yet
// reached full readiness.
//
// It categorizes the current state into:
//   - GraceStatusHealthy: Pod is Running and all containers are Ready.
//   - GraceStatusDegraded: Pod is Running but not all containers are Ready, or container readiness is unknown.
//   - GraceStatusDown: Pod phase is not Running.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment the default behavior.
func DefaultGraceStatusHandler(pod *corev1.Pod) (concepts.GraceStatusWithReason, error) {
	if pod.Status.Phase != corev1.PodRunning {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: "Pod is not running",
		}, nil
	}

	if len(pod.Status.ContainerStatuses) == 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Pod running but container readiness unknown",
		}, nil
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if !cs.Ready {
			return concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusDegraded,
				Reason: "Pod running but not all containers ready",
			}, nil
		}
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusHealthy,
		Reason: "Pod running and all containers ready",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the Pod
// when the parent component is suspended.
//
// It always returns true because pods cannot be paused — they must be deleted when suspended.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendDeletionDecision. It can be reused within custom handlers.
func DefaultDeleteOnSuspendHandler(_ *corev1.Pod) bool {
	return true
}

// DefaultSuspendMutationHandler provides the default mutation applied to a Pod when the component is suspended.
//
// It is a no-op because pods are deleted on suspend rather than mutated.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(_ *Mutator) error {
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// It always reports Suspended because pod suspension is handled by deletion — once
// the framework decides to delete the pod, suspension is considered complete.
//
// This function is used as the default handler by the Resource if no custom handler is registered via
// Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(_ *corev1.Pod) (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "Pod deleted on suspend",
	}, nil
}
