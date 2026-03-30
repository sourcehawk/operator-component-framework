package component

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrerequisiteStatus represents whether a component-level prerequisite is satisfied.
type PrerequisiteStatus string

const (
	// PrerequisiteStatusMet indicates that the prerequisite is satisfied and the component may proceed.
	PrerequisiteStatusMet PrerequisiteStatus = "Met"
	// PrerequisiteStatusNotMet indicates that the prerequisite is not yet satisfied.
	PrerequisiteStatusNotMet PrerequisiteStatus = "NotMet"
)

// PrerequisiteResult carries the outcome of a prerequisite check along with a
// human-readable reason when the prerequisite is not met.
type PrerequisiteResult struct {
	// Status indicates whether the prerequisite is met or not.
	Status PrerequisiteStatus
	// Reason provides a human-readable explanation when the prerequisite is not met.
	Reason string
}

// Prerequisite is an initialization barrier for a component.
//
// Unlike resource-level guards, a prerequisite is evaluated only until the
// component successfully reconciles for the first time. Once the barrier is
// passed, the prerequisite is never re-evaluated, even if the underlying
// condition later becomes false.
//
// If the prerequisite is not met, the component does not reconcile any
// resources and reports a PrerequisiteNotMet condition.
type Prerequisite interface {
	// Check evaluates whether the prerequisite is currently satisfied.
	// The ReconcileContext provides access to the owner object and Kubernetes
	// client for implementations that need to inspect cluster state.
	Check(rec ReconcileContext) (PrerequisiteResult, error)
}

// dependsOnPrerequisite is a Prerequisite that checks whether a specific
// condition on the owner is True.
type dependsOnPrerequisite struct {
	conditionType ConditionType
}

// DependsOn returns a Prerequisite that is met when the named condition on the
// owner has Status=True. This is the common case for expressing that one
// component depends on another component being ready.
//
// The check reads the owner's in-memory status conditions from the
// ReconcileContext, so it does not perform any cluster reads.
func DependsOn(conditionType ConditionType) Prerequisite {
	return &dependsOnPrerequisite{
		conditionType: conditionType,
	}
}

// Check returns PrerequisiteStatusMet if the condition exists on the owner and
// has Status=True. Otherwise, it returns PrerequisiteStatusNotMet with a reason
// describing what is missing.
func (d *dependsOnPrerequisite) Check(rec ReconcileContext) (PrerequisiteResult, error) {
	owner := rec.Owner
	cond := meta.FindStatusCondition(*owner.GetStatusConditions(), string(d.conditionType))
	if cond == nil {
		return PrerequisiteResult{
			Status: PrerequisiteStatusNotMet,
			Reason: fmt.Sprintf(
				"Condition %q not found on %s/%s", d.conditionType, owner.GetKind(), owner.GetName(),
			),
		}, nil
	}

	if cond.Status != metav1.ConditionTrue {
		return PrerequisiteResult{
			Status: PrerequisiteStatusNotMet,
			Reason: fmt.Sprintf(
				"Prerequisite not met: waiting for condition %q to become True (currently %s: %s)",
				d.conditionType, cond.Status, cond.Message,
			),
		}, nil
	}

	return PrerequisiteResult{
		Status: PrerequisiteStatusMet,
	}, nil
}
