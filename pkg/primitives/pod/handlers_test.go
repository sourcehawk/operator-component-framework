package pod

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestDefaultConvergingStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		op         concepts.ConvergingOperation
		pod        *corev1.Pod
		wantStatus concepts.AliveConvergingStatus
		wantReason string
	}{
		{
			name: "healthy - running and all containers ready",
			op:   concepts.ConvergingOperationUpdated,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "app", Ready: true},
					},
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "Pod is running and all containers are ready",
		},
		{
			name: "healthy - running with no container statuses",
			op:   concepts.ConvergingOperationNone,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "Pod is running and all containers are ready",
		},
		{
			name: "failing - crash loop backoff",
			op:   concepts.ConvergingOperationNone,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "app",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "CrashLoopBackOff",
								},
							},
						},
					},
				},
			},
			wantStatus: concepts.AliveConvergingStatusFailing,
			wantReason: "Container app is in CrashLoopBackOff",
		},
		{
			name: "failing - terminated with error",
			op:   concepts.ConvergingOperationNone,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "worker",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 1,
								},
							},
						},
					},
				},
			},
			wantStatus: concepts.AliveConvergingStatusFailing,
			wantReason: "Container worker terminated with error",
		},
		{
			name: "creating - on created operation",
			op:   concepts.ConvergingOperationCreated,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Pod is starting",
		},
		{
			name: "updating - on updated operation",
			op:   concepts.ConvergingOperationUpdated,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Pod is being recreated",
		},
		{
			name: "creating - on none operation when pending",
			op:   concepts.ConvergingOperationNone,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Pod is pending",
		},
		{
			name: "running but not all containers ready",
			op:   concepts.ConvergingOperationNone,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "app", Ready: true},
						{Name: "sidecar", Ready: false},
					},
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Pod running but not all containers ready",
		},
		{
			name: "failed pod phase",
			op:   concepts.ConvergingOperationNone,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
				},
			},
			wantStatus: concepts.AliveConvergingStatusFailing,
			wantReason: "Pod has failed",
		},
		{
			name: "succeeded pod phase",
			op:   concepts.ConvergingOperationNone,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "Pod has completed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultConvergingStatusHandler(tt.op, tt.pod)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultGraceStatusHandler(t *testing.T) {
	t.Run("degraded (running, no container statuses)", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		got, err := DefaultGraceStatusHandler(pod)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "Pod running but container readiness unknown", got.Reason)
	})

	t.Run("degraded (running, not all containers ready)", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", Ready: true},
					{Name: "sidecar", Ready: false},
				},
			},
		}
		got, err := DefaultGraceStatusHandler(pod)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "Pod running but not all containers ready", got.Reason)
	})

	t.Run("healthy (running, all containers ready)", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", Ready: true},
				},
			},
		}
		got, err := DefaultGraceStatusHandler(pod)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
		assert.Equal(t, "Pod running and all containers ready", got.Reason)
	})

	t.Run("down (not running)", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}
		got, err := DefaultGraceStatusHandler(pod)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, got.Status)
		assert.Equal(t, "Pod is not running", got.Reason)
	})

	t.Run("down (failed phase)", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
			},
		}
		got, err := DefaultGraceStatusHandler(pod)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, got.Status)
	})
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	pod := &corev1.Pod{}
	assert.True(t, DefaultDeleteOnSuspendHandler(pod))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	pod := &corev1.Pod{}
	mutator := NewMutator(pod)
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	// No-op; just verify no error
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	pod := &corev1.Pod{}
	got, err := DefaultSuspensionStatusHandler(pod)
	require.NoError(t, err)
	assert.Equal(t, concepts.SuspensionStatusSuspended, got.Status)
	assert.Equal(t, "Pod deleted on suspend", got.Reason)
}
