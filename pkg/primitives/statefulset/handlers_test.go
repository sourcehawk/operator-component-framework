package statefulset

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"
)

func TestDefaultConvergingStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		op         concepts.ConvergingOperation
		sts        *appsv1.StatefulSet
		wantStatus concepts.AliveConvergingStatus
		wantReason string
	}{
		{
			name: "ready with 1 replica (default)",
			op:   concepts.ConvergingOperationUpdated,
			sts: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "All replicas are ready",
		},
		{
			name: "ready with custom replicas",
			op:   concepts.ConvergingOperationUpdated,
			sts: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas: 3,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "All replicas are ready",
		},
		{
			name: "creating",
			op:   concepts.ConvergingOperationCreated,
			sts: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "updating",
			op:   concepts.ConvergingOperationUpdated,
			sts: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "scaling",
			op:   concepts.ConvergingOperation("Scaling"),
			sts: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusScaling,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultConvergingStatusHandler(tt.op, tt.sts)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultGraceStatusHandler(t *testing.T) {
	t.Run("degraded (some ready)", func(t *testing.T) {
		sts := &appsv1.StatefulSet{
			Status: appsv1.StatefulSetStatus{
				ReadyReplicas: 1,
			},
		}
		got, err := DefaultGraceStatusHandler(sts)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "StatefulSet partially available", got.Reason)
	})

	t.Run("down (none ready)", func(t *testing.T) {
		sts := &appsv1.StatefulSet{
			Status: appsv1.StatefulSetStatus{
				ReadyReplicas: 0,
			},
		}
		got, err := DefaultGraceStatusHandler(sts)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, got.Status)
		assert.Equal(t, "No replicas are ready", got.Reason)
	})
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	sts := &appsv1.StatefulSet{}
	assert.False(t, DefaultDeleteOnSuspendHandler(sts))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(int32(3)),
		},
	}
	mutator := NewMutator(sts)
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	err = mutator.Apply()
	require.NoError(t, err)
	assert.Equal(t, int32(0), *sts.Spec.Replicas)
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		sts        *appsv1.StatefulSet
		wantStatus concepts.SuspensionStatus
		wantReason string
	}{
		{
			name: "suspended",
			sts: &appsv1.StatefulSet{
				Status: appsv1.StatefulSetStatus{
					Replicas: 0,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspended,
			wantReason: "StatefulSet scaled to zero",
		},
		{
			name: "suspending",
			sts: &appsv1.StatefulSet{
				Status: appsv1.StatefulSetStatus{
					Replicas: 2,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspending,
			wantReason: "Waiting for replicas to scale down, 2 replicas still running.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultSuspensionStatusHandler(tt.sts)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}
