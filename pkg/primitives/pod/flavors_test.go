package pod

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMutate_OrderingAndFlavors(t *testing.T) {
	desired := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "desired"},
		},
	}

	t.Run("flavors run after baseline applicator", func(t *testing.T) {
		current := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "test-ns",
				Labels:    map[string]string{"extra": "preserved"},
			},
		}

		res, _ := NewBuilder(desired).
			WithFieldApplicationFlavor(PreserveCurrentLabels).
			Build()

		err := res.Mutate(current)
		require.NoError(t, err)

		assert.Equal(t, "desired", current.Labels["app"])
		assert.Equal(t, "preserved", current.Labels["extra"])
	})

	t.Run("flavors run in registration order", func(t *testing.T) {
		current := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "test-ns",
			},
		}

		var order []string
		flavor1 := func(_, _, _ *corev1.Pod) error {
			order = append(order, "flavor1")
			return nil
		}
		flavor2 := func(_, _, _ *corev1.Pod) error {
			order = append(order, "flavor2")
			return nil
		}

		res, _ := NewBuilder(desired).
			WithFieldApplicationFlavor(flavor1).
			WithFieldApplicationFlavor(flavor2).
			Build()

		err := res.Mutate(current)
		require.NoError(t, err)
		assert.Equal(t, []string{"flavor1", "flavor2"}, order)
	})

	t.Run("flavor error is returned with context", func(t *testing.T) {
		current := &corev1.Pod{}
		flavorErr := errors.New("boom")
		flavor := func(_, _, _ *corev1.Pod) error {
			return flavorErr
		}

		res, _ := NewBuilder(desired).
			WithFieldApplicationFlavor(flavor).
			Build()

		err := res.Mutate(current)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to apply field application flavor")
		assert.True(t, errors.Is(err, flavorErr))
	})
}

func TestDefaultFlavors(t *testing.T) {
	t.Run("PreserveCurrentLabels", func(t *testing.T) {
		applied := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"keep": "applied", "overlap": "applied"}}}
		current := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current", "overlap": "current"}}}

		err := PreserveCurrentLabels(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "applied", applied.Labels["keep"])
		assert.Equal(t, "applied", applied.Labels["overlap"])
		assert.Equal(t, "current", applied.Labels["extra"])
	})

	t.Run("PreserveCurrentAnnotations", func(t *testing.T) {
		applied := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"keep": "applied"}}}
		current := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"extra": "current"}}}

		err := PreserveCurrentAnnotations(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "applied", applied.Annotations["keep"])
		assert.Equal(t, "current", applied.Annotations["extra"])
	})

	t.Run("handles nil maps safely", func(t *testing.T) {
		applied := &corev1.Pod{}
		current := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		err := PreserveCurrentLabels(applied, current, nil)
		require.NoError(t, err)
		assert.Equal(t, "current", applied.Labels["extra"])
	})
}
