package component

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvergeResultsReady(t *testing.T) {
	tests := []struct {
		name     string
		results  convergeResults
		expected bool
	}{
		{
			name:     "empty results should be ready",
			results:  convergeResults{},
			expected: true,
		},
		{
			name: "all results ready",
			results: convergeResults{
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusReady}},
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusReady}},
			},
			expected: true,
		},
		{
			name: "one result not ready",
			results: convergeResults{
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusReady}},
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.results.ready())
		})
	}
}

func TestConvergeResultsConvergeSummary(t *testing.T) {
	tests := []struct {
		name     string
		results  convergeResults
		expected ConvergingStatusWithReason
	}{
		{
			name:    "empty results",
			results: convergeResults{},
			expected: ConvergingStatusWithReason{
				Status: ConvergingStatusReady,
				Reason: "All resources ready.",
			},
		},
		{
			name: "single ready",
			results: convergeResults{
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusReady, Reason: "Ready"}},
			},
			expected: ConvergingStatusWithReason{
				Status: ConvergingStatusReady,
				Reason: "All resources ready.",
			},
		},
		{
			name: "mixed statuses, highest priority wins (Scaling > Updating > Creating > Ready)",
			results: convergeResults{
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating, Reason: "Creating CM"}},
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusUpdating, Reason: "Updating Deploy"}},
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusScaling, Reason: "Scaling StatefulSet"}},
			},
			expected: ConvergingStatusWithReason{
				Status: ConvergingStatusScaling,
				Reason: "Scaling StatefulSet",
			},
		},
		{
			name: "same statuses aggregate reasons",
			results: convergeResults{
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating, Reason: "Creating CM"}},
				{Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating, Reason: "Creating Secret"}},
			},
			expected: ConvergingStatusWithReason{
				Status: ConvergingStatusCreating,
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
	t.Run("should return GraceStatusDown if no alive resources", func(t *testing.T) {
		results := convergeResults{
			{Resource: &MockResource{}},
		}
		summary, err := results.graceSummary()
		require.NoError(t, err)
		assert.Equal(t, GraceStatusDown, summary.Status)
	})

	t.Run("should aggregate grace status of alive resources", func(t *testing.T) {
		alive1 := &MockAliveResource{}
		alive1.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDegraded, Reason: "Degraded 1"}, nil)

		alive2 := &MockAliveResource{}
		alive2.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDown, Reason: "Down 2"}, nil)

		results := convergeResults{
			{Resource: alive1},
			{Resource: alive2},
		}

		summary, err := results.graceSummary()
		require.NoError(t, err)
		assert.Equal(t, GraceStatusDown, summary.Status)
		assert.Equal(t, "Down 2", summary.Reason)
	})

	t.Run("should combine reasons for same level grace status", func(t *testing.T) {
		alive1 := &MockAliveResource{}
		alive1.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDegraded, Reason: "Degraded 1"}, nil)

		alive2 := &MockAliveResource{}
		alive2.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDegraded, Reason: "Degraded 2"}, nil)

		results := convergeResults{
			{Resource: alive1},
			{Resource: alive2},
		}

		summary, err := results.graceSummary()
		require.NoError(t, err)
		assert.Equal(t, GraceStatusDegraded, summary.Status)
		assert.Equal(t, "Degraded 1; Degraded 2", summary.Reason)
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

	t.Run("should return Ready condition when all results ready", func(t *testing.T) {
		results := convergeResults{
			{Status: ConvergingStatusWithReason{Status: ConvergingStatusReady}},
		}
		previous := Condition{Type: "Test", Reason: string(Creating)}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Ready), cond.Reason)
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
			{Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating, Reason: "Creating something"}},
		}
		previous := Condition{Type: "Test", Reason: string(Unknown)}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Creating), cond.Reason)
		assert.Equal(t, "Creating something", cond.Message)
	})

	t.Run("should move to GraceStatus when grace expired and progressing", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDegraded, Reason: "Resource Degraded"}, nil)

		results := convergeResults{
			{Resource: alive, Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating}},
		}

		// 10 minutes ago, 5 minute grace period -> expired
		transition := time.Now().Add(-10 * time.Minute)
		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(Creating),
			LastTransitionTime: metav1.Time{Time: transition},
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Degraded), cond.Reason)
		assert.Contains(t, cond.Message, "Resource Degraded")
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
		alive.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDegraded, Reason: "Now Degraded"}, nil)

		results := convergeResults{
			{Resource: alive, Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating}},
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
		alive.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDown, Reason: "Status Down"}, nil)

		results := convergeResults{
			{Resource: alive, Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating, Reason: "Resource Creating"}},
		}

		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(Down),
			Message:            "Initial Down",
			LastTransitionTime: metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Down), cond.Reason)
		assert.Equal(t, "Resource Creating", cond.Message) // Message is refreshed from convergeSummary
		assert.Equal(t, previous.LastTransitionTime, cond.LastTransitionTime)
	})

	t.Run("should transition to Down from Degraded if severity increases", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDown, Reason: "Now Down"}, nil)

		results := convergeResults{
			{Resource: alive, Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating}},
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
		alive.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDegraded, Reason: "Now Degraded"}, nil)

		results := convergeResults{
			{Resource: alive, Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating}},
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
			{Status: ConvergingStatusWithReason{Status: ConvergingStatusUpdating, Reason: "Updating something"}},
		}
		previous := Condition{Type: "Test", Reason: string(Unknown)}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Updating), cond.Reason)
		assert.Equal(t, "Updating something", cond.Message)
	})

	t.Run("should transition from Ready if resources are no longer ready (Recovery from Ready)", func(t *testing.T) {
		results := convergeResults{
			{Status: ConvergingStatusWithReason{Status: ConvergingStatusScaling, Reason: "Scaling resources"}},
		}
		previous := Condition{
			Type:   "Test",
			Status: metav1.ConditionTrue,
			Reason: string(Ready),
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(Scaling), cond.Reason)
		assert.Equal(t, "Scaling resources", cond.Message)
	})

	t.Run("should update ObservedGeneration and Message if no state transition occurs (Steady State Update)", func(t *testing.T) {
		results := convergeResults{
			{Status: ConvergingStatusWithReason{Status: ConvergingStatusUpdating, Reason: "New progress message"}},
		}
		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(Updating),
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
		assert.Equal(t, string(Updating), cond.Reason)
		assert.Equal(t, "New progress message", cond.Message)
		assert.Equal(t, previous.Status, cond.Status)
	})

	t.Run("should update ObservedGeneration if no other changes", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusReady, Reason: "Resource is ready but unready"}, nil)

		results := convergeResults{
			{Resource: alive, Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating, Reason: "Still Creating"}},
		}

		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(Creating),
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
			{Status: ConvergingStatusWithReason{Status: ConvergingStatusUpdating, Reason: "Still creating something"}},
		}

		previous := Condition{
			Type:               "Test",
			Status:             metav1.ConditionFalse,
			Reason:             string(Creating),
			Message:            "Creating something",
			ObservedGeneration: 0,
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 0, previous)

		assert.Equal(t, string(Creating), cond.Reason)
		assert.Equal(t, "Still creating something", cond.Message)

		// Also check Scaling
		results = convergeResults{
			{Status: ConvergingStatusWithReason{Status: ConvergingStatusScaling, Reason: "Taking forever to create"}},
		}
		cond = newConvergingStatusCondition(ctx, owner, results, 0, previous)
		assert.Equal(t, string(Creating), cond.Reason)
		assert.Equal(t, "Taking forever to create", cond.Message)
	})

	t.Run("should stay Degraded or Down even if results contain creating/updating/scaling", func(t *testing.T) {
		alive := &MockAliveResource{}
		alive.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDown, Reason: "Still Down"}, nil)

		results := convergeResults{
			{Resource: alive, Status: ConvergingStatusWithReason{Status: ConvergingStatusCreating, Reason: "Resource Creating"}},
		}

		previous := Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Down),
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, string(Down), cond.Reason)
		assert.Contains(t, cond.Message, "Resource Creating")

		// Same for Degraded
		alive = &MockAliveResource{}
		alive.On("GraceStatus").Return(GraceStatusWithReason{Status: GraceStatusDegraded, Reason: "Still Degraded"}, nil)

		results = convergeResults{
			{Resource: alive, Status: ConvergingStatusWithReason{Status: ConvergingStatusUpdating, Reason: "Resource Updating"}},
		}

		previous = Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Degraded),
		}

		cond = newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)
		assert.Equal(t, string(Degraded), cond.Reason)
		assert.Contains(t, cond.Message, "Resource Updating")
	})

	t.Run("should transition to Ready from Down or Degraded if all results are ready", func(t *testing.T) {
		results := convergeResults{
			{Status: ConvergingStatusWithReason{Status: ConvergingStatusReady}},
		}

		// Transition from Down
		previous := Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Down),
		}

		cond := newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Ready), cond.Reason)

		// Transition from Degraded
		previous = Condition{
			Type:   "Test",
			Status: metav1.ConditionFalse,
			Reason: string(Degraded),
		}

		cond = newConvergingStatusCondition(ctx, owner, results, 5*time.Minute, previous)

		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(Ready), cond.Reason)
	})
}
