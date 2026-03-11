package deployment

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"
)

func TestDefaultConvergingStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		op         component.ConvergingOperation
		deployment *appsv1.Deployment
		wantStatus component.ConvergingStatus
		wantReason string
	}{
		{
			name: "ready with 1 replica (default)",
			op:   component.ConvergingOperationUpdated,
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: component.ConvergingStatusReady,
			wantReason: "All replicas are ready",
		},
		{
			name: "ready with custom replicas",
			op:   component.ConvergingOperationUpdated,
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 3,
				},
			},
			wantStatus: component.ConvergingStatusReady,
			wantReason: "All replicas are ready",
		},
		{
			name: "creating",
			op:   component.ConvergingOperationCreated,
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: component.ConvergingStatusCreating,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "updating",
			op:   component.ConvergingOperationUpdated,
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: component.ConvergingStatusUpdating,
			wantReason: "Waiting for replicas: 1/3 ready",
		},
		{
			name: "scaling",
			op:   component.ConvergingOperation("Scaling"),
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			wantStatus: component.ConvergingStatusScaling,
			wantReason: "Waiting for replicas: 1/3 ready",
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
	t.Run("degraded (some ready)", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 1,
			},
		}
		got, err := DefaultGraceStatusHandler(deployment)
		require.NoError(t, err)
		assert.Equal(t, component.GraceStatusDegraded, got.Status)
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
		assert.Equal(t, component.GraceStatusDown, got.Status)
		assert.Equal(t, "No replicas are ready", got.Reason)
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
		wantStatus component.SuspensionStatus
		wantReason string
	}{
		{
			name: "suspended",
			deployment: &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Replicas: 0,
				},
			},
			wantStatus: component.SuspensionStatusSuspended,
			wantReason: "Deployment scaled to zero",
		},
		{
			name: "suspending",
			deployment: &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Replicas: 2,
				},
			},
			wantStatus: component.SuspensionStatusSuspending,
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
