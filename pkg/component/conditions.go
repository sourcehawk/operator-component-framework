package component

import (
	"context"

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

// convergingCondition returns a component condition representing a progressing state.
// If the aggregate status is Ready, the condition Status is set to True; otherwise, it is False.
func convergingCondition(
	component ConditionType, converging ConvergingStatusWithReason, observedGeneration int64,
) Condition {
	status := metav1.ConditionFalse
	if converging.Status == ConvergingStatusReady {
		status = metav1.ConditionTrue
	}

	return Condition{
		Type:               string(component),
		Status:             status,
		Reason:             string(Status(converging.Status)),
		Message:            converging.Reason,
		ObservedGeneration: observedGeneration,
	}
}

// suspendingCondition returns a component condition representing a suspension progress.
// If the component is fully suspended (SuspensionStatusSuspended), the condition status is set to True.
func suspendingCondition(component ConditionType, suspending SuspensionStatusWithReason, observedGeneration int64) Condition {
	status := metav1.ConditionFalse
	if suspending.Status == SuspensionStatusSuspended {
		status = metav1.ConditionTrue
	}

	return Condition{
		Type:               string(component),
		Status:             status,
		Reason:             string(Status(suspending.Status)),
		Message:            suspending.Reason,
		ObservedGeneration: observedGeneration,
	}
}

// graceCondition returns a component condition representing a degraded or down state
// once the grace period has expired. The condition status is always False as the component
// is not considered Ready in these states.
func graceCondition(component ConditionType, gracing GraceStatusWithReason, observedGeneration int64) Condition {
	message := "Component is down: "
	if gracing.Status == GraceStatusDegraded {
		message = "Component is degraded: "
	}

	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionFalse,
		Reason:             string(Status(gracing.Status)),
		ObservedGeneration: observedGeneration,
		Message:            message + gracing.Reason,
	}
}

// conditionReady returns a component condition representing a Ready state.
func conditionReady(component ConditionType, observedGeneration int64) Condition {
	return Condition{
		Type:               string(component),
		Status:             metav1.ConditionTrue,
		Reason:             string(Ready),
		ObservedGeneration: observedGeneration,
		Message:            "Component is ready.",
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

// setStatusCondition updates the component condition on the owner CRD's status.
// It performs three key actions:
//  1. Updates the condition in the owner's status condition slice.
//  2. Records the condition change in the metrics recorder.
//  3. If the condition has changed, it persists the update to the Kubernetes API.
func setStatusCondition(
	ctx context.Context, rec ReconcileContext, cond Condition,
) error {
	changed := meta.SetStatusCondition(rec.Owner.GetStatusConditions(), metav1.Condition(cond))
	updated := meta.FindStatusCondition(*rec.Owner.GetStatusConditions(), cond.Type)

	if updated != nil {
		rec.Metrics.RecordConditionFor(
			rec.Owner.GetKind(), rec.Owner, updated.Type, string(updated.Status),
			updated.Reason, updated.LastTransitionTime.Time,
		)
	}

	if changed {
		if err := rec.Client.Status().Update(ctx, rec.Owner); err != nil {
			return err
		}
	}

	return nil
}
