package pvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultFieldApplicator(t *testing.T) {
	storageClass := "fast-ssd"
	volumeMode := corev1.PersistentVolumeFilesystem

	tests := []struct {
		name    string
		current *corev1.PersistentVolumeClaim
		desired *corev1.PersistentVolumeClaim
		verify  func(t *testing.T, result *corev1.PersistentVolumeClaim)
	}{
		{
			name: "new PVC uses desired immutable fields",
			current: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-data",
					Namespace: "default",
				},
			},
			desired: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-data",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					StorageClassName: &storageClass,
					VolumeMode:       &volumeMode,
					VolumeName:       "pv-001",
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("10Gi"),
						},
					},
				},
			},
			verify: func(t *testing.T, result *corev1.PersistentVolumeClaim) {
				assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, result.Spec.AccessModes)
				assert.Equal(t, &storageClass, result.Spec.StorageClassName)
				assert.Equal(t, &volumeMode, result.Spec.VolumeMode)
				assert.Equal(t, "pv-001", result.Spec.VolumeName)
				assert.Equal(t, resource.MustParse("10Gi"), result.Spec.Resources.Requests[corev1.ResourceStorage])
			},
		},
		{
			name: "existing PVC preserves immutable fields from current",
			current: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "app-data",
					Namespace:       "default",
					ResourceVersion: "12345",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					StorageClassName: &storageClass,
					VolumeMode:       &volumeMode,
					VolumeName:       "pv-original",
				},
			},
			desired: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-data",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany},
					StorageClassName: func() *string { s := "slow-hdd"; return &s }(),
					VolumeMode:       func() *corev1.PersistentVolumeMode { m := corev1.PersistentVolumeBlock; return &m }(),
					VolumeName:       "pv-new",
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("20Gi"),
						},
					},
				},
			},
			verify: func(t *testing.T, result *corev1.PersistentVolumeClaim) {
				// Immutable fields should be preserved from current.
				assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}, result.Spec.AccessModes)
				assert.Equal(t, &storageClass, result.Spec.StorageClassName)
				assert.Equal(t, &volumeMode, result.Spec.VolumeMode)
				assert.Equal(t, "pv-original", result.Spec.VolumeName)
				// Mutable fields should come from desired.
				assert.Equal(t, resource.MustParse("20Gi"), result.Spec.Resources.Requests[corev1.ResourceStorage])
			},
		},
		{
			name: "existing PVC with empty ResourceVersion treated as new",
			current: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "app-data",
					Namespace:       "default",
					ResourceVersion: "",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeName:  "pv-original",
				},
			},
			desired: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-data",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany},
					VolumeName:  "pv-new",
				},
			},
			verify: func(t *testing.T, result *corev1.PersistentVolumeClaim) {
				// No ResourceVersion means new — desired fields should be used.
				assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}, result.Spec.AccessModes)
				assert.Equal(t, "pv-new", result.Spec.VolumeName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DefaultFieldApplicator(tt.current, tt.desired)
			require.NoError(t, err)
			tt.verify(t, tt.current)
		})
	}
}
