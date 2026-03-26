package cronjob

import (
	"testing"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestDefaultOperationalStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		op         concepts.ConvergingOperation
		cronjob    *batchv1.CronJob
		wantStatus concepts.OperationalStatus
		wantReason string
	}{
		{
			name: "pending when never scheduled",
			op:   concepts.ConvergingOperationCreated,
			cronjob: &batchv1.CronJob{
				Status: batchv1.CronJobStatus{},
			},
			wantStatus: concepts.OperationalStatusPending,
			wantReason: "CronJob has never been scheduled",
		},
		{
			name: "operational when scheduled at least once",
			op:   concepts.ConvergingOperationUpdated,
			cronjob: &batchv1.CronJob{
				Status: batchv1.CronJobStatus{
					LastScheduleTime: &metav1.Time{Time: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)},
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "CronJob last scheduled at 2025-01-01T12:00:00Z",
		},
		{
			name: "operational regardless of converging operation",
			op:   concepts.ConvergingOperationNone,
			cronjob: &batchv1.CronJob{
				Status: batchv1.CronJobStatus{
					LastScheduleTime: &metav1.Time{Time: time.Date(2025, 6, 15, 8, 30, 0, 0, time.UTC)},
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
			wantReason: "CronJob last scheduled at 2025-06-15T08:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultOperationalStatusHandler(tt.op, tt.cronjob)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
		},
	}
	mutator := NewMutator(cj)
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	err = mutator.Apply()
	require.NoError(t, err)
	require.NotNil(t, cj.Spec.Suspend)
	assert.True(t, *cj.Spec.Suspend)
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		cronjob    *batchv1.CronJob
		wantStatus concepts.SuspensionStatus
		wantReason string
	}{
		{
			name: "suspended with no active jobs",
			cronjob: &batchv1.CronJob{
				Spec: batchv1.CronJobSpec{
					Suspend: ptr.To(true),
				},
				Status: batchv1.CronJobStatus{},
			},
			wantStatus: concepts.SuspensionStatusSuspended,
			wantReason: "CronJob suspended",
		},
		{
			name: "suspending with active jobs",
			cronjob: &batchv1.CronJob{
				Spec: batchv1.CronJobSpec{
					Suspend: ptr.To(true),
				},
				Status: batchv1.CronJobStatus{
					Active: []corev1.ObjectReference{
						{Name: "job-1"},
						{Name: "job-2"},
					},
				},
			},
			wantStatus: concepts.SuspensionStatusSuspending,
			wantReason: "CronJob suspended but 2 active jobs still running",
		},
		{
			name: "suspending when suspend flag not yet applied",
			cronjob: &batchv1.CronJob{
				Spec: batchv1.CronJobSpec{},
			},
			wantStatus: concepts.SuspensionStatusSuspending,
			wantReason: "Waiting for suspend flag to be applied",
		},
		{
			name: "suspending when suspend is false",
			cronjob: &batchv1.CronJob{
				Spec: batchv1.CronJobSpec{
					Suspend: ptr.To(false),
				},
			},
			wantStatus: concepts.SuspensionStatusSuspending,
			wantReason: "Waiting for suspend flag to be applied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultSuspensionStatusHandler(tt.cronjob)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultGraceStatusHandler(t *testing.T) {
	t.Run("healthy when never scheduled", func(t *testing.T) {
		got, err := DefaultGraceStatusHandler(&batchv1.CronJob{})
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
	})

	t.Run("healthy when scheduled", func(t *testing.T) {
		cj := &batchv1.CronJob{
			Status: batchv1.CronJobStatus{
				LastScheduleTime: &metav1.Time{Time: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)},
			},
		}
		got, err := DefaultGraceStatusHandler(cj)
		require.NoError(t, err)
		assert.Equal(t, concepts.GraceStatusHealthy, got.Status)
	})
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	cj := &batchv1.CronJob{}
	assert.False(t, DefaultDeleteOnSuspendHandler(cj))
}
