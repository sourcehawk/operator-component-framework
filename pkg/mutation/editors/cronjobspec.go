package editors

import (
	batchv1 "k8s.io/api/batch/v1"
)

// CronJobSpecEditor provides a typed API for mutating a Kubernetes CronJobSpec.
//
// Note: spec.suspend is NOT exposed here — it is managed by the framework's suspension
// system via DefaultSuspendMutationHandler.
type CronJobSpecEditor struct {
	spec *batchv1.CronJobSpec
}

// NewCronJobSpecEditor creates a new CronJobSpecEditor for the given CronJobSpec.
func NewCronJobSpecEditor(spec *batchv1.CronJobSpec) *CronJobSpecEditor {
	return &CronJobSpecEditor{spec: spec}
}

// Raw returns the underlying *batchv1.CronJobSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *CronJobSpecEditor) Raw() *batchv1.CronJobSpec {
	return e.spec
}

// SetSchedule sets the cron schedule expression.
func (e *CronJobSpecEditor) SetSchedule(cron string) {
	e.spec.Schedule = cron
}

// SetConcurrencyPolicy sets the concurrency policy for the CronJob.
func (e *CronJobSpecEditor) SetConcurrencyPolicy(policy batchv1.ConcurrencyPolicy) {
	e.spec.ConcurrencyPolicy = policy
}

// SetStartingDeadlineSeconds sets the optional deadline in seconds for starting the job
// if it misses its scheduled time.
func (e *CronJobSpecEditor) SetStartingDeadlineSeconds(seconds int64) {
	e.spec.StartingDeadlineSeconds = &seconds
}

// SetSuccessfulJobsHistoryLimit sets the number of successful finished jobs to retain.
func (e *CronJobSpecEditor) SetSuccessfulJobsHistoryLimit(n int32) {
	e.spec.SuccessfulJobsHistoryLimit = &n
}

// SetFailedJobsHistoryLimit sets the number of failed finished jobs to retain.
func (e *CronJobSpecEditor) SetFailedJobsHistoryLimit(n int32) {
	e.spec.FailedJobsHistoryLimit = &n
}

// SetTimeZone sets the time zone for the cron schedule.
func (e *CronJobSpecEditor) SetTimeZone(tz string) {
	e.spec.TimeZone = &tz
}
