package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusLevel(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   int
	}{
		{"unknown", Unknown, 0},
		{"ready", Ready, 1},
		{"creating", Creating, 2},
		{"updating", Updating, 3},
		{"scaling", Scaling, 4},
		{"suspended", Suspended, 5},
		{"suspending", Suspending, 6},
		{"pending suspension", PendingSuspension, 7},
		{"degraded", Degraded, 8},
		{"down", Down, 9},
		{"error", Error, 10},
		{"invalid", Status("bogus"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.Level())
		})
	}
}

func TestStatusProgressing(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"creating", Creating, true},
		{"updating", Updating, true},
		{"scaling", Scaling, true},
		{"unknown", Unknown, false},
		{"ready", Ready, false},
		{"pending suspension", PendingSuspension, false},
		{"suspending", Suspending, false},
		{"suspended", Suspended, false},
		{"degraded", Degraded, false},
		{"down", Down, false},
		{"error", Error, false},
		{"invalid", Status("bogus"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.progressing())
		})
	}
}

func TestStatusLevelOrdering(t *testing.T) {
	// Regression tests in case someone decides to mess with the code
	// The converge logic relies on this ordering
	assert.Greater(t, Error.Level(), Down.Level())
	assert.Greater(t, Down.Level(), Degraded.Level())
	assert.Greater(t, Degraded.Level(), PendingSuspension.Level())
	assert.Greater(t, PendingSuspension.Level(), Suspending.Level())
	assert.Greater(t, Suspending.Level(), Suspended.Level())
	assert.Greater(t, Scaling.Level(), Updating.Level())
	assert.Greater(t, Updating.Level(), Creating.Level())
	assert.Greater(t, Creating.Level(), Ready.Level())
}

func TestStatusConstantsMatchSourceTypes(t *testing.T) {
	// Regression tests in case someone decides to mess with the status types
	assert.Equal(t, Ready, Status(ConvergingStatusReady))
	assert.Equal(t, Creating, Status(ConvergingStatusCreating))
	assert.Equal(t, Updating, Status(ConvergingStatusUpdating))
	assert.Equal(t, Scaling, Status(ConvergingStatusScaling))

	assert.Equal(t, PendingSuspension, Status(SuspensionStatusPending))
	assert.Equal(t, Suspending, Status(SuspensionStatusSuspending))
	assert.Equal(t, Suspended, Status(SuspensionStatusSuspended))

	assert.Equal(t, Degraded, Status(GraceStatusDegraded))
	assert.Equal(t, Down, Status(GraceStatusDown))
}
