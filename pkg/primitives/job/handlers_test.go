package job

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestDefaultConvergingStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		op         concepts.ConvergingOperation
		job        *batchv1.Job
		wantStatus concepts.CompletionStatus
		wantReason string
	}{
		{
			name: "completed",
			op:   concepts.ConvergingOperationUpdated,
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
					},
				},
			},
			wantStatus: concepts.CompletionStatusCompleted,
			wantReason: "Job completed successfully",
		},
		{
			name: "failed",
			op:   concepts.ConvergingOperationUpdated,
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "BackoffLimitExceeded"},
					},
				},
			},
			wantStatus: concepts.CompletionStatusFailing,
			wantReason: "Job failed: BackoffLimitExceeded",
		},
		{
			name: "failed with reason only",
			op:   concepts.ConvergingOperationUpdated,
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded"},
					},
				},
			},
			wantStatus: concepts.CompletionStatusFailing,
			wantReason: "Job failed: BackoffLimitExceeded",
		},
		{
			name: "failed without message or reason",
			op:   concepts.ConvergingOperationUpdated,
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
					},
				},
			},
			wantStatus: concepts.CompletionStatusFailing,
			wantReason: "Job failed",
		},
		{
			name: "running",
			op:   concepts.ConvergingOperationUpdated,
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Active: 2,
				},
			},
			wantStatus: concepts.CompletionStatusRunning,
			wantReason: "Job has 2 active pod(s)",
		},
		{
			name: "pending after creation",
			op:   concepts.ConvergingOperationCreated,
			job: &batchv1.Job{
				Status: batchv1.JobStatus{},
			},
			wantStatus: concepts.CompletionStatusPending,
			wantReason: "Job was just created, waiting for pods to be scheduled",
		},
		{
			name: "pending after update",
			op:   concepts.ConvergingOperationUpdated,
			job: &batchv1.Job{
				Status: batchv1.JobStatus{},
			},
			wantStatus: concepts.CompletionStatusPending,
			wantReason: "Job is waiting for pods to be scheduled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultConvergingStatusHandler(tt.op, tt.job)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	job := &batchv1.Job{}
	assert.True(t, DefaultDeleteOnSuspendHandler(job))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	job := &batchv1.Job{}
	mutator := NewMutator(job)
	mutator.BeginFeature()
	err := DefaultSuspendMutationHandler(mutator)
	require.NoError(t, err)
	err = mutator.Apply()
	require.NoError(t, err)
	require.NotNil(t, job.Spec.Suspend)
	assert.True(t, *job.Spec.Suspend)
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		job        *batchv1.Job
		wantStatus concepts.SuspensionStatus
		wantReason string
	}{
		{
			name: "suspended",
			job: &batchv1.Job{
				Spec: batchv1.JobSpec{
					Suspend: boolPtr(true),
				},
				Status: batchv1.JobStatus{
					Active: 0,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspended,
			wantReason: "Job is suspended",
		},
		{
			name: "suspending with active pods",
			job: &batchv1.Job{
				Spec: batchv1.JobSpec{
					Suspend: boolPtr(true),
				},
				Status: batchv1.JobStatus{
					Active: 2,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspending,
			wantReason: "Job is suspending, 2 pod(s) still active",
		},
		{
			name: "waiting for suspend field",
			job: &batchv1.Job{
				Spec: batchv1.JobSpec{},
				Status: batchv1.JobStatus{
					Active: 1,
				},
			},
			wantStatus: concepts.SuspensionStatusSuspending,
			wantReason: "Waiting for Job suspend field to be applied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultSuspensionStatusHandler(tt.job)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}
