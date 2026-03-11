package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

func TestDeploymentSpecEditor(t *testing.T) {
	t.Run("SetReplicas", func(t *testing.T) {
		spec := &appsv1.DeploymentSpec{}
		editor := NewDeploymentSpecEditor(spec)
		editor.SetReplicas(3)
		assert.Equal(t, int32(3), *spec.Replicas)
	})

	t.Run("SetPaused", func(t *testing.T) {
		spec := &appsv1.DeploymentSpec{}
		editor := NewDeploymentSpecEditor(spec)
		editor.SetPaused(true)
		assert.True(t, spec.Paused)
	})

	t.Run("SetMinReadySeconds", func(t *testing.T) {
		spec := &appsv1.DeploymentSpec{}
		editor := NewDeploymentSpecEditor(spec)
		editor.SetMinReadySeconds(10)
		assert.Equal(t, int32(10), spec.MinReadySeconds)
	})

	t.Run("SetRevisionHistoryLimit", func(t *testing.T) {
		spec := &appsv1.DeploymentSpec{}
		editor := NewDeploymentSpecEditor(spec)
		editor.SetRevisionHistoryLimit(5)
		assert.Equal(t, int32(5), *spec.RevisionHistoryLimit)
	})

	t.Run("SetProgressDeadlineSeconds", func(t *testing.T) {
		spec := &appsv1.DeploymentSpec{}
		editor := NewDeploymentSpecEditor(spec)
		editor.SetProgressDeadlineSeconds(600)
		assert.Equal(t, int32(600), *spec.ProgressDeadlineSeconds)
	})

	t.Run("Raw", func(t *testing.T) {
		spec := &appsv1.DeploymentSpec{}
		editor := NewDeploymentSpecEditor(spec)
		assert.Equal(t, spec, editor.Raw())
	})
}
