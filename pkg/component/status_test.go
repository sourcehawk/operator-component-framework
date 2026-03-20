package component

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
)

func TestStatusLevelOrdering(t *testing.T) {
	// Grace ordering
	assert.Greater(t, Down.Priority(), Degraded.Priority())

	// Suspension ordering
	assert.Greater(t, PendingSuspension.Priority(), Suspending.Priority())
	assert.Greater(t, Suspending.Priority(), Suspended.Priority())

	// Alive ordering
	assert.Greater(t, AliveFailing.Priority(), AliveScaling.Priority())
	assert.Greater(t, AliveScaling.Priority(), AliveUpdating.Priority())
	assert.Greater(t, AliveUpdating.Priority(), AliveCreating.Priority())
	assert.Greater(t, AliveCreating.Priority(), Healthy.Priority())

	// Completion ordering
	assert.Greater(t, CompletionFailing.Priority(), CompletionRunning.Priority())
	assert.Greater(t, CompletionRunning.Priority(), CompletionPending.Priority())
	assert.Greater(t, CompletionPending.Priority(), Completed.Priority())

	// Operational ordering
	assert.Greater(t, OperationFailing.Priority(), OperationPending.Priority())
	assert.Greater(t, OperationPending.Priority(), Operational.Priority())

	// Healthy state: Alive > Operational > Completable
	assert.Greater(t, Healthy.Priority(), Operational.Priority())
	assert.Greater(t, Operational.Priority(), Completed.Priority())

	// Failing state: Alive > Operational > Completable
	assert.Greater(t, AliveFailing.Priority(), Operational.Priority())
	assert.Greater(t, OperationFailing.Priority(), CompletionFailing.Priority())

	// Progression states: Alive > Operational > Completable
	assert.Greater(t, AliveCreating.Priority(), OperationPending.Priority())
	assert.Greater(t, OperationPending.Priority(), CompletionPending.Priority())
	assert.Greater(t, AliveScaling.Priority(), CompletionRunning.Priority())

	// Suspension > Failure (assumes the failing state assertions above)
	assert.Greater(t, Suspended.Priority(), AliveFailing.Priority())
	assert.Greater(t, Error.Priority(), Down.Priority())

	// Grace > Suspension
	assert.Greater(t, Degraded.Priority(), Suspended.Priority())

	// Error > Grace
}

func TestStatusConstantsMatchSourceTypes(t *testing.T) {
	// Regression tests in case someone decides to mess with the status values
	assert.Equal(t, Healthy, Status(concepts.AliveConvergingStatusHealthy))
	assert.Equal(t, AliveCreating, Status(concepts.AliveConvergingStatusCreating))
	assert.Equal(t, AliveUpdating, Status(concepts.AliveConvergingStatusUpdating))
	assert.Equal(t, AliveScaling, Status(concepts.AliveConvergingStatusScaling))

	assert.Equal(t, Completed, Status(concepts.CompletionStatusCompleted))
	assert.Equal(t, CompletionPending, Status(concepts.CompletionStatusPending))
	assert.Equal(t, CompletionRunning, Status(concepts.CompletionStatusRunning))
	assert.Equal(t, CompletionFailing, Status(concepts.CompletionStatusFailing))

	assert.Equal(t, Operational, Status(concepts.OperationalStatusOperational))
	assert.Equal(t, OperationPending, Status(concepts.OperationalStatusPending))
	assert.Equal(t, OperationFailing, Status(concepts.OperationalStatusFailing))

	assert.Equal(t, PendingSuspension, Status(concepts.SuspensionStatusPending))
	assert.Equal(t, Suspending, Status(concepts.SuspensionStatusSuspending))
	assert.Equal(t, Suspended, Status(concepts.SuspensionStatusSuspended))

	assert.Equal(t, Degraded, Status(concepts.GraceStatusDegraded))
	assert.Equal(t, Down, Status(concepts.GraceStatusDown))
}

func TestConvergingStatusSeverity(t *testing.T) {
	tests := []struct {
		status   convergingStatus
		expected int
	}{
		{convergingStatusAliveHealthy, 1},
		{convergingStatusOperationalOperational, 1},
		{convergingStatusCompletableCompleted, 1},
		{convergingStatusAliveCreating, 2},
		{convergingStatusOperationalPending, 2},
		{convergingStatusCompletablePending, 2},
		{convergingStatusAliveUpdating, 3},
		{convergingStatusAliveScaling, 4},
		{convergingStatusCompletableRunning, 4},
		{convergingStatusAliveFailing, 5},
		{convergingStatusOperationalFailing, 5},
		{convergingStatusCompletableFailed, 5},
		{convergingStatus("unknown"), 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.severity())
		})
	}
}

func TestConvergingStatusPriority(t *testing.T) {
	tests := []struct {
		status   convergingStatus
		expected int
	}{
		{convergingStatusCompletableCompleted, 1},
		{convergingStatusOperationalOperational, 2},
		{convergingStatusAliveHealthy, 3},
		{convergingStatusCompletablePending, 4},
		{convergingStatusOperationalPending, 5},
		{convergingStatusAliveCreating, 6},
		{convergingStatusAliveUpdating, 7},
		{convergingStatusCompletableRunning, 8},
		{convergingStatusAliveScaling, 9},
		{convergingStatusCompletableFailed, 10},
		{convergingStatusOperationalFailing, 11},
		{convergingStatusAliveFailing, 12},
		{convergingStatus("unknown"), 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.priority())
		})
	}
}

func TestConvergingStatusHealthy(t *testing.T) {
	tests := []struct {
		status   convergingStatus
		expected bool
	}{
		{convergingStatusAliveHealthy, true},
		{convergingStatusOperationalOperational, true},
		{convergingStatusCompletableCompleted, true},
		{convergingStatusAliveCreating, false},
		{convergingStatusAliveUpdating, false},
		{convergingStatusAliveScaling, false},
		{convergingStatusAliveFailing, false},
		{convergingStatusOperationalPending, false},
		{convergingStatusOperationalFailing, false},
		{convergingStatusCompletablePending, false},
		{convergingStatusCompletableRunning, false},
		{convergingStatusCompletableFailed, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.healthy())
		})
	}
}
