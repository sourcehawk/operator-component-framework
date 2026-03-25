package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
)

func TestJobSpecEditor(t *testing.T) {
	t.Run("SetCompletions", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetCompletions(5)
		require.NotNil(t, spec.Completions)
		assert.Equal(t, int32(5), *spec.Completions)
	})

	t.Run("SetParallelism", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetParallelism(3)
		require.NotNil(t, spec.Parallelism)
		assert.Equal(t, int32(3), *spec.Parallelism)
	})

	t.Run("SetBackoffLimit", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetBackoffLimit(6)
		require.NotNil(t, spec.BackoffLimit)
		assert.Equal(t, int32(6), *spec.BackoffLimit)
	})

	t.Run("SetActiveDeadlineSeconds", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetActiveDeadlineSeconds(300)
		require.NotNil(t, spec.ActiveDeadlineSeconds)
		assert.Equal(t, int64(300), *spec.ActiveDeadlineSeconds)
	})

	t.Run("SetTTLSecondsAfterFinished", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetTTLSecondsAfterFinished(100)
		require.NotNil(t, spec.TTLSecondsAfterFinished)
		assert.Equal(t, int32(100), *spec.TTLSecondsAfterFinished)
	})

	t.Run("SetCompletionMode", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		editor.SetCompletionMode(batchv1.IndexedCompletion)
		require.NotNil(t, spec.CompletionMode)
		assert.Equal(t, batchv1.IndexedCompletion, *spec.CompletionMode)
	})

	t.Run("Raw returns pointer to spec", func(t *testing.T) {
		spec := &batchv1.JobSpec{}
		editor := NewJobSpecEditor(spec)
		assert.Same(t, spec, editor.Raw())
	})
}
