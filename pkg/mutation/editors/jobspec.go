package editors

import (
	batchv1 "k8s.io/api/batch/v1"
)

// JobSpecEditor provides a typed API for mutating a Kubernetes JobSpec.
type JobSpecEditor struct {
	spec *batchv1.JobSpec
}

// NewJobSpecEditor creates a new JobSpecEditor for the given JobSpec.
func NewJobSpecEditor(spec *batchv1.JobSpec) *JobSpecEditor {
	return &JobSpecEditor{spec: spec}
}

// Raw returns the underlying *batchv1.JobSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *JobSpecEditor) Raw() *batchv1.JobSpec {
	return e.spec
}

// SetCompletions sets the desired number of successfully finished pods.
func (e *JobSpecEditor) SetCompletions(n int32) {
	e.spec.Completions = &n
}

// SetParallelism sets the maximum number of pods running in parallel.
func (e *JobSpecEditor) SetParallelism(n int32) {
	e.spec.Parallelism = &n
}

// SetBackoffLimit sets the number of retries before marking the job as failed.
func (e *JobSpecEditor) SetBackoffLimit(n int32) {
	e.spec.BackoffLimit = &n
}

// SetActiveDeadlineSeconds sets the maximum duration in seconds for the job to run.
func (e *JobSpecEditor) SetActiveDeadlineSeconds(seconds int64) {
	e.spec.ActiveDeadlineSeconds = &seconds
}

// SetTTLSecondsAfterFinished sets how long (in seconds) to keep the job after it finishes.
func (e *JobSpecEditor) SetTTLSecondsAfterFinished(seconds int32) {
	e.spec.TTLSecondsAfterFinished = &seconds
}

// SetCompletionMode sets the completion mode of the job (e.g. NonIndexed or Indexed).
func (e *JobSpecEditor) SetCompletionMode(mode batchv1.CompletionMode) {
	e.spec.CompletionMode = &mode
}
