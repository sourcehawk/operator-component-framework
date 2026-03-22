package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

func TestDaemonSetSpecEditor(t *testing.T) {
	t.Run("SetUpdateStrategy", func(t *testing.T) {
		spec := &appsv1.DaemonSetSpec{}
		editor := NewDaemonSetSpecEditor(spec)
		strategy := appsv1.DaemonSetUpdateStrategy{
			Type: appsv1.RollingUpdateDaemonSetStrategyType,
		}
		editor.SetUpdateStrategy(strategy)
		assert.Equal(t, appsv1.RollingUpdateDaemonSetStrategyType, spec.UpdateStrategy.Type)
	})

	t.Run("SetMinReadySeconds", func(t *testing.T) {
		spec := &appsv1.DaemonSetSpec{}
		editor := NewDaemonSetSpecEditor(spec)
		editor.SetMinReadySeconds(30)
		assert.Equal(t, int32(30), spec.MinReadySeconds)
	})

	t.Run("SetRevisionHistoryLimit", func(t *testing.T) {
		spec := &appsv1.DaemonSetSpec{}
		editor := NewDaemonSetSpecEditor(spec)
		editor.SetRevisionHistoryLimit(5)
		assert.Equal(t, int32(5), *spec.RevisionHistoryLimit)
	})

	t.Run("Raw", func(t *testing.T) {
		spec := &appsv1.DaemonSetSpec{}
		editor := NewDaemonSetSpecEditor(spec)
		assert.Equal(t, spec, editor.Raw())
	})
}
