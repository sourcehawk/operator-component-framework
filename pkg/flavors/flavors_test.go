package flavors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPreserveCurrentLabels(t *testing.T) {
	t.Run("preserves current-only keys", func(t *testing.T) {
		applied := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"keep": "applied"}}}
		current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		err := PreserveCurrentLabels[*corev1.ConfigMap]()(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "applied", applied.Labels["keep"])
		assert.Equal(t, "current", applied.Labels["extra"])
	})

	t.Run("does not overwrite applied keys", func(t *testing.T) {
		applied := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"overlap": "applied"}}}
		current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"overlap": "current"}}}

		err := PreserveCurrentLabels[*corev1.ConfigMap]()(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "applied", applied.Labels["overlap"])
	})

	t.Run("handles nil maps", func(t *testing.T) {
		applied := &corev1.ConfigMap{}
		current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		err := PreserveCurrentLabels[*corev1.ConfigMap]()(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "current", applied.Labels["extra"])

		appliedEmpty := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"keep": "applied"}}}
		currentNil := &corev1.ConfigMap{}
		err = PreserveCurrentLabels[*corev1.ConfigMap]()(appliedEmpty, currentNil, nil)
		require.NoError(t, err)
		assert.Equal(t, "applied", appliedEmpty.Labels["keep"])
		assert.Len(t, appliedEmpty.Labels, 1)
	})
}

func TestPreserveCurrentAnnotations(t *testing.T) {
	t.Run("preserves current-only keys", func(t *testing.T) {
		applied := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"keep": "applied"}}}
		current := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"extra": "current"}}}

		err := PreserveCurrentAnnotations[*appsv1.Deployment]()(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "applied", applied.Annotations["keep"])
		assert.Equal(t, "current", applied.Annotations["extra"])
	})

	t.Run("does not overwrite applied keys", func(t *testing.T) {
		applied := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"overlap": "applied"}}}
		current := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"overlap": "current"}}}

		err := PreserveCurrentAnnotations[*appsv1.Deployment]()(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "applied", applied.Annotations["overlap"])
	})

	t.Run("handles nil maps", func(t *testing.T) {
		applied := &appsv1.Deployment{}
		current := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"extra": "current"}}}

		err := PreserveCurrentAnnotations[*appsv1.Deployment]()(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "current", applied.Annotations["extra"])
	})
}
