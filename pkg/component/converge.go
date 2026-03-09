package component

import (
	"context"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type convergingResult struct {
	Resource Resource
	Status   ConvergingStatusWithReason
}

type convergeResults []convergingResult

// ready returns true if all component resources are Ready.
func (c convergeResults) ready() bool {
	for _, result := range c {
		if result.Status.Status != ConvergingStatusReady {
			return false
		}
	}
	return true
}

// convergeSummary determines the aggregate converging status of all component resources.
// It iterates through the results and picks the status with the highest priority level
// (e.g., Scaling > Updating > Creating > Ready).
// If multiple resources share the same highest priority level, their reasons are concatenated
// to provide a comprehensive summary.
func (c convergeResults) convergeSummary() ConvergingStatusWithReason {
	var maxStatus ConvergingStatus
	var reasons []string

	for _, result := range c {
		if result.Status.Status.level() > maxStatus.level() {
			maxStatus = result.Status.Status
			reasons = []string{result.Status.Reason}
		} else if result.Status.Status.level() == maxStatus.level() && result.Status.Reason != "" {
			reasons = append(reasons, result.Status.Reason)
		}
	}

	if maxStatus == "" || maxStatus == ConvergingStatusReady {
		return ConvergingStatusWithReason{
			Status: ConvergingStatusReady,
			Reason: "All resources ready.",
		}
	}

	return ConvergingStatusWithReason{
		Status: maxStatus,
		Reason: strings.Join(reasons, "; "),
	}
}

// graceSummary returns an aggregate grace status of all component resources that implement the Alive interface.
// If multiple alive resources are present, the one with the most severe status takes precedence
// (Down > Degraded > Ready).
// If no resources implement the Alive interface, it returns a Down status with an explanation,
// as the component cannot provide health information.
func (c convergeResults) graceSummary() (GraceStatusWithReason, error) {
	var maxStatus GraceStatus
	var reasons []string
	anyAlive := false

	for _, result := range c {
		if alive, ok := result.Resource.(Alive); ok {
			anyAlive = true

			current, err := alive.GraceStatus()
			if err != nil {
				return GraceStatusWithReason{}, err
			}

			if current.Status.level() > maxStatus.level() {
				maxStatus = current.Status
				reasons = []string{current.Reason}
			} else if current.Status.level() == maxStatus.level() && current.Reason != "" {
				reasons = append(reasons, current.Reason)
			}
		}
	}

	if !anyAlive {
		return GraceStatusWithReason{
			Status: GraceStatusDown,
			Reason: "Component failed to converge without alive resources.",
		}, nil
	}

	if maxStatus == "" || maxStatus == GraceStatusReady {
		return GraceStatusWithReason{
			Status: GraceStatusReady,
			Reason: "All resources ready.",
		}, nil
	}

	return GraceStatusWithReason{
		Status: maxStatus,
		Reason: strings.Join(reasons, "; "),
	}, nil
}

// graceExpired returns true if the grace duration of the component has been exceeded
// since the last condition transition.
// If gracePeriod is 0, grace never expires (infinite grace).
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
//     between different progressing states.
//     - The Message field is updated in every loop to provide current details.
//     - This state is held until the component becomes Ready or the gracePeriod expires.
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

	if results.ready() {
		return conditionReady(conditionType, generation)
	}

	// No condition is set, we create the initial condition from the converge summary
	if previousCondition.Reason == string(Unknown) {
		summary := results.convergeSummary()
		return convergingCondition(conditionType, summary, generation)
	}

	reason := Status(previousCondition.Reason)

	// If we're no longer ready (results.ready() = false) but have a Ready reason on the previous condition,
	// calculate a new condition from the converge summary
	if reason == Ready {
		summary := results.convergeSummary()
		return convergingCondition(conditionType, summary, generation)
	}

	logger := log.FromContext(ctx)

	// Get the aggregate grace status of all relevant component resources
	graceSummary, err := results.graceSummary()
	if err != nil {
		logger.Error(err, "failed to get grace summary for component")
		return conditionError(conditionType, err, generation)
	}

	// If the grace period expired, and we're still progressing, set a down/degraded status
	if reason.progressing() && graceExpired(gracePeriod, previousCondition.LastTransitionTime.Time) {
		if graceSummary.Status == GraceStatusDown || graceSummary.Status == GraceStatusDegraded {
			return graceCondition(conditionType, graceSummary, generation)
		}

		// Something is misconfigured in the grace logic in the Resources
		// We continue onto other code paths but log a warning
		logger.V(0).Info(
			"component progressor encountered GraceStatus=Ready on unready component after detecting grace expiry",
		)
	}

	// If we have already transitioned to Down or Degraded due to grace expiry, we stay there unless the resource status changes.
	if (reason == Down || reason == Degraded) && (graceSummary.Status == GraceStatusDown || graceSummary.Status == GraceStatusDegraded) {
		// Only update the condition if the specific grace status (Down vs Degraded) has changed
		if string(graceSummary.Status) != string(reason) {
			return graceCondition(conditionType, graceSummary, generation)
		}
	}

	// No changes, get the summary for an updated description of why we're still here
	summary := results.convergeSummary()

	// Copy old condition and update observed generation and message
	out := previousCondition
	out.ObservedGeneration = generation
	out.Message = summary.Reason

	return out
}
