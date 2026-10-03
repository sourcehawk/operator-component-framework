package component

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type reconcileResult struct {
	Entry       reconcileEntry
	Status      convergingStatusWithReason
	GraceStatus *concepts.GraceStatusWithReason
}

type reconcileResults []reconcileResult

// healthy returns true if all component resources are healthy.
func (c reconcileResults) healthy() bool {
	for _, result := range c {
		if !result.Status.Status.healthy() {
			return false
		}
	}
	return true
}

func (c reconcileResults) filterParticipators() reconcileResults {
	var results []reconcileResult

	for _, result := range c {
		// Guard-blocked results always participate in aggregation regardless of
		// the resource's participation mode, because a blocked guard halts the
		// entire reconciliation pipeline -- subsequent required resources are
		// skipped and their absence must not be mistaken for health.
		if result.Status.Status == convergingStatusGuardBlocked {
			results = append(results, result)
			continue
		}

		if result.Entry.Options.ParticipationMode == ParticipationModeRequired {
			results = append(results, result)
		}
	}

	return results
}

// convergeSummary determines the aggregate converging status of all component resources.
// It iterates through the results and picks the status with the highest priority level
// (e.g., Scaling > Updating > Creating > Ready).
// If multiple resources share the same highest priority level, their reasons are concatenated
// to provide a comprehensive summary.
func (c reconcileResults) convergeSummary() convergingStatusWithReason {
	var maxStatus convergingStatus
	var reasons []string

	for _, result := range c {
		if result.Status.Status.priority() > maxStatus.priority() {
			maxStatus = result.Status.Status
		}
	}

	if maxStatus == "" {
		maxStatus = convergingStatusAliveHealthy
	}

	for _, result := range c {
		if result.Status.Status.severity() == maxStatus.severity() && result.Status.Reason != "" {
			reasons = append(reasons, result.Status.Reason)
		}
	}

	if maxStatus.healthy() {
		return convergingStatusWithReason{
			Status: maxStatus,
			Reason: "All resources healthy.",
		}
	}

	return convergingStatusWithReason{
		Status: maxStatus,
		Reason: strings.Join(reasons, "; "),
	}
}

// evaluateGrace populates the GraceStatus field on each result whose resource
// implements the Graceful interface, and on each Blocked result. Other results
// without a Graceful resource are left with a nil GraceStatus.
func (c reconcileResults) evaluateGrace() error {
	for i := range c {
		// A blocked resource was neither applied nor read, so its object holds
		// only desired state and its own grace handler would grade an empty
		// status instead of the reason it is blocked.
		if c[i].Status.Status == convergingStatusGuardBlocked {
			reason := c[i].Status.Reason
			if reason == "" {
				reason = fmt.Sprintf("%s is blocked", c[i].Entry.Resource.Identity())
			}

			c[i].GraceStatus = &concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusDown,
				Reason: reason,
			}
			continue
		}

		graceful, ok := c[i].Entry.Resource.(concepts.Graceful)
		if !ok {
			continue
		}
		status, err := graceful.GraceStatus()
		if err != nil {
			return fmt.Errorf("failed to evaluate grace status for resource %s: %w", c[i].Entry.Resource.Identity(), err)
		}
		c[i].GraceStatus = &status
	}
	return nil
}

// graceSummary aggregates the evaluated grace statuses into a single result.
// The most severe status wins (Down > Degraded > Healthy). If no result
// carries a grace status, it returns Down. Must be called after evaluateGrace.
func (c reconcileResults) graceSummary() concepts.GraceStatusWithReason {
	var maxStatus concepts.GraceStatus
	var reasons []string
	anyGraceful := false

	for _, result := range c {
		if result.GraceStatus == nil {
			continue
		}
		anyGraceful = true

		if result.GraceStatus.Status.Priority() > maxStatus.Priority() {
			maxStatus = result.GraceStatus.Status
			reasons = []string{result.GraceStatus.Reason}
		} else if result.GraceStatus.Status.Priority() == maxStatus.Priority() && result.GraceStatus.Reason != "" {
			reasons = append(reasons, result.GraceStatus.Reason)
		}
	}

	if !anyGraceful {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: "Component failed to converge within grace period.",
		}
	}

	if maxStatus == "" || maxStatus == concepts.GraceStatusHealthy {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusHealthy,
			Reason: "All resources healthy.",
		}
	}

	return concepts.GraceStatusWithReason{
		Status: maxStatus,
		Reason: strings.Join(reasons, "; "),
	}
}

// graceExpired returns true if the grace duration of the component has been exceeded
// since the last condition transition. If gracePeriod is 0, grace never expires (infinite grace).
func graceExpired(gracePeriod time.Duration, transition time.Time) bool {
	deadline, ok := graceDeadline(gracePeriod, transition)
	return ok && time.Now().After(deadline)
}

// graceDeadline returns the last instant of a grace period that started at
// transition. It returns false for a grace period of 0, which never ends.
func graceDeadline(gracePeriod time.Duration, transition time.Time) (time.Time, bool) {
	if gracePeriod == 0 {
		return time.Time{}, false
	}
	return transition.Add(gracePeriod), true
}

// graceRemaining returns the delay after which a reconcile grades cond because
// its grace period has ended, and false when no reconcile will grade it.
func graceRemaining(cond Condition, gracePeriod time.Duration, now time.Time) (time.Duration, bool) {
	status := Status(cond.Reason)
	if !status.graceTimed() || status == Degraded || status == Down {
		return 0, false
	}

	deadline, ok := graceDeadline(gracePeriod, cond.LastTransitionTime.Time)
	if !ok {
		return 0, false
	}

	// graceExpired needs a time strictly after the deadline. The API server
	// stores LastTransitionTime in whole seconds, so the reconcile can read a
	// transition time up to one second earlier than cond holds, which only
	// moves its deadline earlier. The first whole second after the deadline is
	// therefore late enough for both values.
	requeueAt := deadline.Truncate(time.Second).Add(time.Second)

	remaining := requeueAt.Sub(now)
	if remaining <= 0 {
		return 0, false
	}
	return remaining, true
}

// newConvergingStatusCondition derives the next component condition based on the
// current resource results and the previously reported condition.
//
// This function implements the core state machine for component readiness. It avoids
// "condition flapping" by using a "sticky" state model during the grace period.
//
// Reconciliation Logic:
//
//  1. Immediate Ready: If all resources are Ready, the component becomes Ready immediately.
//
//  2. Initialization: If no prior condition exists (Reason=Unknown), or the previous
//     condition was produced outside this state machine (Disabled, FeatureGateError,
//     PrerequisiteNotMet, or a suspension reason), it is initialized from the current
//     aggregate status (Creating, Updating, etc.). The previous condition's Status and
//     LastTransitionTime are not carried forward, so a component that was disabled
//     or suspended does not inherit a True status or an expired grace period.
//
//  3. Recovery from Ready: If the previous status was Ready but resources are now unready,
//     the status transitions to the current aggregate status.
//
//  4. Progressing State (The Grace Period):
//     - While the status is Creating, Updating, or Scaling, the condition Reason remains
//     stable (e.g., "Creating") even if the underlying aggregate status fluctuates
//     between different Progressing states.
//     - The Message field is updated in every loop to provide current details.
//     - This state is held until the component becomes Ready or the gracePeriod expires.
//     - Other states indicating non-healthiness do not remain stable.
//
//  5. Grace Expiry (Transition to Failure):
//     - Once graceExpired() is true, a Down or Degraded aggregate grace status (see evaluateGrace)
//     becomes the condition. A Healthy aggregate leaves the converging condition in place.
//
//  6. Sticky Failure:
//     - Once Down or Degraded, the component stays in that failure state until it either
//     recovers (all resources Ready) or the severity of the failure changes
//     (e.g., transitions from Degraded to Down).
//
//  7. Steady State Update:
//     - If no state transition occurs, the previous condition is returned with an
//     updated ObservedGeneration and a refreshed Message.
//
// If health aggregation (GraceStatus) fails for any resource, an Error condition is returned.
func newConvergingStatusCondition(
	ctx context.Context, owner OperatorCRD, results reconcileResults, gracePeriod time.Duration, previousCondition Condition,
) Condition {
	generation := owner.GetGeneration()
	conditionType := ConditionType(previousCondition.Type)

	if results.healthy() {
		return conditionReady(conditionType, generation)
	}

	// Convert the previous condition reason to a component status
	status := Status(previousCondition.Reason)

	// Get the summary for an updated description of why we're here
	convergeSummary := results.convergeSummary()

	// The grace period does not run for the previous condition: it was not
	// produced by this state machine (no condition yet, feature gate disabled,
	// prerequisite barrier, suspension), or it was ready and the resources are
	// no longer healthy. Its status and transition time say nothing about this
	// convergence, so derive a new condition from the converge summary instead
	// of building on it.
	if !status.graceTimed() {
		return convergingCondition(conditionType, convergeSummary, generation)
	}

	// Update the status if the previous one should not be retained across reconciles
	if !status.sticky() {
		status = Status(convergeSummary.Status)
	}

	logger := log.FromContext(ctx)

	// If the grace period expired, and we're still not healthy, set a down/degraded status
	if graceExpired(gracePeriod, previousCondition.LastTransitionTime.Time) {
		if err := results.evaluateGrace(); err != nil {
			logger.Error(err, "failed to evaluate grace status for component")
			return conditionError(conditionType, err, generation)
		}

		summary := results.graceSummary()
		if summary.Status == concepts.GraceStatusDown || summary.Status == concepts.GraceStatusDegraded {
			return graceCondition(conditionType, summary, generation)
		}

		// Log per-resource inconsistencies where grace reports Healthy but
		// convergence reports non-healthy. Both handlers evaluate the same object
		// in the same reconcile loop, so this indicates a handler misconfiguration.
		for _, result := range results {
			if result.GraceStatus == nil {
				continue
			}
			if result.GraceStatus.Status != concepts.GraceStatusHealthy || result.Status.Status.healthy() {
				continue
			}
			if result.Entry.Options.SuppressGraceInconsistencyWarning {
				continue
			}
			logger.V(0).Info(
				"Grace inconsistency detected: resource grace status is Healthy but convergence status is non-healthy",
				"resource", result.Entry.Resource.Identity(),
				"convergeStatus", result.Status,
				"graceStatus", result.GraceStatus,
			)
		}
	}

	// Copy old condition and update
	out := previousCondition
	out.Reason = string(status)
	out.Message = convergeSummary.Reason
	out.ObservedGeneration = generation

	return out
}
