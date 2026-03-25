package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPVSpecEditor_Raw(t *testing.T) {
	spec := &corev1.PersistentVolumeSpec{}
	editor := NewPVSpecEditor(spec)
	assert.Same(t, spec, editor.Raw())
}

func TestPVSpecEditor_SetCapacity(t *testing.T) {
	t.Run("sets storage on nil capacity", func(t *testing.T) {
		spec := &corev1.PersistentVolumeSpec{}
		editor := NewPVSpecEditor(spec)
		qty := resource.MustParse("10Gi")
		editor.SetCapacity(qty)

		require.NotNil(t, spec.Capacity)
		assert.True(t, spec.Capacity[corev1.ResourceStorage].Equal(qty))
	})

	t.Run("overwrites existing capacity", func(t *testing.T) {
		spec := &corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("5Gi"),
			},
		}
		editor := NewPVSpecEditor(spec)
		qty := resource.MustParse("20Gi")
		editor.SetCapacity(qty)

		assert.True(t, spec.Capacity[corev1.ResourceStorage].Equal(qty))
	})
}

func TestPVSpecEditor_SetAccessModes(t *testing.T) {
	spec := &corev1.PersistentVolumeSpec{}
	editor := NewPVSpecEditor(spec)
	modes := []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
		corev1.ReadOnlyMany,
	}
	editor.SetAccessModes(modes)
	assert.Equal(t, modes, spec.AccessModes)
}

func TestPVSpecEditor_SetPersistentVolumeReclaimPolicy(t *testing.T) {
	spec := &corev1.PersistentVolumeSpec{}
	editor := NewPVSpecEditor(spec)
	editor.SetPersistentVolumeReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
	assert.Equal(t, corev1.PersistentVolumeReclaimRetain, spec.PersistentVolumeReclaimPolicy)
}

func TestPVSpecEditor_SetStorageClassName(t *testing.T) {
	spec := &corev1.PersistentVolumeSpec{}
	editor := NewPVSpecEditor(spec)
	editor.SetStorageClassName("fast-ssd")
	assert.Equal(t, "fast-ssd", spec.StorageClassName)
}

func TestPVSpecEditor_SetMountOptions(t *testing.T) {
	spec := &corev1.PersistentVolumeSpec{}
	editor := NewPVSpecEditor(spec)
	opts := []string{"hard", "nfsvers=4.1"}
	editor.SetMountOptions(opts)
	assert.Equal(t, opts, spec.MountOptions)
}

func TestPVSpecEditor_SetVolumeMode(t *testing.T) {
	spec := &corev1.PersistentVolumeSpec{}
	editor := NewPVSpecEditor(spec)
	mode := corev1.PersistentVolumeBlock
	editor.SetVolumeMode(mode)
	require.NotNil(t, spec.VolumeMode)
	assert.Equal(t, mode, *spec.VolumeMode)
}

func TestPVSpecEditor_SetNodeAffinity(t *testing.T) {
	spec := &corev1.PersistentVolumeSpec{}
	editor := NewPVSpecEditor(spec)
	affinity := &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "topology.kubernetes.io/zone",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"us-east-1a"},
						},
					},
				},
			},
		},
	}
	editor.SetNodeAffinity(affinity)
	assert.Equal(t, affinity, spec.NodeAffinity)
}
