package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

func TestReplicaSetSpecEditor(t *testing.T) {
	t.Run("SetReplicas", func(t *testing.T) {
		spec := &appsv1.ReplicaSetSpec{}
		editor := NewReplicaSetSpecEditor(spec)
		editor.SetReplicas(3)
		assert.Equal(t, int32(3), *spec.Replicas)
	})

	t.Run("SetMinReadySeconds", func(t *testing.T) {
		spec := &appsv1.ReplicaSetSpec{}
		editor := NewReplicaSetSpecEditor(spec)
		editor.SetMinReadySeconds(10)
		assert.Equal(t, int32(10), spec.MinReadySeconds)
	})

	t.Run("Raw", func(t *testing.T) {
		spec := &appsv1.ReplicaSetSpec{}
		editor := NewReplicaSetSpecEditor(spec)
		assert.Equal(t, spec, editor.Raw())
	})
}
