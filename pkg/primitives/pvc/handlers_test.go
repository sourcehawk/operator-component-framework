package pvc

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestDefaultOperationalStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		pvc        *corev1.PersistentVolumeClaim
		wantStatus concepts.OperationalStatus
		wantReason string
	}{
		{
			name: "bound",
			pvc: &corev1.PersistentVolumeClaim{
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "pv-001",
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "PVC is bound to volume pv-001",
		},
		{
			name: "pending",
			pvc: &corev1.PersistentVolumeClaim{
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimPending,
				},
			},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "Waiting for PVC to be bound",
		},
		{
			name: "lost",
			pvc: &corev1.PersistentVolumeClaim{
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimLost,
				},
			},
			wantStatus: concepts.OperationalStatusFailing,
			wantReason: "PVC has lost its bound volume",
		},
		{
			name:       "empty phase",
			pvc:        &corev1.PersistentVolumeClaim{},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "Waiting for PVC to be bound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultOperationalStatusHandler(concepts.ConvergingOperationCreated, tt.pvc)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	assert.False(t, DefaultDeleteOnSuspendHandler(pvc))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	mutator := NewMutator(pvc)
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	require.NoError(t, mutator.Apply())
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	got, err := DefaultSuspensionStatusHandler(pvc)
	require.NoError(t, err)
	assert.Equal(t, concepts.SuspensionStatusSuspended, got.Status)
	assert.Equal(t, "PVC does not require suspension actions", got.Reason)
}
