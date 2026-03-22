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
//   - OperationalStatusPending: conditions are absent, or ScalingActive is Unknown.
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
		}
	}

	// Conditions absent or ScalingActive has an unrecognized status
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusPending,
		Reason: "Waiting for HPA conditions to be populated",
	}, nil
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete the HPA
// when the parent component is suspended.
//
// It always returns true. HPA has no native suspend field; deleting prevents it from
// interfering with manually-scaled replicas while suspended.
//
// This function is used as the default handler by the Resource if no custom handler is registered
// via Builder.WithCustomSuspendDeletionDecision. It can be reused within custom handlers.
func DefaultDeleteOnSuspendHandler(_ *autoscalingv2.HorizontalPodAutoscaler) bool {
	return true
}

// DefaultSuspendMutationHandler provides the default mutation applied to an HPA when
// the component is suspended.
//
// It is a no-op because the HPA is deleted on suspend (DefaultDeleteOnSuspendHandler returns true).
//
// This function is used as the default handler by the Resource if no custom handler is registered
// via Builder.WithCustomSuspendMutation. It can be reused within custom handlers.
func DefaultSuspendMutationHandler(_ *Mutator) error {
	return nil
}

// DefaultSuspensionStatusHandler reports the suspension status of the HPA.
//
// It always returns Suspended with a reason indicating the HPA is deleted on suspend.
// Since the HPA is deleted during suspension, its status is inherently Suspended once
// the framework reaches this handler.
//
// This handler assumes DefaultDeleteOnSuspendHandler (delete-on-suspend) is in effect.
// If you override the deletion decision to keep the HPA, provide a custom suspension
// status handler via Builder.WithCustomSuspendStatus as well.
//
// This function is used as the default handler by the Resource if no custom handler is registered
// via Builder.WithCustomSuspendStatus. It can be reused within custom handlers.
func DefaultSuspensionStatusHandler(
	_ *autoscalingv2.HorizontalPodAutoscaler,
) (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "HorizontalPodAutoscaler deleted on suspend",
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
