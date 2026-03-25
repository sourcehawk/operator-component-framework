package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

func TestStatefulSetSpecEditor(t *testing.T) {
	t.Run("SetReplicas", func(t *testing.T) {
		spec := &appsv1.StatefulSetSpec{}
		editor := NewStatefulSetSpecEditor(spec)
		editor.SetReplicas(3)
		assert.Equal(t, int32(3), *spec.Replicas)
	})

	t.Run("SetServiceName", func(t *testing.T) {
		spec := &appsv1.StatefulSetSpec{}
		editor := NewStatefulSetSpecEditor(spec)
		editor.SetServiceName("my-service")
		assert.Equal(t, "my-service", spec.ServiceName)
	})

	t.Run("SetPodManagementPolicy", func(t *testing.T) {
		spec := &appsv1.StatefulSetSpec{}
		editor := NewStatefulSetSpecEditor(spec)
		editor.SetPodManagementPolicy(appsv1.ParallelPodManagement)
		assert.Equal(t, appsv1.ParallelPodManagement, spec.PodManagementPolicy)
	})

	t.Run("SetUpdateStrategy", func(t *testing.T) {
		spec := &appsv1.StatefulSetSpec{}
		editor := NewStatefulSetSpecEditor(spec)
		strategy := appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.OnDeleteStatefulSetStrategyType,
		}
		editor.SetUpdateStrategy(strategy)
		assert.Equal(t, appsv1.OnDeleteStatefulSetStrategyType, spec.UpdateStrategy.Type)
	})

	t.Run("SetRevisionHistoryLimit", func(t *testing.T) {
		spec := &appsv1.StatefulSetSpec{}
		editor := NewStatefulSetSpecEditor(spec)
		editor.SetRevisionHistoryLimit(5)
		assert.Equal(t, int32(5), *spec.RevisionHistoryLimit)
	})

	t.Run("SetMinReadySeconds", func(t *testing.T) {
		spec := &appsv1.StatefulSetSpec{}
		editor := NewStatefulSetSpecEditor(spec)
		editor.SetMinReadySeconds(10)
		assert.Equal(t, int32(10), spec.MinReadySeconds)
	})

	t.Run("SetPersistentVolumeClaimRetentionPolicy", func(t *testing.T) {
		spec := &appsv1.StatefulSetSpec{}
		editor := NewStatefulSetSpecEditor(spec)
		policy := &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
			WhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
			WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		}
		editor.SetPersistentVolumeClaimRetentionPolicy(policy)
		assert.Equal(t, policy, spec.PersistentVolumeClaimRetentionPolicy)
	})

	t.Run("Raw", func(t *testing.T) {
		spec := &appsv1.StatefulSetSpec{}
		editor := NewStatefulSetSpecEditor(spec)
		assert.Equal(t, spec, editor.Raw())
	})
}
