package deployment

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
		deployment *appsv1.Deployment
		wantStatus concepts.AliveConvergingStatus
		wantReason string
	}{
		{
			name: "ready with 1 replica (default)",
			op:   concepts.ConvergingOperationUpdated,
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{},
				Status: appsv1.DeploymentStatus{
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
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					UpdatedReplicas:    3,
					ReadyReplicas:      3,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "All replicas are ready",
		},
		{
			name: "creating",
			op:   concepts.ConvergingOperationCreated,
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "updating",
			op:   concepts.ConvergingOperationUpdated,
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "scaling",
			op:   concepts.ConvergingOperation("Scaling"),
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusScaling,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "stale observed generation after create",
			op:   concepts.ConvergingOperationCreated,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for deployment controller to observe latest spec",
		},
		{
			name: "stale observed generation after update",
			op:   concepts.ConvergingOperationUpdated,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for deployment controller to observe latest spec",
		},
		{
			name: "stale observed generation with no operation",
			op:   concepts.ConvergingOperationNone,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for deployment controller to observe latest spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultConvergingStatusHandler(tt.op, tt.deployment)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultGraceStatusHandler(t *testing.T) {
	t.Run("healthy (all ready)", func(t *testing.T) {
		replicas := int32(3)
		deployment := &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        3,
				UpdatedReplicas: 3,
				ReadyReplicas:   3,
			},
		}
		got, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
		assert.Equal(t, "All replicas are ready", got.Reason)
	})

	t.Run("healthy (nil replicas, one ready)", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				Replicas:        1,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
		}
		got, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
		assert.Equal(t, "All replicas are ready", got.Reason)
	})

	t.Run("degraded (ready exceeds desired)", func(t *testing.T) {
		replicas := int32(1)
		deployment := &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 3,
			},
		}
		got, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "Deployment partially available", got.Reason)
	})

	t.Run("degraded (some ready)", func(t *testing.T) {
		replicas := int32(3)
		deployment := &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 1,
			},
		}
		got, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "Deployment partially available", got.Reason)
	})

	t.Run("down (none ready)", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 0,
			},
		}
		got, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, got.Status)
		assert.Equal(t, "No replicas are ready", got.Reason)
	})
}

// TestDefaultHandlers_UnfinishedRollout covers a Deployment whose ready replica
// count matches the desired count while its rollout is not complete. The
// framework reports the converging status until the grace period expires and the
// grace status after it, so both handlers must report the rollout as not healthy.
// A Healthy grace status here is what makes the framework log a grace
// inconsistency and keep the progress reason.
func TestDefaultHandlers_UnfinishedRollout(t *testing.T) {
	tests := []struct {
		name            string
		op              concepts.ConvergingOperation
		deployment      *appsv1.Deployment
		wantConverge    concepts.AliveConvergingStatus
		wantGrace       concepts.GraceStatus
		wantReason      string
		wantGraceReason string
	}{
		{
			name: "stale observed generation with all replicas ready",
			op:   concepts.ConvergingOperationNone,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					UpdatedReplicas:    3,
					ReadyReplicas:      3,
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDegraded,
			wantReason:      "Waiting for deployment controller to observe latest spec",
			wantGraceReason: "Waiting for deployment controller to observe latest spec",
		},
		{
			name: "stale observed generation with no replicas ready",
			op:   concepts.ConvergingOperationNone,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDown,
			wantReason:      "Waiting for deployment controller to observe latest spec",
			wantGraceReason: "No replicas are ready",
		},
		{
			name: "new replicas not updated while all old replicas stay ready",
			op:   concepts.ConvergingOperationNone,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
					Replicas:           4,
					UpdatedReplicas:    1,
					ReadyReplicas:      3,
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDegraded,
			wantReason:      "Waiting for rollout: 1/3 replicas updated",
			wantGraceReason: "Waiting for rollout: 1/3 replicas updated",
		},
		{
			name: "all replicas updated while old replicas remain",
			op:   concepts.ConvergingOperationNone,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 2,
					Replicas:           6,
					UpdatedReplicas:    3,
					ReadyReplicas:      3,
				},
			},
			wantConverge:    concepts.AliveConvergingStatusUpdating,
			wantGrace:       concepts.GraceStatusDegraded,
			wantReason:      "Waiting for rollout: 3 old replicas pending termination",
			wantGraceReason: "Waiting for rollout: 3 old replicas pending termination",
		},
		{
			name: "unfinished rollout of a just created deployment",
			op:   concepts.ConvergingOperationCreated,
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(3))},
				Status: appsv1.DeploymentStatus{
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
			converge, err := DefaultConvergingStatusHandler(tt.op, tt.deployment)
			require.NoError(t, err)
			assert.Equal(t, tt.wantConverge, converge.Status)
			assert.Equal(t, tt.wantReason, converge.Reason)

			grace, err := DefaultGraceStatusHandler(tt.deployment)
			require.NoError(t, err)
			assert.Equal(t, tt.wantGrace, grace.Status)
			assert.Equal(t, tt.wantGraceReason, grace.Reason)
		})
	}
}

// TestDefaultHandlers_PausedRollout covers a paused Deployment whose pod template
// changed. The deployment controller does not roll out a paused Deployment, so
// the handlers do not wait for the rollout, but they still require the current
// generation to be observed and all desired replicas to be ready.
func TestDefaultHandlers_PausedRollout(t *testing.T) {
	paused := func(observedGeneration, readyReplicas int32) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Generation: 2},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(3)),
				Paused:   true,
			},
			Status: appsv1.DeploymentStatus{
				ObservedGeneration: int64(observedGeneration),
				Replicas:           3,
				ReadyReplicas:      readyReplicas,
			},
		}
	}

	t.Run("healthy when all old replicas are ready", func(t *testing.T) {
		deployment := paused(2, 3)

		converge, err := DefaultConvergingStatusHandler(concepts.ConvergingOperationNone, deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.AliveConvergingStatusHealthy, converge.Status)

		grace, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, grace.Status)
	})

	t.Run("not healthy while the generation is stale", func(t *testing.T) {
		deployment := paused(1, 3)

		converge, err := DefaultConvergingStatusHandler(concepts.ConvergingOperationNone, deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.AliveConvergingStatusUpdating, converge.Status)

		grace, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, grace.Status)
	})

	t.Run("not healthy while replicas are not ready", func(t *testing.T) {
		deployment := paused(2, 2)

		converge, err := DefaultConvergingStatusHandler(concepts.ConvergingOperationNone, deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.AliveConvergingStatusScaling, converge.Status)

		grace, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, grace.Status)
	})
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	deploy := &appsv1.Deployment{}
	assert.False(t, DefaultDeleteOnSuspendHandler(deploy))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
		},
	}
	mutator := NewMutator(deploy)
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	err = mutator.Apply()
	require.NoError(t, err)
	assert.Equal(t, int32(0), *deploy.Spec.Replicas)
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		deployment *appsv1.Deployment
		wantStatus concepts.SuspensionStatus
		wantReason string
	}{
		{
			name: "suspended",
			deployment: &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Replicas: 0,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspended,
			wantReason: "Deployment scaled to zero",
		},
		{
			name: "suspending",
			deployment: &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Replicas: 2,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspending,
			wantReason: "Waiting for replicas to scale down, 2 replicas still running.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultSuspensionStatusHandler(tt.deployment)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}
