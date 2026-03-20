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
