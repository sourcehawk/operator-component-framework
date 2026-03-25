package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
)

func TestCronJobSpecEditor(t *testing.T) {
	t.Run("SetSchedule", func(t *testing.T) {
		spec := &batchv1.CronJobSpec{}
		editor := NewCronJobSpecEditor(spec)
		editor.SetSchedule("*/5 * * * *")
		assert.Equal(t, "*/5 * * * *", spec.Schedule)
	})

	t.Run("SetConcurrencyPolicy", func(t *testing.T) {
		spec := &batchv1.CronJobSpec{}
		editor := NewCronJobSpecEditor(spec)
		editor.SetConcurrencyPolicy(batchv1.ForbidConcurrent)
		assert.Equal(t, batchv1.ForbidConcurrent, spec.ConcurrencyPolicy)
	})

	t.Run("SetStartingDeadlineSeconds", func(t *testing.T) {
		spec := &batchv1.CronJobSpec{}
		editor := NewCronJobSpecEditor(spec)
		editor.SetStartingDeadlineSeconds(200)
		assert.Equal(t, int64(200), *spec.StartingDeadlineSeconds)
	})

	t.Run("SetSuccessfulJobsHistoryLimit", func(t *testing.T) {
		spec := &batchv1.CronJobSpec{}
		editor := NewCronJobSpecEditor(spec)
		editor.SetSuccessfulJobsHistoryLimit(3)
		assert.Equal(t, int32(3), *spec.SuccessfulJobsHistoryLimit)
	})

	t.Run("SetFailedJobsHistoryLimit", func(t *testing.T) {
		spec := &batchv1.CronJobSpec{}
		editor := NewCronJobSpecEditor(spec)
		editor.SetFailedJobsHistoryLimit(1)
		assert.Equal(t, int32(1), *spec.FailedJobsHistoryLimit)
	})

	t.Run("SetTimeZone", func(t *testing.T) {
		spec := &batchv1.CronJobSpec{}
		editor := NewCronJobSpecEditor(spec)
		editor.SetTimeZone("America/New_York")
		assert.Equal(t, "America/New_York", *spec.TimeZone)
	})

	t.Run("Raw", func(t *testing.T) {
		spec := &batchv1.CronJobSpec{}
		editor := NewCronJobSpecEditor(spec)
		assert.Equal(t, spec, editor.Raw())
	})
}
