package daemonset

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultConvergingStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		op         concepts.ConvergingOperation
		daemonset  *appsv1.DaemonSet
		wantStatus concepts.AliveConvergingStatus
		wantReason string
	}{
		{
			name: "ready with all pods scheduled",
			op:   concepts.ConvergingOperationUpdated,
			daemonset: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberReady:            3,
					ObservedGeneration:     1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "All pods are ready",
		},
		{
			name: "ready with more than desired",
			op:   concepts.ConvergingOperationUpdated,
			daemonset: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberReady:            5,
					ObservedGeneration:     1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "All pods are ready",
		},
		{
			name: "stale observed generation with desired pods ready reports updating not healthy",
			op:   concepts.ConvergingOperationUpdated,
			daemonset: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberReady:            3,
					ObservedGeneration:     1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for DaemonSet controller to observe latest spec",
		},
		{
			name: "stale observed generation after create",
			op:   concepts.ConvergingOperationCreated,
			daemonset: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberReady:            3,
					ObservedGeneration:     1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for DaemonSet controller to observe latest spec",
		},
		{
			name: "stale observed generation with no operation",
			op:   concepts.ConvergingOperationNone,
			daemonset: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberReady:            3,
					ObservedGeneration:     1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for DaemonSet controller to observe latest spec",
		},
		{
			name: "creating",
			op:   concepts.ConvergingOperationCreated,
			daemonset: &appsv1.DaemonSet{
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberReady:            1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for pods: 1/3 ready",
		},
		{
			name: "updating",
			op:   concepts.ConvergingOperationUpdated,
			daemonset: &appsv1.DaemonSet{
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberReady:            1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusUpdating,
			wantReason: "Waiting for pods: 1/3 ready",
		},
		{
			name: "scaling",
			op:   concepts.ConvergingOperation("Scaling"),
			daemonset: &appsv1.DaemonSet{
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberReady:            1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusScaling,
			wantReason: "Waiting for pods: 1/3 ready",
		},
		{
			name: "zero desired with observed generation is healthy",
			op:   concepts.ConvergingOperationCreated,
			daemonset: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 0,
					NumberReady:            0,
					ObservedGeneration:     1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusHealthy,
			wantReason: "No nodes match the DaemonSet node selector",
		},
		{
			name: "zero desired with stale generation is not ready",
			op:   concepts.ConvergingOperationCreated,
			daemonset: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 0,
					NumberReady:            0,
					ObservedGeneration:     1,
				},
			},
			wantStatus: concepts.AliveConvergingStatusCreating,
			wantReason: "Waiting for DaemonSet controller to observe latest spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultConvergingStatusHandler(tt.op, tt.daemonset)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultGraceStatusHandler(t *testing.T) {
	t.Run("healthy when no nodes match selector and generation observed", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 0,
				NumberReady:            0,
				ObservedGeneration:     1,
			},
		}
		got, err := DefaultGraceStatusHandler(ds)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
		assert.Equal(t, "No nodes match the DaemonSet node selector", got.Reason)
	})

	t.Run("degraded when no nodes match selector but generation stale", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Generation: 2},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 0,
				NumberReady:            0,
				ObservedGeneration:     1,
			},
		}
		got, err := DefaultGraceStatusHandler(ds)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "Waiting for DaemonSet controller to observe latest generation", got.Reason)
	})

	t.Run("healthy (all ready)", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				NumberReady:            3,
			},
		}
		got, err := DefaultGraceStatusHandler(ds)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
		assert.Equal(t, "All pods are ready", got.Reason)
	})

	t.Run("degraded (ready exceeds desired)", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 2,
				NumberReady:            3,
			},
		}
		got, err := DefaultGraceStatusHandler(ds)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "DaemonSet partially available", got.Reason)
	})

	t.Run("degraded (some ready)", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				NumberReady:            1,
			},
		}
		got, err := DefaultGraceStatusHandler(ds)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDegraded, got.Status)
		assert.Equal(t, "DaemonSet partially available", got.Reason)
	})

	t.Run("down (none ready)", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				NumberReady:            0,
			},
		}
		got, err := DefaultGraceStatusHandler(ds)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusDown, got.Status)
		assert.Equal(t, "No pods are ready", got.Reason)
	})
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	assert.True(t, DefaultDeleteOnSuspendHandler(ds))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	mutator := NewMutator(ds)
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	got, err := DefaultSuspensionStatusHandler(ds)
	require.NoError(t, err)
	assert.Equal(t, concepts.SuspensionStatusSuspended, got.Status)
	assert.Equal(t, "DaemonSet deleted on suspend", got.Reason)
}
