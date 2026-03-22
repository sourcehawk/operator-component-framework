package pvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPreserveCurrentLabels(t *testing.T) {
	t.Parallel()

	t.Run("preserves missing labels", func(t *testing.T) {
		applied := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "myapp"},
			},
		}
		current := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "myapp", "external": "value"},
			},
		}
		desired := &corev1.PersistentVolumeClaim{}

		err := PreserveCurrentLabels(applied, current, desired)
		require.NoError(t, err)
		assert.Equal(t, "myapp", applied.Labels["app"])
		assert.Equal(t, "value", applied.Labels["external"])
	})

	t.Run("applied wins on overlap", func(t *testing.T) {
		applied := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "new"},
			},
		}
		current := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "old"},
			},
		}
		desired := &corev1.PersistentVolumeClaim{}

		err := PreserveCurrentLabels(applied, current, desired)
		require.NoError(t, err)
		assert.Equal(t, "new", applied.Labels["app"])
	})

	t.Run("nil labels on current", func(t *testing.T) {
		applied := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "myapp"},
			},
		}
		current := &corev1.PersistentVolumeClaim{}
		desired := &corev1.PersistentVolumeClaim{}

		err := PreserveCurrentLabels(applied, current, desired)
		require.NoError(t, err)
		assert.Equal(t, "myapp", applied.Labels["app"])
	})
}

func TestPreserveCurrentAnnotations(t *testing.T) {
	t.Parallel()

	t.Run("preserves missing annotations", func(t *testing.T) {
		applied := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"managed": "true"},
			},
		}
		current := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"managed": "true", "external": "injected"},
			},
		}
		desired := &corev1.PersistentVolumeClaim{}

		err := PreserveCurrentAnnotations(applied, current, desired)
		require.NoError(t, err)
		assert.Equal(t, "true", applied.Annotations["managed"])
		assert.Equal(t, "injected", applied.Annotations["external"])
	})

	t.Run("nil annotations on current", func(t *testing.T) {
		applied := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"managed": "true"},
			},
		}
		current := &corev1.PersistentVolumeClaim{}
		desired := &corev1.PersistentVolumeClaim{}

		err := PreserveCurrentAnnotations(applied, current, desired)
		require.NoError(t, err)
		assert.Equal(t, "true", applied.Annotations["managed"])
	})
}

func TestFlavorsViaBuilder(t *testing.T) {
	t.Parallel()

	base := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			Labels: map[string]string{
				"app": "myapp",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("5Gi"),
				},
			},
		},
	}

	res, err := NewBuilder(base).
		WithFieldApplicationFlavor(PreserveCurrentLabels).
		WithFieldApplicationFlavor(PreserveCurrentAnnotations).
		Build()
	require.NoError(t, err)

	current := base.DeepCopy()
	current.Labels["external-label"] = "preserved"
	current.Annotations = map[string]string{"external-annotation": "preserved"}

	err = res.Mutate(current)
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	pvc := obj.(*corev1.PersistentVolumeClaim)
	assert.Equal(t, "preserved", pvc.Labels["external-label"])
	assert.Equal(t, "preserved", pvc.Annotations["external-annotation"])
}
