package networkpolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPreserveCurrentLabels(t *testing.T) {
	t.Run("adds missing labels from current", func(t *testing.T) {
		applied := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"keep": "applied"}}}
		current := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "applied", applied.Labels["keep"])
		assert.Equal(t, "current", applied.Labels["extra"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"key": "applied"}}}
		current := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"key": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "applied", applied.Labels["key"])
	})

	t.Run("handles nil applied labels", func(t *testing.T) {
		applied := &networkingv1.NetworkPolicy{}
		current := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "current", applied.Labels["extra"])
	})
}

func TestPreserveCurrentAnnotations(t *testing.T) {
	t.Run("adds missing annotations from current", func(t *testing.T) {
		applied := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"keep": "applied"}}}
		current := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentAnnotations(applied, current, nil))
		assert.Equal(t, "applied", applied.Annotations["keep"])
		assert.Equal(t, "current", applied.Annotations["extra"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"key": "applied"}}}
		current := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"key": "current"}}}

		require.NoError(t, PreserveCurrentAnnotations(applied, current, nil))
		assert.Equal(t, "applied", applied.Annotations["key"])
	})
}

func TestFlavors_Integration(t *testing.T) {
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-np",
			Namespace: "default",
			Labels:    map[string]string{"app": "desired"},
		},
	}

	t.Run("PreserveCurrentLabels via Mutate", func(t *testing.T) {
		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(PreserveCurrentLabels).
			Build()
		require.NoError(t, err)

		current := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"external": "keep", "app": "old"},
			},
		}
		require.NoError(t, res.Mutate(current))

		assert.Equal(t, "desired", current.Labels["app"])
		assert.Equal(t, "keep", current.Labels["external"])
	})

	t.Run("flavors run in registration order", func(t *testing.T) {
		var order []string
		flavor1 := func(_, _, _ *networkingv1.NetworkPolicy) error {
			order = append(order, "flavor1")
			return nil
		}
		flavor2 := func(_, _, _ *networkingv1.NetworkPolicy) error {
			order = append(order, "flavor2")
			return nil
		}

		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(flavor1).
			WithFieldApplicationFlavor(flavor2).
			Build()
		require.NoError(t, err)

		require.NoError(t, res.Mutate(&networkingv1.NetworkPolicy{}))
		assert.Equal(t, []string{"flavor1", "flavor2"}, order)
	})

	t.Run("flavor error is returned", func(t *testing.T) {
		flavorErr := errors.New("flavor boom")
		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(func(_, _, _ *networkingv1.NetworkPolicy) error {
				return flavorErr
			}).
			Build()
		require.NoError(t, err)

		err = res.Mutate(&networkingv1.NetworkPolicy{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, flavorErr))
	})
}
