package component_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
)

const gracePeriod = 5 * time.Minute

// graceComponent builds a component named name with the given grace period.
func graceComponent(t *testing.T, name string, grace time.Duration, suspended bool) *component.Component {
	t.Helper()

	comp, err := component.NewComponentBuilder().
		WithName(name).
		WithConditionType(component.ConditionType(name + "Ready")).
		WithGracePeriod(grace).
		Suspend(suspended).
		Build()
	require.NoError(t, err)

	return comp
}

// stageCondition writes the condition of comp onto owner with the given reason
// and transition time. The status is True for the reasons that the component
// reports as True, and False for all other reasons.
func stageCondition(owner *component.MockOperatorCRD, comp *component.Component, reason component.Status, transition time.Time) {
	status := metav1.ConditionFalse
	if reason.Healthy() || reason == component.Disabled || reason == component.Suspended ||
		reason == component.Suspending || reason == component.PendingSuspension {
		status = metav1.ConditionTrue
	}

	conditions := owner.GetStatusConditions()
	*conditions = append(*conditions, metav1.Condition{
		Type:               comp.GetName() + "Ready",
		Status:             status,
		Reason:             string(reason),
		LastTransitionTime: metav1.NewTime(transition),
	})
}

func TestGraceRemainingReportsTimeUntilGraceEnds(t *testing.T) {
	comp := graceComponent(t, "backend", gracePeriod, false)
	owner := &component.MockOperatorCRD{}
	stageCondition(owner, comp, component.AliveCreating, time.Now().Add(-2*time.Minute))

	remaining, ok := comp.GraceRemaining(owner)

	require.True(t, ok)
	assert.Greater(t, remaining, 3*time.Minute-time.Second)
	assert.LessOrEqual(t, remaining, 3*time.Minute+time.Second)
}

func TestGraceRemainingIsPendingForEveryReasonTheGracePeriodGrades(t *testing.T) {
	reasons := []component.Status{
		component.AliveCreating,
		component.AliveUpdating,
		component.AliveScaling,
		component.AliveFailing,
		component.OperationPending,
		component.OperationFailing,
		component.CompletionPending,
		component.CompletionRunning,
		component.CompletionFailing,
		component.GuardBlocked,
	}

	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			comp := graceComponent(t, "backend", gracePeriod, false)
			owner := &component.MockOperatorCRD{}
			stageCondition(owner, comp, reason, time.Now())

			remaining, ok := comp.GraceRemaining(owner)

			require.True(t, ok)
			assert.Positive(t, remaining)
		})
	}
}

func TestGraceRemainingReportsNoExpiryForReasonsTheGracePeriodDoesNotGrade(t *testing.T) {
	reasons := []component.Status{
		component.Healthy,
		component.Operational,
		component.Completed,
		component.Degraded,
		component.Down,
		component.PendingSuspension,
		component.Suspending,
		component.Suspended,
		component.Disabled,
		component.PrerequisiteNotMet,
		component.FeatureGateError,
		component.Unknown,
	}

	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			comp := graceComponent(t, "backend", gracePeriod, false)
			owner := &component.MockOperatorCRD{}
			stageCondition(owner, comp, reason, time.Now())

			_, ok := comp.GraceRemaining(owner)

			assert.False(t, ok)
		})
	}
}

func TestGraceRemainingReportsNoExpiryBeforeTheFirstReconcile(t *testing.T) {
	comp := graceComponent(t, "backend", gracePeriod, false)
	owner := &component.MockOperatorCRD{}

	_, ok := comp.GraceRemaining(owner)

	assert.False(t, ok)
}

func TestGraceRemainingReportsNoExpiryWithoutAGracePeriod(t *testing.T) {
	comp := graceComponent(t, "backend", 0, false)
	owner := &component.MockOperatorCRD{}
	stageCondition(owner, comp, component.AliveCreating, time.Now())

	_, ok := comp.GraceRemaining(owner)

	assert.False(t, ok)
}

func TestGraceRemainingReportsNoExpiryForASuspendedComponent(t *testing.T) {
	// The condition still holds the converging reason of the reconcile before
	// the component was suspended.
	comp := graceComponent(t, "backend", gracePeriod, true)
	owner := &component.MockOperatorCRD{}
	stageCondition(owner, comp, component.AliveCreating, time.Now())

	_, ok := comp.GraceRemaining(owner)

	assert.False(t, ok)
}

func TestGraceRemainingReportsNoExpiryOnceTheGracePeriodHasEnded(t *testing.T) {
	// A reconcile after the end of the grace period already graded the
	// condition, so no later expiry is left to wait for.
	comp := graceComponent(t, "backend", gracePeriod, false)
	owner := &component.MockOperatorCRD{}
	stageCondition(owner, comp, component.AliveCreating, time.Now().Add(-2*gracePeriod))

	_, ok := comp.GraceRemaining(owner)

	assert.False(t, ok)
}

func TestGraceRemainingEndsAfterTheGracePeriodOfTheTruncatedTransitionTime(t *testing.T) {
	// The API server stores LastTransitionTime in whole seconds, so the next
	// reconcile can read a transition time up to one second earlier than the
	// one in memory. A requeue at the reported time must be strictly after
	// the end of the grace period for both values.
	tests := map[string]time.Duration{
		"fractional transition time": 500 * time.Millisecond,
		"whole-second transition":    0,
	}

	for name, fraction := range tests {
		t.Run(name, func(t *testing.T) {
			transition := time.Now().Add(-time.Minute).Truncate(time.Second).Add(fraction)
			comp := graceComponent(t, "backend", gracePeriod, false)
			owner := &component.MockOperatorCRD{}
			stageCondition(owner, comp, component.AliveCreating, transition)

			before := time.Now()
			remaining, ok := comp.GraceRemaining(owner)

			require.True(t, ok)
			requeueAt := before.Add(remaining)
			assert.True(t, requeueAt.After(transition.Add(gracePeriod)),
				"requeue at %v is not after the end of the grace period at %v", requeueAt, transition.Add(gracePeriod))
			assert.True(t, requeueAt.After(transition.Truncate(time.Second).Add(gracePeriod)))
			assert.LessOrEqual(t, requeueAt.Sub(transition.Add(gracePeriod)), time.Second)
		})
	}
}

func TestEarliestGraceRemainingReportsTheSoonestExpiry(t *testing.T) {
	owner := &component.MockOperatorCRD{}
	later := graceComponent(t, "later", gracePeriod, false)
	sooner := graceComponent(t, "sooner", gracePeriod, false)
	ready := graceComponent(t, "ready", gracePeriod, false)
	stageCondition(owner, later, component.AliveCreating, time.Now().Add(-time.Minute))
	stageCondition(owner, sooner, component.AliveUpdating, time.Now().Add(-4*time.Minute))
	stageCondition(owner, ready, component.Healthy, time.Now())

	remaining, ok := component.EarliestGraceRemaining(owner, later, sooner, ready)

	require.True(t, ok)
	soonest, _ := sooner.GraceRemaining(owner)
	assert.InDelta(t, soonest, remaining, float64(time.Second))
	assert.Less(t, remaining, 2*time.Minute)
}

func TestEarliestGraceRemainingReportsNoExpiryWhenNoComponentHasOne(t *testing.T) {
	owner := &component.MockOperatorCRD{}
	ready := graceComponent(t, "ready", gracePeriod, false)
	down := graceComponent(t, "down", gracePeriod, false)
	stageCondition(owner, ready, component.Healthy, time.Now())
	stageCondition(owner, down, component.Down, time.Now())

	_, ok := component.EarliestGraceRemaining(owner, ready, down)

	assert.False(t, ok)
}

func TestEarliestGraceRemainingReportsNoExpiryWithoutComponents(t *testing.T) {
	_, ok := component.EarliestGraceRemaining(&component.MockOperatorCRD{})

	assert.False(t, ok)
}
