package concepts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

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
			assert.Equal(t, tt.want, ConvergingOperationFromOperationResult(tt.result))
		})
	}
}

func TestStaleGenerationStatus(t *testing.T) {
	tests := []struct {
		name               string
		op                 ConvergingOperation
		observedGeneration int64
		generation         int64
		wantNil            bool
		wantStatus         AliveConvergingStatus
	}{
		{
			name:               "current generation returns nil",
			op:                 ConvergingOperationUpdated,
			observedGeneration: 2,
			generation:         2,
			wantNil:            true,
		},
		{
			name:               "ahead generation returns nil",
			op:                 ConvergingOperationUpdated,
			observedGeneration: 3,
			generation:         2,
			wantNil:            true,
		},
		{
			name:               "zero values return nil",
			op:                 ConvergingOperationNone,
			observedGeneration: 0,
			generation:         0,
			wantNil:            true,
		},
		{
			name:               "stale after create returns creating",
			op:                 ConvergingOperationCreated,
			observedGeneration: 0,
			generation:         1,
			wantStatus:         AliveConvergingStatusCreating,
		},
		{
			name:               "stale after update returns updating",
			op:                 ConvergingOperationUpdated,
			observedGeneration: 1,
			generation:         2,
			wantStatus:         AliveConvergingStatusUpdating,
		},
		{
			name:               "stale with no operation returns updating",
			op:                 ConvergingOperationNone,
			observedGeneration: 1,
			generation:         2,
			wantStatus:         AliveConvergingStatusUpdating,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StaleGenerationStatus(tt.op, tt.observedGeneration, tt.generation, "test")
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantStatus, result.Status)
			}
		})
	}
}
