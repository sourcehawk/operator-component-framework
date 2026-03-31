package component

import (
	"context"
	"strings"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type convergingResult struct {
	Resource Resource
	Status   convergingStatusWithReason
}

type convergeResults []convergingResult

// healthy returns true if all component resources are healthy.
func (c convergeResults) healthy() bool {
	for _, result := range c {
		if !result.Status.Status.healthy() {
			return false
		}
	}
	return true
}

func (c convergeResults) filterParticipators(resourceModeLookup map[string]ParticipationMode) convergeResults {
	var results []convergingResult

	for _, result := range c {
		// Guard-blocked results always participate in aggregation regardless of
		// the resource's participation mode, because a blocked guard halts the
		// entire reconciliation pipeline -- subsequent required resources are
		// skipped and their absence must not be mistaken for health.
		if result.Status.Status == convergingStatusGuardBlocked {
			results = append(results, result)
			continue
		}

		if mode, ok := resourceModeLookup[result.Resource.Identity()]; ok && mode == ParticipationModeRequired {
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
func (c convergeResults) convergeSummary() convergingStatusWithReason {
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

// graceSummary returns an aggregate grace status of all component resources that implement the Graceful interface.
// If multiple alive resources are present, the one with the most severe status takes precedence
// (Down > Degraded > healthy).
// If no resources implement the Graceful interface, it returns a Down status with an explanation,
// as the component cannot provide health information.
func (c convergeResults) graceSummary() (concepts.GraceStatusWithReason, error) {
	var maxStatus concepts.GraceStatus
	var reasons []string
	anyGraceful := false

	for _, result := range c {
		if graceful, ok := result.Resource.(concepts.Graceful); ok {
			anyGraceful = true

			current, err := graceful.GraceStatus()
			if err != nil {
				return concepts.GraceStatusWithReason{}, err
			}

			if current.Status.Priority() > maxStatus.Priority() {
				maxStatus = current.Status
				reasons = []string{current.Reason}
			} else if current.Status.Priority() == maxStatus.Priority() && current.Reason != "" {
				reasons = append(reasons, current.Reason)
			}
		}
	}

	if !anyGraceful {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusDown,
			Reason: "Component failed to converge within grace period",
		}, nil
	}

	if maxStatus == "" || maxStatus == concepts.GraceStatusHealthy {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusHealthy,
			Reason: "All resources healthy.",
		}, nil
	}

	return concepts.GraceStatusWithReason{
		Status: maxStatus,
		Reason: strings.Join(reasons, "; "),
	}, nil
}

// graceExpired returns true if the grace duration of the component has been exceeded
// since the last condition transition. If gracePeriod is 0, grace never expires (infinite grace).
func graceExpired(gracePeriod time.Duration, transition time.Time) bool {
	if gracePeriod == 0 {
		return false
	}
	return time.Since(transition) > gracePeriod
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
//  2. Initialization: If no prior condition exists (Status=Unknown), it is initialized
//     from the current aggregate status (Creating, Updating, etc.).
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
//     - Once graceExpired() is true, the status transitions to Down or Degraded
//     based on the aggregate status of resources that implement the Alive interface.
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
	ctx context.Context, owner OperatorCRD, results convergeResults, gracePeriod time.Duration, previousCondition Condition,
) Condition {
	generation := owner.GetGeneration()
	conditionType := ConditionType(previousCondition.Type)

	if results.healthy() {
		return conditionReady(conditionType, generation)
	}

	// No condition is set, we create the initial condition from the converge summary
	if previousCondition.Reason == string(Unknown) {
		summary := results.convergeSummary()
		return convergingCondition(conditionType, summary, generation)
	}

	// Convert the previous condition reason to a component status
	status := Status(previousCondition.Reason)

	// Get the summary for an updated description of why we're here
	convergeSummary := results.convergeSummary()

	// If we're no longer healthy (results.healthy() = false) but have a Healthy reason on the previous condition,
	// calculate a new condition from the converge summary
	if status.Healthy() {
		return convergingCondition(conditionType, convergeSummary, generation)
	}

	// Update the status if the previous one should not be retained across reconciles
	if !status.sticky() {
		status = Status(convergeSummary.Status)
	}

	logger := log.FromContext(ctx)

	// If the grace period expired, and we're still not healthy, set a down/degraded status
	if graceExpired(gracePeriod, previousCondition.LastTransitionTime.Time) {
		summary, err := results.graceSummary()
		if err != nil {
			logger.Error(err, "failed to get grace summary for component")
			return conditionError(conditionType, err, generation)
		}

		if summary.Status == concepts.GraceStatusDown || summary.Status == concepts.GraceStatusDegraded {
			return graceCondition(conditionType, summary, generation)
		}

		// One of the resource's status handlers is misconfigured. Convergence status and
		// grace status are determined in the same loop with no refetch of the underlying
		// resource between them. A resource should never report non-healthy for convergence
		// and Healthy for grace in the same reconcile. Log a warning and continue onto other
		// code paths rather than escalating.
		logger.V(0).Info(
			"component progressor encountered GraceStatus=Healthy on unready component after detecting grace expiry",
		)
	}

	// Copy old condition and update
	out := previousCondition
	out.Reason = string(status)
	out.Message = convergeSummary.Reason
	out.ObservedGeneration = generation

	return out
}
