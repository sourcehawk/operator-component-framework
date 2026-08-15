package component

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Aggregate collapses the conditions of several components into a single
// owner-level condition of the given type.
//
// Two independent decisions produce the result:
//
//  1. Truth by unanimity. The returned Status is [metav1.ConditionTrue] if and
//     only if every component's condition is True. Priority never decides True
//     or False.
//  2. Reason by priority. The Reason and Message come from the governing
//     component: the one with the highest [Status.Priority] among the conditions
//     whose status is not True, if any exist, otherwise the highest among all of
//     them. Argument order breaks ties, so the result is deterministic.
//
// Keeping the two decisions apart is what makes the result trustworthy. Deriving
// truth from priority instead would report True with reason Suspended for an
// owner with a failing component, because [Suspended] has priority 15 and maps to
// condition status True while [AliveFailing] has priority 13 and maps to False.
// Unanimity answers "is everything in its expected state", priority answers "what
// should the reader look at first".
//
// The Message names the governing component and carries its message through, in
// the form "<component name>: <component message>", so the reader knows which
// component explains the aggregate. If the governing condition carries no
// message, the component name alone is used.
//
// ObservedGeneration is the owner's generation.
//
// Each condition is read with [Component.GetCondition], which synthesizes an
// Unknown condition with status False for a component that has not reconciled
// yet. A freshly created owner therefore never reports True.
//
// Behavior worth knowing before relying on the result:
//
//   - Passing no components returns status False with reason [Unknown]. A caller
//     that aggregates nothing must never report ready.
//   - Partial suspension reports True with reason [Suspended]. A suspended
//     component is in its expected state and cannot make the aggregate false. An
//     operator that wants owner-level suspension as an explicit state branches on
//     its own suspend field before calling Aggregate.
//   - A component disabled by a feature gate reports True with reason [Disabled]
//     and therefore counts as ready, for the same reason.
//   - Only the components passed in are considered. A component the controller
//     has stopped building leaves a stale condition on the owner that Aggregate
//     ignores but users still see, because [FlushStatus] merges conditions by type
//     and never prunes.
//
// The returned [Condition] is a plain struct. A caller that wants different prose
// can adjust the Message before staging it:
//
//	cond := component.Aggregate("Ready", owner, brokerComp, gatewayComp)
//	meta.SetStatusCondition(owner.GetStatusConditions(), metav1.Condition(cond))
func Aggregate(conditionType ConditionType, owner OperatorCRD, comps ...*Component) Condition {
	generation := owner.GetGeneration()

	if len(comps) == 0 {
		return Condition{
			Type:               string(conditionType),
			Status:             metav1.ConditionFalse,
			Reason:             string(Unknown),
			Message:            "No components were aggregated.",
			ObservedGeneration: generation,
		}
	}

	var governing *Component
	var governingCondition Condition
	unanimous := true

	for _, comp := range comps {
		cond := comp.GetCondition(owner)

		if cond.Status != metav1.ConditionTrue {
			unanimous = false
		}

		if governing == nil || supersedes(cond, governingCondition) {
			governing, governingCondition = comp, cond
		}
	}

	status := metav1.ConditionFalse
	if unanimous {
		status = metav1.ConditionTrue
	}

	return Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             governingCondition.Reason,
		Message:            aggregateMessage(governing.GetName(), governingCondition.Message),
		ObservedGeneration: generation,
	}
}

// supersedes reports whether candidate should replace current as the governing
// condition. A condition whose status is not True always supersedes one that is
// True, so the reason is drawn from the group that explains why the aggregate is
// false. Within the same group the higher Status.Priority wins. A tie leaves
// current in place, which makes argument order the tie-breaker.
func supersedes(candidate, current Condition) bool {
	candidateTrue := candidate.Status == metav1.ConditionTrue
	currentTrue := current.Status == metav1.ConditionTrue

	if candidateTrue != currentTrue {
		return currentTrue
	}

	return candidate.ComponentStatus().Priority() > current.ComponentStatus().Priority()
}

// aggregateMessage renders the aggregate message from the governing component's
// name and message. A governing condition without a message reduces to the name.
func aggregateMessage(name, message string) string {
	if message == "" {
		return name
	}
	return name + ": " + message
}
