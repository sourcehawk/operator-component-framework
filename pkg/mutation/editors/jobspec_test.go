package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
)

func TestJobSpecEditor(t *testing.T) {
	t.Run("SetCompletions", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetCompletions(5)
		assert.Equal(t, int32(5), *spec.Completions)
	})

	t.Run("SetParallelism", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetParallelism(3)
		assert.Equal(t, int32(3), *spec.Parallelism)
	})

	t.Run("SetBackoffLimit", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetBackoffLimit(6)
		assert.Equal(t, int32(6), *spec.BackoffLimit)
	})

	t.Run("SetActiveDeadlineSeconds", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetActiveDeadlineSeconds(300)
		assert.Equal(t, int64(300), *spec.ActiveDeadlineSeconds)
	})

	t.Run("SetTTLSecondsAfterFinished", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetTTLSecondsAfterFinished(100)
		assert.Equal(t, int32(100), *spec.TTLSecondsAfterFinished)
	})

	t.Run("SetCompletionMode", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetCompletionMode(batchv1.IndexedCompletion)
		assert.Equal(t, batchv1.IndexedCompletion, *spec.CompletionMode)
	})

	t.Run("Raw", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		assert.Equal(t, spec, editor.Raw())
	})
}
