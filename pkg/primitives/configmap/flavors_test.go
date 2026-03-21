package configmap

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPreserveCurrentLabels(t *testing.T) {
	t.Run("adds missing labels from current", func(t *testing.T) {
		applied := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"keep": "applied"}}}
		current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "applied", applied.Labels["keep"])
		assert.Equal(t, "current", applied.Labels["extra"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"key": "applied"}}}
		current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"key": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "applied", applied.Labels["key"])
	})

	t.Run("handles nil applied labels", func(t *testing.T) {
		applied := &corev1.ConfigMap{}
		current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "current", applied.Labels["extra"])
	})
}

func TestPreserveCurrentAnnotations(t *testing.T) {
	t.Run("adds missing annotations from current", func(t *testing.T) {
		applied := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"keep": "applied"}}}
		current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentAnnotations(applied, current, nil))
		assert.Equal(t, "applied", applied.Annotations["keep"])
		assert.Equal(t, "current", applied.Annotations["extra"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"key": "applied"}}}
		current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"key": "current"}}}

		require.NoError(t, PreserveCurrentAnnotations(applied, current, nil))
		assert.Equal(t, "applied", applied.Annotations["key"])
	})
}

func TestPreserveExternalEntries(t *testing.T) {
	t.Run("adds missing data keys from current", func(t *testing.T) {
		applied := &corev1.ConfigMap{Data: map[string]string{"owned": "applied"}}
		current := &corev1.ConfigMap{Data: map[string]string{"external": "current"}}

		require.NoError(t, PreserveExternalEntries(applied, current, nil))
		assert.Equal(t, "applied", applied.Data["owned"])
		assert.Equal(t, "current", applied.Data["external"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &corev1.ConfigMap{Data: map[string]string{"key": "applied"}}
		current := &corev1.ConfigMap{Data: map[string]string{"key": "current"}}

		require.NoError(t, PreserveExternalEntries(applied, current, nil))
		assert.Equal(t, "applied", applied.Data["key"])
	})

	t.Run("handles nil applied data", func(t *testing.T) {
		applied := &corev1.ConfigMap{}
		current := &corev1.ConfigMap{Data: map[string]string{"external": "current"}}

		require.NoError(t, PreserveExternalEntries(applied, current, nil))
		assert.Equal(t, "current", applied.Data["external"])
	})

	t.Run("no-op when current has no data", func(t *testing.T) {
		applied := &corev1.ConfigMap{Data: map[string]string{"owned": "applied"}}
		current := &corev1.ConfigMap{}

		require.NoError(t, PreserveExternalEntries(applied, current, nil))
		assert.Len(t, applied.Data, 1)
		assert.Equal(t, "applied", applied.Data["owned"])
	})
}

func TestFlavors_Integration(t *testing.T) {
	// Flavors run after baseline applicator via Mutate, in registration order.
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
			Labels:    map[string]string{"app": "desired"},
		},
		Data: map[string]string{"owned": "yes"},
	}

	t.Run("PreserveExternalEntries via Mutate", func(t *testing.T) {
		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(PreserveExternalEntries).
			Build()
		require.NoError(t, err)

		current := &corev1.ConfigMap{
			Data: map[string]string{"external": "keep", "owned": "old"},
		}
		require.NoError(t, res.Mutate(current))

		assert.Equal(t, "yes", current.Data["owned"])
		assert.Equal(t, "keep", current.Data["external"])
	})

	t.Run("flavors run in registration order", func(t *testing.T) {
		var order []string
		flavor1 := func(_, _, _ *corev1.ConfigMap) error {
			order = append(order, "flavor1")
			return nil
		}
		flavor2 := func(_, _, _ *corev1.ConfigMap) error {
			order = append(order, "flavor2")
			return nil
		}

		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(flavor1).
			WithFieldApplicationFlavor(flavor2).
			Build()
		require.NoError(t, err)

		require.NoError(t, res.Mutate(&corev1.ConfigMap{}))
		assert.Equal(t, []string{"flavor1", "flavor2"}, order)
	})

	t.Run("flavor error is returned", func(t *testing.T) {
		flavorErr := errors.New("flavor boom")
		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(func(_, _, _ *corev1.ConfigMap) error {
				return flavorErr
			}).
			Build()
		require.NoError(t, err)

		err = res.Mutate(&corev1.ConfigMap{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, flavorErr))
	})
}
