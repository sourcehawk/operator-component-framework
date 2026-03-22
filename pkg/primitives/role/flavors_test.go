package role

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPreserveCurrentLabels(t *testing.T) {
	t.Run("adds missing labels from current", func(t *testing.T) {
		applied := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"keep": "applied"}}}
		current := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "applied", applied.Labels["keep"])
		assert.Equal(t, "current", applied.Labels["extra"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"key": "applied"}}}
		current := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"key": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "applied", applied.Labels["key"])
	})

	t.Run("handles nil applied labels", func(t *testing.T) {
		applied := &rbacv1.Role{}
		current := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "current", applied.Labels["extra"])
	})
}

func TestPreserveCurrentAnnotations(t *testing.T) {
	t.Run("adds missing annotations from current", func(t *testing.T) {
		applied := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"keep": "applied"}}}
		current := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentAnnotations(applied, current, nil))
		assert.Equal(t, "applied", applied.Annotations["keep"])
		assert.Equal(t, "current", applied.Annotations["extra"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"key": "applied"}}}
		current := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"key": "current"}}}

		require.NoError(t, PreserveCurrentAnnotations(applied, current, nil))
		assert.Equal(t, "applied", applied.Annotations["key"])
	})
}

func TestFlavors_Integration(t *testing.T) {
	// Flavors run after baseline applicator via Mutate, in registration order.
	desired := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
			Labels:    map[string]string{"app": "desired"},
		},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
		},
	}

	t.Run("PreserveCurrentLabels via Mutate", func(t *testing.T) {
		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(PreserveCurrentLabels).
			Build()
		require.NoError(t, err)

		current := &rbacv1.Role{
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
		flavor1 := func(_, _, _ *rbacv1.Role) error {
			order = append(order, "flavor1")
			return nil
		}
		flavor2 := func(_, _, _ *rbacv1.Role) error {
			order = append(order, "flavor2")
			return nil
		}

		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(flavor1).
			WithFieldApplicationFlavor(flavor2).
			Build()
		require.NoError(t, err)

		require.NoError(t, res.Mutate(&rbacv1.Role{}))
		assert.Equal(t, []string{"flavor1", "flavor2"}, order)
	})

	t.Run("flavor error is returned", func(t *testing.T) {
		flavorErr := errors.New("flavor boom")
		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(func(_, _, _ *rbacv1.Role) error {
				return flavorErr
			}).
			Build()
		require.NoError(t, err)

		err = res.Mutate(&rbacv1.Role{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, flavorErr))
	})
}
