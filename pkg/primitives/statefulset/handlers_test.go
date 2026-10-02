package statefulset

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
					Replicas:        1,
					UpdatedReplicas: 1,
					ReadyReplicas:   1,
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
					Replicas:        3,
					UpdatedReplicas: 3,
					ReadyReplicas:   3,
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
		{
			name: "stale observed generation after create",
			op:   concepts.ConvergingOperationCreated,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for statefulset controller to observe latest spec",
		},
		{
			name: "stale observed generation after update",
			op:   concepts.ConvergingOperationUpdated,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for statefulset controller to observe latest spec",
		},
		{
			name: "stale observed generation with no operation",
			op:   concepts.ConvergingOperationNone,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for statefulset controller to observe latest spec",
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
	t.Run("healthy (all ready)", func(t *testing.T) {
		replicas := int32(3)
		sts := &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
			},
			Status: appsv1.StatefulSetStatus{
				Replicas:        3,
				UpdatedReplicas: 3,
				ReadyReplicas:   3,
			},
		}
		got, err := DefaultGraceStatusHandler(sts)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
		assert.Equal(t, "All replicas are ready", got.Reason)
	})

	t.Run("healthy (nil replicas, one ready)", func(t *testing.T) {
		sts := &appsv1.StatefulSet{
			Status: appsv1.StatefulSetStatus{
				Replicas:        1,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
		}
		got, err := DefaultGraceStatusHandler(sts)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
		assert.Equal(t, "All replicas are ready", got.Reason)
	})

	t.Run("degraded (ready exceeds desired)", func(t *testing.T) {
		replicas := int32(1)
		sts := &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
			},
			Status: appsv1.StatefulSetStatus{
				ReadyReplicas: 3,
			},
		}
		got, err := DefaultGraceStatusHandler(sts)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "StatefulSet partially available", got.Reason)
	})

	t.Run("degraded (some ready)", func(t *testing.T) {
		replicas := int32(3)
		sts := &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
			},
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

// TestDefaultHandlers_UnfinishedRollout covers a StatefulSet whose ready replica
// count matches the desired count while its rollout is not complete. The
// framework reports the converging status until the grace period expires and the
// grace status after it, so both handlers must report the rollout as not healthy.
// A Healthy grace status here is what makes the framework log a grace
// inconsistency and keep the progress reason.
func TestDefaultHandlers_UnfinishedRollout(t *testing.T) {
	tests := []struct {
		name            string
		op              concepts.ConvergingOperation
		sts             *appsv1.StatefulSet
		wantConverge    concepts.AliveConvergingStatus
		wantGrace       concepts.GraceStatus
		wantReason      string
		wantGraceReason string
	}{
		{
			name: "stale observed generation with all replicas ready",
			op:   concepts.ConvergingOperationNone,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					UpdatedReplicas:    3,
					ReadyReplicas:      3,
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDegraded,
			wantReason:      "Waiting for statefulset controller to observe latest spec",
			wantGraceReason: "Waiting for statefulset controller to observe latest spec",
		},
		{
			name: "stale observed generation with no replicas ready",
			op:   concepts.ConvergingOperationNone,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDown,
			wantReason:      "Waiting for statefulset controller to observe latest spec",
			wantGraceReason: "No replicas are ready",
		},
		{
			name: "replicas not updated while all old replicas stay ready",
			op:   concepts.ConvergingOperationNone,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					UpdatedReplicas:    1,
					ReadyReplicas:      3,
					CurrentRevision:    "web-1",
					UpdateRevision:     "web-2",
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDegraded,
			wantReason:      "Waiting for rollout: 1/3 replicas updated",
			wantGraceReason: "Waiting for rollout: 1/3 replicas updated",
		},
		{
			name: "all replicas updated before the controller completes the rolling update",
			op:   concepts.ConvergingOperationNone,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					UpdatedReplicas:    3,
					ReadyReplicas:      3,
					CurrentRevision:    "web-1",
					UpdateRevision:     "web-2",
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDegraded,
			wantReason:      "Waiting for rollout to revision web-2",
			wantGraceReason: "Waiting for rollout to revision web-2",
		},
		{
			name: "partitioned replicas not updated",
			op:   concepts.ConvergingOperationNone,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
							Partition: ptr.To(int32(1)),
						},
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					UpdatedReplicas:    1,
					ReadyReplicas:      3,
					CurrentRevision:    "web-1",
					UpdateRevision:     "web-2",
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDegraded,
			wantReason:      "Waiting for partitioned rollout: 1/2 replicas updated",
			wantGraceReason: "Waiting for partitioned rollout: 1/2 replicas updated",
		},
		{
			name: "unfinished rollout of a just created statefulset",
			op:   concepts.ConvergingOperationCreated,
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					Replicas:           3,
					UpdatedReplicas:    2,
					ReadyReplicas:      3,
				},
			},
			wantConverge:    concepts.AliveConvergingStatusCreating,
			wantGrace:       concepts.GraceStatusDegraded,
			wantReason:      "Waiting for rollout: 2/3 replicas updated",
			wantGraceReason: "Waiting for rollout: 2/3 replicas updated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converge, err := DefaultConvergingStatusHandler(tt.op, tt.sts)
			require.NoError(t, err)
			assert.Equal(t, tt.wantConverge, converge.Status)
			assert.Equal(t, tt.wantReason, converge.Reason)

			grace, err := DefaultGraceStatusHandler(tt.sts)
			require.NoError(t, err)
			assert.Equal(t, tt.wantGrace, grace.Status)
			assert.Equal(t, tt.wantGraceReason, grace.Reason)
		})
	}
}

// TestDefaultHandlers_RolloutHeldBack covers update strategies under which the
// statefulset controller does not move every replica to the update revision by
// itself. The handlers report the StatefulSet as healthy once the replicas that
// the strategy updates are updated and all replicas are ready.
func TestDefaultHandlers_RolloutHeldBack(t *testing.T) {
	tests := []struct {
		name string
		sts  *appsv1.StatefulSet
	}{
		{
			name: "partition holds back the remaining replicas",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
							Partition: ptr.To(int32(1)),
						},
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					UpdatedReplicas:    2,
					ReadyReplicas:      3,
					CurrentRevision:    "web-1",
					UpdateRevision:     "web-2",
				},
			},
		},
		{
			name: "partition at or above the replica count holds back all replicas",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
							Partition: ptr.To(int32(5)),
						},
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					ReadyReplicas:      3,
					CurrentRevision:    "web-1",
					UpdateRevision:     "web-2",
				},
			},
		},
		{
			name: "OnDelete leaves replicas on the old revision",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.OnDeleteStatefulSetStrategyType,
					},
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					ReadyReplicas:      3,
					CurrentRevision:    "web-1",
					UpdateRevision:     "web-2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converge, err := DefaultConvergingStatusHandler(concepts.ConvergingOperationNone, tt.sts)
			require.NoError(t, err)
			assert.Equal(t, concepts.AliveConvergingStatusHealthy, converge.Status)

			grace, err := DefaultGraceStatusHandler(tt.sts)
			require.NoError(t, err)
			assert.Equal(t, concepts.GraceStatusHealthy, grace.Status)
		})
	}
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
