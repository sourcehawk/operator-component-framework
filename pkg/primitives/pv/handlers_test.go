package pv

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
		phase      corev1.PersistentVolumePhase
		wantStatus concepts.OperationalStatus
		wantReason string
	}{
		{
			name:       "available",
			phase:      corev1.VolumeAvailable,
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "PersistentVolume is available",
		},
		{
			name:       "bound",
			phase:      corev1.VolumeBound,
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "PersistentVolume is bound to a claim",
		},
		{
			name:       "pending",
			phase:      corev1.VolumePending,
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "PersistentVolume is pending",
		},
		{
			name:       "released",
			phase:      corev1.VolumeReleased,
			wantStatus: concepts.OperationalStatusFailing,
			wantReason: "PersistentVolume has been released and is not yet reclaimed",
		},
		{
			name:       "failed",
			phase:      corev1.VolumeFailed,
			wantStatus: concepts.OperationalStatusFailing,
			wantReason: "PersistentVolume reclamation has failed",
		},
		{
			name:       "unknown phase",
			phase:      corev1.PersistentVolumePhase("Unknown"),
			wantStatus: concepts.OperationalStatusPending,
			wantReason: `PersistentVolume phase is "Unknown"`,
		},
		{
			name:       "empty phase",
			phase:      "",
			wantStatus: concepts.OperationalStatusPending,
			wantReason: `PersistentVolume phase is ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pv := &corev1.PersistentVolume{
				Status: corev1.PersistentVolumeStatus{
					Phase: tt.phase,
				},
			}
			got, err := DefaultOperationalStatusHandler(concepts.ConvergingOperationNone, pv)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}
