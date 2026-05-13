package component

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionType represents the type of condition in the Kubernetes status.
// It is used to identify the specific component's condition on the owner CRD.
type ConditionType string

// Condition is a type alias for metav1.Condition.
// It represents a single condition for a component.
type Condition metav1.Condition

// ConditionType returns the component-specific type for the condition.
func (c Condition) ConditionType() ConditionType {
	return ConditionType(c.Type)
}

// ComponentStatus returns the component's internal status (Reason) from the condition.
func (c Condition) ComponentStatus() Status {
	return Status(c.Reason)
}

// convergingCondition returns a component condition representing a Progressing state.
// If the aggregate status is Ready, the condition Status is set to True; otherwise, it is False.
func convergingCondition(
	component ConditionType, converging convergingStatusWithReason, observedGeneration int64,
) Condition {
	status := metav1.ConditionFalse
	if converging.Status.healthy() {
		status = metav1.ConditionTrue
	}

	return Condition{
		Type:               string(component),
		Status:             status,
		Reason:             string(converging.Status),
		Message:            converging.Reason,
		ObservedGeneration: observedGeneration,
	}
}

// suspendingCondition returns a component condition representing a suspension progress.
// If the component is fully suspended (SuspensionStatusSuspended), the condition status is set to True.
func suspendingCondition(component ConditionType, suspending concepts.SuspensionStatusWithReason, observedGeneration int64) Condition {
	status := metav1.ConditionFalse
	if suspending.Status == concepts.SuspensionStatusSuspended {
		status = metav1.ConditionTrue
	}

	return Condition{
		Type:               string(component),
		Status:             status,
		Reason:             string(suspending.Status),
		Message:            suspending.Reason,
		ObservedGeneration: observedGeneration,
	}
}

// graceCondition returns a component condition representing a degraded or down state
// once the grace period has expired. The condition status is always False as the component
// is not considered Ready in these states.
func graceCondition(component ConditionType, gracing concepts.GraceStatusWithReason, observedGeneration int64) Condition {
	message := "Component is down: "
	if gracing.Status == concepts.GraceStatusDegraded {
		message = "Component is degraded: "
	}

	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionFalse,
		Reason:             string(gracing.Status),
		ObservedGeneration: observedGeneration,
		Message:            message + gracing.Reason,
	}
}

// conditionReady returns a component condition representing a Ready state.
func conditionReady(component ConditionType, observedGeneration int64) Condition {
	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionTrue,
		Reason:             string(Healthy),
		ObservedGeneration: observedGeneration,
		Message:            "Component is healthy.",
	}
}

// conditionError returns a component condition representing an Error state.
func conditionError(component ConditionType, err error, observedGeneration int64) Condition {
	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionFalse,
		Reason:             string(Error),
		ObservedGeneration: observedGeneration,
		Message:            err.Error(),
	}
}

// conditionFeatureGateError returns a component condition indicating that the
// feature gate check failed with an error. This is distinct from conditionError
// so that the prerequisite initialization barrier can distinguish pre-prerequisite
// failures from post-prerequisite failures.
func conditionFeatureGateError(component ConditionType, err error, observedGeneration int64) Condition {
	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionFalse,
		Reason:             string(FeatureGateError),
		ObservedGeneration: observedGeneration,
		Message:            err.Error(),
	}
}

// conditionDisabled returns a component condition representing a Disabled state.
// The condition status is True because a disabled component is in its expected state,
// consistent with the convention used for suspended components.
func conditionDisabled(component ConditionType, observedGeneration int64) Condition {
	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionTrue,
		Reason:             string(Disabled),
		ObservedGeneration: observedGeneration,
		Message:            "Component is disabled.",
	}
}

// conditionPrerequisiteNotMet returns a component condition indicating that a
// component-level prerequisite has not been satisfied. The component has not
// reconciled any resources.
func conditionPrerequisiteNotMet(component ConditionType, reason string, observedGeneration int64) Condition {
	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionFalse,
		Reason:             string(PrerequisiteNotMet),
		ObservedGeneration: observedGeneration,
		Message:            reason,
	}
}

// conditionUnknown returns a component condition representing an Unknown state.
func conditionUnknown(component ConditionType, observedGeneration int64) Condition {
	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionFalse,
		Reason:             string(Unknown),
		ObservedGeneration: observedGeneration,
		Message:            "Component has not been reconciled yet.",
	}
}

// applyStatusCondition updates the component condition on the owner's in-memory
// status conditions. It does not call the Kubernetes API and does not record
// metrics; persistence and metrics recording are performed once per reconcile
// by [FlushStatus]. Keeping this function purely in-memory is what allows a
// controller with several components to share a single status write at the end
// of reconciliation instead of racing multiple writes against the same owner.
func applyStatusCondition(rec ReconcileContext, cond Condition) {
	meta.SetStatusCondition(rec.Owner.GetStatusConditions(), metav1.Condition(cond))
}
