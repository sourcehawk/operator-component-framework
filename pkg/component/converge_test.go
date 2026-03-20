package component

import (
	"testing"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvergeResultsHealthy(t *testing.T) {
	tests := []struct {
		name     string
		results  convergeResults
		expected bool
	}{
		{
			name:     "empty results should be healthy",
			results:  convergeResults{},
			expected: true,
		},
		{
			name: "all results healthy",
			results: convergeResults{
				{Status: convergingStatusWithReason{Status: convergingStatusAliveHealthy}},
				{Status: convergingStatusWithReason{Status: convergingStatusAliveHealthy}},
			},
			expected: true,
		},
		{
			name: "one result not healthy",
			results: convergeResults{
				{Status: convergingStatusWithReason{Status: convergingStatusAliveHealthy}},
				{Status: convergingStatusWithReason{Status: convergingStatusAliveCreating}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.results.healthy())
		})
	}
}

func TestConvergeResultsConvergeSummary(t *testing.T) {
	tests := []struct {
		name     string
		results  convergeResults
		expected convergingStatusWithReason
	}{
		{
			name:    "empty results",
			results: convergeResults{},
			expected: convergingStatusWithReason{
				Status: convergingStatusAliveHealthy,
				Reason: "All resources healthy.",
			},
		},
		{
			name: "single healthy",
			results: convergeResults{
				{Status: convergingStatusWithReason{Status: convergingStatusAliveHealthy, Reason: "Healthy"}},
			},
			expected: convergingStatusWithReason{
				Status: convergingStatusAliveHealthy,
				Reason: "All resources healthy.",
			},
		},
		{
			name: "mixed statuses, highest priority wins (Scaling > Updating > Creating > Healthy)",
			results: convergeResults{
				{Status: convergingStatusWithReason{Status: convergingStatusAliveCreating, Reason: "Creating CM"}},
				{Status: convergingStatusWithReason{Status: convergingStatusAliveUpdating, Reason: "Updating Deploy"}},
				{Status: convergingStatusWithReason{Status: convergingStatusAliveScaling, Reason: "Scaling StatefulSet"}},
			},
			expected: convergingStatusWithReason{
				Status: convergingStatusAliveScaling,
				Reason: "Scaling StatefulSet",
			},
		},
		{
			name: "mixed resource concepts, highest priority wins",
			results: convergeResults{
				{Status: convergingStatusWithReason{Status: convergingStatusAliveHealthy, Reason: "Alive Healthy"}},
				{Status: convergingStatusWithReason{Status: convergingStatusOperationalPending, Reason: "Operational Pending"}},
				{Status: convergingStatusWithReason{Status: convergingStatusCompletableRunning, Reason: "Completable Running"}},
			},
			expected: convergingStatusWithReason{
				Status: convergingStatusCompletableRunning,
				Reason: "Completable Running",
			},
		},
		{
			name: "failing statuses priority",
			results: convergeResults{
				{Status: convergingStatusWithReason{Status: convergingStatusAliveFailing, Reason: "Alive Failing"}},
				{Status: convergingStatusWithReason{Status: convergingStatusOperationalFailing, Reason: "Operational Failing"}},
				{Status: convergingStatusWithReason{Status: convergingStatusCompletableFailed, Reason: "Completable Failed"}},
			},
			expected: convergingStatusWithReason{
				Status: convergingStatusAliveFailing,
				Reason: "Alive Failing; Operational Failing; Completable Failed",
			},
		},
		{
			name: "same severity aggregate reasons (Creating, OperationPending, CompletionPending)",
			results: convergeResults{
				{Status: convergingStatusWithReason{Status: convergingStatusAliveCreating, Reason: "Creating CM"}},
				{Status: convergingStatusWithReason{Status: convergingStatusOperationalPending, Reason: "Awaiting LB"}},
				{Status: convergingStatusWithReason{Status: convergingStatusCompletablePending, Reason: "Job pending"}},
			},
			expected: convergingStatusWithReason{
				Status: convergingStatusAliveCreating,
				Reason: "Creating CM; Awaiting LB; Job pending",
			},
		},
		{
			name: "same statuses aggregate reasons",
			results: convergeResults{
				{Status: convergingStatusWithReason{Status: convergingStatusAliveCreating, Reason: "Creating CM"}},
				{Status: convergingStatusWithReason{Status: convergingStatusAliveCreating, Reason: "Creating Secret"}},
			},
			expected: convergingStatusWithReason{
				Status: convergingStatusAliveCreating,
				Reason: "Creating CM; Creating Secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.results.convergeSummary())
		})
	}
}

func TestConvergeResultsGraceSummary(t *testing.T) {
	t.Run("should return concepts.GraceStatusDown if no alive resources", func(t *testing.T) {
		results := convergeResults{
			{Resource: &MockResource{}},
		}
		summary, err := results.graceSummary()
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, summary.Status)
	})

	t.Run("should aggregate grace status of alive resources", func(t *testing.T) {
		alive1 := &MockAliveResource{}
		alive1.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded, Reason: "Degraded 1"}, nil)

		alive2 := &MockAliveResource{}
		alive2.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDown, Reason: "Down 2"}, nil)

		results := convergeResults{
			{Resource: alive1},
			{Resource: alive2},
		}

		summary, err := results.graceSummary()
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, summary.Status)
		assert.Equal(t, "Down 2", summary.Reason)
	})

	t.Run("should combine reasons for same level grace status", func(t *testing.T) {
		alive1 := &MockAliveResource{}
		alive1.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded, Reason: "Degraded 1"}, nil)

		alive2 := &MockAliveResource{}
		alive2.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded, Reason: "Degraded 2"}, nil)

		results := convergeResults{
			{Resource: alive1},
			{Resource: alive2},
		}

		summary, err := results.graceSummary()
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, summary.Status)
		assert.Equal(t, "Degraded 1; Degraded 2", summary.Reason)
	})

	t.Run("should aggregate grace status of mixed resource types", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded, Reason: "Alive Degraded"}, nil)

		completable := &MockCompletableResource{}
		completable.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDown, Reason: "Completable Down"}, nil)

		operational := &MockOperationalResource{}
		operational.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy, Reason: "Operational Healthy"}, nil)

		results := convergeResults{
			{Resource: alive},
			{Resource: completable},
			{Resource: operational},
		}

		summary, err := results.graceSummary()
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, summary.Status)
		assert.Equal(t, "Completable Down", summary.Reason)
	})
}

func TestGraceExpired(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		gracePeriod time.Duration
		transition  time.Time
		expected    bool
	}{
		{
			name:        "zero grace period",
			gracePeriod: 0,
			transition:  now.Add(-10 * time.Minute),
			expected:    false,
		},
		{
			name:        "not expired",
			gracePeriod: 5 * time.Minute,
			transition:  now.Add(-2 * time.Minute),
			expected:    false,
		},
		{
			name:        "expired",
			gracePeriod: 5 * time.Minute,
			transition:  now.Add(-10 * time.Minute),
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, graceExpired(tt.gracePeriod, tt.transition))
		})
	}
}

func TestNewConvergingStatusCondition(t *testing.T) {
	var (
		ownerGeneration = int64(1)
		owner           = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-owner",
				Namespace:  "test-ns",
				Generation: ownerGeneration,
			},
		}
		ctx = t.Context()
	)

	t.Run("should return Healthy condition when all results healthy", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusAliveHealthy}},
		}
		previous := Condition{Type: "Test", Reason: string(AliveCreating)}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Healthy), cond.Reason)
	})
}

func TestNewConvergingStatusCondition_Initialization(t *testing.T) {
	var (
		ownerGeneration = int64(1)
		owner           = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-owner",
				Namespace:  "test-ns",
				Generation: ownerGeneration,
			},
		}
		ctx = t.Context()
	)

	t.Run("should initialize condition from summary if previous is Unknown", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusAliveCreating, Reason: "Creating something"}},
		}
		previous := Condition{Type: "Test", Reason: string(Unknown)}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(AliveCreating), cond.Reason)
		assert.Equal(t, "Creating something", cond.Message)
	})

	t.Run("should initialize condition from OperationalPending if previous is Unknown", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusOperationalPending, Reason: "Awaiting LB IP"}},
		}
		previous := Condition{Type: "Test", Reason: string(Unknown)}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(OperationPending), cond.Reason)
		assert.Equal(t, "Awaiting LB IP", cond.Message)
	})

	t.Run("should initialize condition from TaskRunning if previous is Unknown", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusCompletableRunning, Reason: "Task is running"}},
		}
		previous := Condition{Type: "Test", Reason: string(Unknown)}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(CompletionRunning), cond.Reason)
		assert.Equal(t, "Task is running", cond.Message)
	})

	t.Run("should move to GraceStatus when grace expired and Progressing", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded, Reason: "Resource Degraded"}, nil)

		results := convergeResults{
			{Resource: alive, Status: convergingStatusWithReason{Status: convergingStatusAliveCreating}},
		}

		// 10 minutes ago, 5 minute grace period -> expired
		transition := time.Now().Add(-10 * time.Minute)
		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(AliveCreating),
			LastTransitionTime: metav1.Time{Time: transition},
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Degraded), cond.Reason)
		assert.Contains(t, cond.Message, "Resource Degraded")
	})

	t.Run("should move to GraceStatus when grace expired and OperationalPending", func(t *testing.T) {
		operational := &MockOperationalResource{}
		operational.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDown, Reason: "Operational Down"}, nil)

		results := convergeResults{
			{Resource: operational, Status: convergingStatusWithReason{Status: convergingStatusOperationalPending}},
		}

		transition := time.Now().Add(-10 * time.Minute)
		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(OperationPending),
			LastTransitionTime: metav1.Time{Time: transition},
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Down), cond.Reason)
		assert.Contains(t, cond.Message, "Operational Down")
	})
}

func TestNewConvergingStatusCondition_GracePeriod(t *testing.T) {
	var (
		ownerGeneration = int64(1)
		owner           = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-owner",
				Namespace:  "test-ns",
				Generation: ownerGeneration,
			},
		}
		ctx = t.Context()
	)

	t.Run("should transition from Down to Degraded after grace period", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded, Reason: "Now Degraded"}, nil)

		results := convergeResults{
			{Resource: alive, Status: convergingStatusWithReason{Status: convergingStatusAliveCreating}},
		}

		previous := Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Down),
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Degraded), cond.Reason)
		assert.Contains(t, cond.Message, "Now Degraded")
	})

	t.Run("should stay Down if it was already Down and grace status is still Down", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDown, Reason: "Status Down"}, nil)

		results := convergeResults{
			{Resource: alive, Status: convergingStatusWithReason{Status: convergingStatusAliveCreating, Reason: "Resource Creating"}},
		}

		// Use a fixed time for LastTransitionTime to avoid flaky tests due to time.Now()
		transitionTime := time.Date(2026, 3, 20, 17, 3, 0, 0, time.UTC)
		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(Down),
			Message:            "Initial Down",
			LastTransitionTime: metav1.Time{Time: transitionTime},
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Down), cond.Reason)
		// It currently uses the message from graceStatus summary
		assert.Contains(t, cond.Message, "Status Down")
	})

	t.Run("should transition to Down from Degraded if severity increases", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDown, Reason: "Now Down"}, nil)

		results := convergeResults{
			{Resource: alive, Status: convergingStatusWithReason{Status: convergingStatusAliveCreating}},
		}

		previous := Condition{
			Status:  metav1.ConditionFalse,
			Reason:  string(Degraded),
			Message: "Initial Degraded",
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Down), cond.Reason)
		assert.Contains(t, cond.Message, "Now Down")
	})

	t.Run("should transition to Degraded from Down if severity decreases", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded, Reason: "Now Degraded"}, nil)

		results := convergeResults{
			{Resource: alive, Status: convergingStatusWithReason{Status: convergingStatusAliveCreating}},
		}

		previous := Condition{
			Status:  metav1.ConditionFalse,
			Reason:  string(Down),
			Message: "Initial Down",
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Degraded), cond.Reason)
		assert.Contains(t, cond.Message, "Now Degraded")
	})
}

func TestNewConvergingStatusCondition_Transitions(t *testing.T) {
	var (
		ownerGeneration = int64(1)
		owner           = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-owner",
				Namespace:  "test-ns",
				Generation: ownerGeneration,
			},
		}
		ctx = t.Context()
	)

	t.Run("should initialize condition from summary if previous is Unknown (Initialization)", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusAliveUpdating, Reason: "Updating something"}},
		}
		previous := Condition{Type: "Test", Reason: string(Unknown)}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(AliveUpdating), cond.Reason)
		assert.Equal(t, "Updating something", cond.Message)
	})

	t.Run("should transition from Healthy if resources are no longer healthy (Recovery from Healthy)", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusAliveScaling, Reason: "Scaling resources"}},
		}
		previous := Condition{
			Type:   "Test",
			Status: metav1.ConditionTrue,
			Reason: string(Healthy),
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(AliveScaling), cond.Reason)
		assert.Equal(t, "Scaling resources", cond.Message)
	})

	t.Run("should update ObservedGeneration and Message if no state transition occurs (Steady State Update)", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusAliveUpdating, Reason: "New progress message"}},
		}
		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(AliveUpdating),
			Message:            "Old progress message",
			ObservedGeneration: 10,
		}

		ownerWithNewGen := &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 11,
			},
		}

		cond := newConvergingStatusCondition(ctx, ownerWithNewGen, results, 0, previous)

		assert.Equal(t, int64(11), cond.ObservedGeneration)
		assert.Equal(t, string(AliveUpdating), cond.Reason)
		assert.Equal(t, "New progress message", cond.Message)
		assert.Equal(t, previous.Status, cond.Status)
	})

	t.Run("should update ObservedGeneration if no other changes", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusHealthy, Reason: "Resource is healthy but unready"}, nil)

		results := convergeResults{
			{Resource: alive, Status: convergingStatusWithReason{Status: convergingStatusAliveCreating, Reason: "Still Creating"}},
		}

		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(AliveCreating),
			Message:            "Still Creating",
			ObservedGeneration: 0,
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, ownerGeneration, cond.ObservedGeneration)
		assert.Equal(t, previous.Reason, cond.Reason)
		assert.Equal(t, previous.Message, cond.Message)
	})

	t.Run("should not update condition reason from Creating to Updating or Scaling", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusAliveUpdating, Reason: "Still creating something"}},
		}

		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(AliveCreating),
			Message:            "Creating something",
			ObservedGeneration: 0,
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, string(AliveCreating), cond.Reason)
		assert.Equal(t, "Still creating something", cond.Message)

		// Also check Scaling
		results = convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusAliveScaling, Reason: "Taking forever to create"}},
		}
		cond = newConvergingStatusCondition(ctx, owner, results, 0, previous)
		assert.Equal(t, string(AliveCreating), cond.Reason)
		assert.Equal(t, "Taking forever to create", cond.Message)
	})

	t.Run("should stay Degraded or Down even if results contain creating/updating/scaling", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDown, Reason: "Still Down"}, nil)

		results := convergeResults{
			{Resource: alive, Status: convergingStatusWithReason{Status: convergingStatusAliveCreating, Reason: "Resource Creating"}},
		}

		previous := Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Down),
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Down), cond.Reason)
		assert.Contains(t, cond.Message, "Still Down")

		// Same for Degraded
		alive = &MockAliveResource{}
		alive.On("GraceStatus").Return(concepts.GraceStatusWithReason{Status: concepts.GraceStatusDegraded, Reason: "Still Degraded"}, nil)

		results = convergeResults{
			{Resource: alive, Status: convergingStatusWithReason{Status: convergingStatusAliveUpdating, Reason: "Resource Updating"}},
		}

		previous = Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Degraded),
		}

		cond = newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)
		assert.Equal(t, string(Degraded), cond.Reason)
		assert.Contains(t, cond.Message, "Still Degraded")
	})

	t.Run("should transition to Healthy from Down or Degraded if all results are healthy", func(t *testing.T) {
		results := convergeResults{
			{Status: convergingStatusWithReason{Status: convergingStatusAliveHealthy}},
		}

		// Transition from Down
		previous := Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Down),
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Healthy), cond.Reason)

		// Transition from Degraded
		previous = Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Degraded),
		}

		cond = newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Healthy), cond.Reason)
	})
}
