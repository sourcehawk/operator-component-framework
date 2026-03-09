package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestSuspensionStatusLevel(t *testing.T) {
	tests := []struct {
		name   string
		status SuspensionStatus
		want   int
	}{
		{"suspended", SuspensionStatusSuspended, 1},
		{"suspending", SuspensionStatusSuspending, 2},
		{"pending", SuspensionStatusPending, 3},
		{"unknown", SuspensionStatus("bogus"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.level())
		})
	}
}

func TestConvergingStatusLevel(t *testing.T) {
	tests := []struct {
		name   string
		status ConvergingStatus
		want   int
	}{
		{"ready", ConvergingStatusReady, 1},
		{"creating", ConvergingStatusCreating, 2},
		{"updating", ConvergingStatusUpdating, 3},
		{"scaling", ConvergingStatusScaling, 4},
		{"unknown", ConvergingStatus("bogus"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.level())
		})
	}
}

func TestGraceStatusLevel(t *testing.T) {
	tests := []struct {
		name   string
		status GraceStatus
		want   int
	}{
		{"ready", GraceStatusReady, 1},
		{"degraded", GraceStatusDegraded, 2},
		{"down", GraceStatusDown, 3},
		{"unknown", GraceStatus("bogus"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.level())
		})
	}
}

func TestConvergingOperationFromOperationResult(t *testing.T) {
	tests := []struct {
		name   string
		result controllerutil.OperationResult
		want   ConvergingOperation
	}{
		{"created", controllerutil.OperationResultCreated, ConvergingOperationCreated},
		{"updated", controllerutil.OperationResultUpdated, ConvergingOperationUpdated},
		{"none", controllerutil.OperationResultNone, ConvergingOperationNone},
		{"updated status", controllerutil.OperationResultUpdatedStatus, ConvergingOperationNone},
		{"updated status only", controllerutil.OperationResultUpdatedStatusOnly, ConvergingOperationNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, convergingOperationFromOperationResult(tt.result))
		})
	}
}
