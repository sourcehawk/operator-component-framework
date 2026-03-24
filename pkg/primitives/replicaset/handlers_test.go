package replicaset

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestDefaultConvergingStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		op         concepts.ConvergingOperation
		rs         *appsv1.ReplicaSet
		wantStatus concepts.AliveConvergingStatus
		wantReason string
	}{
		{
			name: "ready with 1 replica (default)",
			op:   concepts.ConvergingOperationUpdated,
			rs: &appsv1.ReplicaSet{
				Spec: appsv1.ReplicaSetSpec{},
				Status: appsv1.ReplicaSetStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "All replicas are ready",
		},
		{
			name: "ready with custom replicas",
			op:   concepts.ConvergingOperationUpdated,
			rs: &appsv1.ReplicaSet{
				Spec: appsv1.ReplicaSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.ReplicaSetStatus{
					ReadyReplicas: 3,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "All replicas are ready",
		},
		{
			name: "creating",
			op:   concepts.ConvergingOperationCreated,
			rs: &appsv1.ReplicaSet{
				Spec: appsv1.ReplicaSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.ReplicaSetStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "updating",
			op:   concepts.ConvergingOperationUpdated,
			rs: &appsv1.ReplicaSet{
				Spec: appsv1.ReplicaSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.ReplicaSetStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "scaling",
			op:   concepts.ConvergingOperation("Scaling"),
			rs: &appsv1.ReplicaSet{
				Spec: appsv1.ReplicaSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.ReplicaSetStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusScaling,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "zero replicas desired and ready",
			op:   concepts.ConvergingOperationUpdated,
			rs: &appsv1.ReplicaSet{
				Spec: appsv1.ReplicaSetSpec{
					Replicas: ptr.To(int32(0)),
				},
				Status: appsv1.ReplicaSetStatus{
					ReadyReplicas: 0,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "All replicas are ready",
		},
		{
			name: "stale observed generation after create",
			op:   concepts.ConvergingOperationCreated,
			rs: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.ReplicaSetStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for replicaset controller to observe latest spec",
		},
		{
			name: "stale observed generation after update",
			op:   concepts.ConvergingOperationUpdated,
			rs: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.ReplicaSetStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for replicaset controller to observe latest spec",
		},
		{
			name: "stale observed generation with no operation",
			op:   concepts.ConvergingOperationNone,
			rs: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.ReplicaSetStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for replicaset controller to observe latest spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultConvergingStatusHandler(tt.op, tt.rs)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultGraceStatusHandler(t *testing.T) {
	t.Run("degraded (some ready)", func(t *testing.T) {
		rs := &appsv1.ReplicaSet{
			Status: appsv1.ReplicaSetStatus{
				ReadyReplicas: 1,
			},
		}
		got, err := DefaultGraceStatusHandler(rs)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "ReplicaSet partially available", got.Reason)
	})

	t.Run("down (none ready)", func(t *testing.T) {
		rs := &appsv1.ReplicaSet{
			Status: appsv1.ReplicaSetStatus{
				ReadyReplicas: 0,
			},
		}
		got, err := DefaultGraceStatusHandler(rs)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, got.Status)
		assert.Equal(t, "No replicas are ready", got.Reason)
	})
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	rs := &appsv1.ReplicaSet{}
	assert.False(t, DefaultDeleteOnSuspendHandler(rs))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Replicas: ptr.To(int32(3)),
		},
	}
	mutator := NewMutator(rs)
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	err = mutator.Apply()
	require.NoError(t, err)
	assert.Equal(t, int32(0), *rs.Spec.Replicas)
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		rs         *appsv1.ReplicaSet
		wantStatus concepts.SuspensionStatus
		wantReason string
	}{
		{
			name: "suspended",
			rs: &appsv1.ReplicaSet{
				Status: appsv1.ReplicaSetStatus{
					Replicas: 0,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspended,
			wantReason: "ReplicaSet scaled to zero",
		},
		{
			name: "suspending",
			rs: &appsv1.ReplicaSet{
				Status: appsv1.ReplicaSetStatus{
					Replicas: 2,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspending,
			wantReason: "Waiting for replicas to scale down, 2 replicas still running.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultSuspensionStatusHandler(tt.rs)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}
