package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPVCSpecEditor_Raw(t *testing.T) {
	spec := &corev1.PersistentVolumeClaimSpec{}
	editor := NewPVCSpecEditor(spec)
	assert.Same(t, spec, editor.Raw())
}

func TestPVCSpecEditor_SetStorageRequest(t *testing.T) {
	t.Run("nil requests map", func(t *testing.T) {
		spec := &corev1.PersistentVolumeClaimSpec{}
		editor := NewPVCSpecEditor(spec)
		qty := resource.MustParse("10Gi")
		editor.SetStorageRequest(qty)
		require.NotNil(t, spec.Resources.Requests)
		assert.True(t, spec.Resources.Requests[corev1.ResourceStorage].Equal(qty))
	})

	t.Run("existing requests map", func(t *testing.T) {
		spec := &corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("5Gi"),
				},
			},
		}
		editor := NewPVCSpecEditor(spec)
		qty := resource.MustParse("20Gi")
		editor.SetStorageRequest(qty)
		assert.True(t, spec.Resources.Requests[corev1.ResourceStorage].Equal(qty))
	})
}

func TestPVCSpecEditor_SetAccessModes(t *testing.T) {
	spec := &corev1.PersistentVolumeClaimSpec{}
	editor := NewPVCSpecEditor(spec)
	modes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany}
	editor.SetAccessModes(modes)
	assert.Equal(t, modes, spec.AccessModes)
}

func TestPVCSpecEditor_SetStorageClassName(t *testing.T) {
	spec := &corev1.PersistentVolumeClaimSpec{}
	editor := NewPVCSpecEditor(spec)
	editor.SetStorageClassName("fast-ssd")
	require.NotNil(t, spec.StorageClassName)
	assert.Equal(t, "fast-ssd", *spec.StorageClassName)
}

func TestPVCSpecEditor_SetVolumeMode(t *testing.T) {
	spec := &corev1.PersistentVolumeClaimSpec{}
	editor := NewPVCSpecEditor(spec)
	editor.SetVolumeMode(corev1.PersistentVolumeBlock)
	require.NotNil(t, spec.VolumeMode)
	assert.Equal(t, corev1.PersistentVolumeBlock, *spec.VolumeMode)
}

func TestPVCSpecEditor_SetVolumeName(t *testing.T) {
	spec := &corev1.PersistentVolumeClaimSpec{}
	editor := NewPVCSpecEditor(spec)
	editor.SetVolumeName("pv-001")
	assert.Equal(t, "pv-001", spec.VolumeName)
}
