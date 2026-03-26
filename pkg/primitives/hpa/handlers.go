package hpa

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
)

// DefaultOperationalStatusHandler is the default logic for determining the operational status
// of a HorizontalPodAutoscaler.
//
// It inspects Status.Conditions to classify the HPA's state:
//   - OperationalStatusOperational: condition ScalingActive is True.
//   - OperationalStatusPending: conditions are absent, ScalingActive is missing, or ScalingActive is Unknown.
//   - OperationalStatusFailing: condition ScalingActive is False, or condition AbleToScale is False.
//
// This function is used as the default handler by the Resource if no custom handler is registered
// via Builder.WithCustomOperationalStatus. It can be reused within custom handlers to augment
// the default behavior.
func DefaultOperationalStatusHandler(
	_ concepts.ConvergingOperation, hpa *autoscalingv2.HorizontalPodAutoscaler,
) (concepts.OperationalStatusWithReason, error) {
	scalingActive := findCondition(hpa.Status.Conditions, autoscalingv2.ScalingActive)
	ableToScale := findCondition(hpa.Status.Conditions, autoscalingv2.AbleToScale)

	// Check for failing conditions first
	if ableToScale != nil && ableToScale.Status == corev1.ConditionFalse {
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusFailing,
			Reason: conditionReason(ableToScale, "AbleToScale is False"),
		}, nil
	}

	if scalingActive != nil {
		switch scalingActive.Status {
		case corev1.ConditionTrue:
			return concepts.OperationalStatusWithReason{
				Status: concepts.OperationalStatusOperational,
				Reason: conditionReason(scalingActive, "ScalingActive is True"),
			}, nil
		case corev1.ConditionFalse:
			return concepts.OperationalStatusWithReason{
				Status: concepts.OperationalStatusFailing,
				Reason: conditionReason(scalingActive, "ScalingActive is False"),
			}, nil
		case corev1.ConditionUnknown:
			return concepts.OperationalStatusWithReason{
				Status: concepts.OperationalStatusPending,
				Reason: conditionReason(scalingActive, "ScalingActive is Unknown"),
			}, nil
		default:
			return concepts.OperationalStatusWithReason{
				Status: concepts.OperationalStatusPending,
				Reason: conditionReason(scalingActive, "ScalingActive has unrecognized status"),
			}, nil
		}
	}

	// Distinguish between no conditions at all and ScalingActive missing/unrecognized
	if len(hpa.Status.Conditions) == 0 {
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusPending,
			Reason: "Waiting for HPA conditions to be populated",
		}, nil
	}

	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusPending,
		Reason: "Waiting for ScalingActive condition on HPA",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the HPA
// when the parent component is suspended.
//
// It always returns true. A retained HPA would conflict with the suspension of its scale
// target (e.g. a Deployment scaled to zero) because the Kubernetes HPA controller
// continuously enforces minReplicas and would scale the target back up. Deleting the HPA
// prevents this interference and guarantees clean suspension semantics. On resume the
// framework recreates the HPA with the desired spec.
//
// Override this via Builder.WithCustomSuspendDeletionDecision if your use case requires
// the HPA to be retained during suspension.
//
// This function is used as the default handler by the Resource if no custom handler is registered
// via Builder.WithCustomSuspendDeletionDecision. It can be reused within custom handlers.
func DefaultDeleteOnSuspendHandler(_ *autoscalingv2.HorizontalPodAutoscaler) bool {
	return true
}

// DefaultSuspendMutationHandler provides the default mutation applied to an HPA when
// the component is suspended.
//
// It is a no-op. The default suspension behavior deletes the HPA
// (DefaultDeleteOnSuspendHandler returns true), so no spec mutations are needed before
// deletion.
//
// This function is used as the default handler by the Resource if no custom handler is registered
// via Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(_ *Mutator) error {
	return nil
}

// DefaultSuspensionStatusHandler reports the suspension status of the HPA.
//
// It always returns Suspended immediately. The default suspension behaviour deletes the
// HPA to prevent it from interfering with the scale target's suspension. Because
// deletion is handled by the framework after this status is reported, no additional
// work is required and the status is always Suspended.
//
// The reason string is intentionally deletion-agnostic so that this handler remains
// accurate even when the deletion decision is overridden via
// Builder.WithCustomSuspendDeletionDecision. If you need a reason that reflects your
// custom deletion behaviour, override this handler via Builder.WithCustomSuspendStatus.
//
// This function is used as the default handler by the Resource if no custom handler is registered
// via Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(
	_ *autoscalingv2.HorizontalPodAutoscaler,
) (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "HorizontalPodAutoscaler suspended to prevent scaling interference",
	}, nil
}

// DefaultGraceStatusHandler is the default logic for determining the grace status
// of a HorizontalPodAutoscaler after the allowed grace period has expired.
//
// It inspects Status.Conditions to classify the HPA's health:
//   - GraceStatusHealthy: condition ScalingActive is True.
//   - GraceStatusDown: condition AbleToScale is False, or condition ScalingActive is False.
//   - GraceStatusDegraded: ScalingActive is Unknown, missing, or no conditions are present.
//
// This function is used as the default handler by the Resource if no custom handler is registered
// via Builder.WithCustomGraceStatus. It can be reused within custom handlers to augment
// the default behavior.
func DefaultGraceStatusHandler(
	hpa *autoscalingv2.HorizontalPodAutoscaler,
) (concepts.GraceStatusWithReason, error) {
	scalingActive := findCondition(hpa.Status.Conditions, autoscalingv2.ScalingActive)
	ableToScale := findCondition(hpa.Status.Conditions, autoscalingv2.AbleToScale)

	// Check for down conditions first
	if ableToScale != nil && ableToScale.Status == corev1.ConditionFalse {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: conditionReason(ableToScale, "AbleToScale is False"),
		}, nil
	}

	if scalingActive != nil {
		switch scalingActive.Status {
		case corev1.ConditionTrue:
			return concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusHealthy,
				Reason: conditionReason(scalingActive, "ScalingActive is True"),
			}, nil
		case corev1.ConditionFalse:
			return concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusDown,
				Reason: conditionReason(scalingActive, "ScalingActive is False"),
			}, nil
		default:
			return concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusDegraded,
				Reason: conditionReason(scalingActive, "ScalingActive is Unknown"),
			}, nil
		}
	}

	// No conditions or ScalingActive missing
	if len(hpa.Status.Conditions) == 0 {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDegraded,
			Reason: "Waiting for HPA conditions to be populated",
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDegraded,
		Reason: "Waiting for ScalingActive condition on HPA",
	}, nil
}

// findCondition returns the first condition matching the given type, or nil.
func findCondition(
	conditions []autoscalingv2.HorizontalPodAutoscalerCondition,
	condType autoscalingv2.HorizontalPodAutoscalerConditionType,
) *autoscalingv2.HorizontalPodAutoscalerCondition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

// conditionReason returns the condition's message if present, otherwise a fallback.
func conditionReason(cond *autoscalingv2.HorizontalPodAutoscalerCondition, fallback string) string {
	if cond.Message != "" {
		return cond.Message
	}
	return fallback
}
