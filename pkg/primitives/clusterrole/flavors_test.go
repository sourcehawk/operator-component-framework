package clusterrole

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
		applied := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"keep": "applied"}}}
		current := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "applied", applied.Labels["keep"])
		assert.Equal(t, "current", applied.Labels["extra"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"key": "applied"}}}
		current := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"key": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "applied", applied.Labels["key"])
	})

	t.Run("handles nil applied labels", func(t *testing.T) {
		applied := &rbacv1.ClusterRole{}
		current := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentLabels(applied, current, nil))
		assert.Equal(t, "current", applied.Labels["extra"])
	})
}

func TestPreserveCurrentAnnotations(t *testing.T) {
	t.Run("adds missing annotations from current", func(t *testing.T) {
		applied := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"keep": "applied"}}}
		current := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"extra": "current"}}}

		require.NoError(t, PreserveCurrentAnnotations(applied, current, nil))
		assert.Equal(t, "applied", applied.Annotations["keep"])
		assert.Equal(t, "current", applied.Annotations["extra"])
	})

	t.Run("applied value wins on overlap", func(t *testing.T) {
		applied := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"key": "applied"}}}
		current := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"key": "current"}}}

		require.NoError(t, PreserveCurrentAnnotations(applied, current, nil))
		assert.Equal(t, "applied", applied.Annotations["key"])
	})
}

func TestPreserveExternalRules(t *testing.T) {
	t.Run("adds missing rules from current", func(t *testing.T) {
		applied := &rbacv1.ClusterRole{
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
		}
		current := &rbacv1.ClusterRole{
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"list"}},
			},
		}

		require.NoError(t, PreserveExternalRules(applied, current, nil))
		assert.Len(t, applied.Rules, 2)
		assert.Equal(t, "pods", applied.Rules[0].Resources[0])
		assert.Equal(t, "secrets", applied.Rules[1].Resources[0])
	})

	t.Run("does not duplicate existing rules", func(t *testing.T) {
		rule := rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
		applied := &rbacv1.ClusterRole{Rules: []rbacv1.PolicyRule{rule}}
		current := &rbacv1.ClusterRole{Rules: []rbacv1.PolicyRule{rule}}

		require.NoError(t, PreserveExternalRules(applied, current, nil))
		assert.Len(t, applied.Rules, 1)
	})

	t.Run("handles nil applied rules", func(t *testing.T) {
		applied := &rbacv1.ClusterRole{}
		current := &rbacv1.ClusterRole{
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
			},
		}

		require.NoError(t, PreserveExternalRules(applied, current, nil))
		assert.Len(t, applied.Rules, 1)
	})

	t.Run("treats reordered-but-equivalent rules as equal", func(t *testing.T) {
		applied := &rbacv1.ClusterRole{
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods", "services"}, Verbs: []string{"get", "list"}},
			},
		}
		current := &rbacv1.ClusterRole{
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"services", "pods"}, Verbs: []string{"list", "get"}},
			},
		}

		require.NoError(t, PreserveExternalRules(applied, current, nil))
		assert.Len(t, applied.Rules, 1, "reordered rule should not be duplicated")
	})

	t.Run("no-op when current has no rules", func(t *testing.T) {
		applied := &rbacv1.ClusterRole{
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
		}
		current := &rbacv1.ClusterRole{}

		require.NoError(t, PreserveExternalRules(applied, current, nil))
		assert.Len(t, applied.Rules, 1)
	})
}

func TestFlavors_Integration(t *testing.T) {
	desired := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-cr",
			Labels: map[string]string{"app": "desired"},
		},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
		},
	}

	t.Run("PreserveExternalRules via Mutate", func(t *testing.T) {
		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(PreserveExternalRules).
			Build()
		require.NoError(t, err)

		current := &rbacv1.ClusterRole{
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"list"}},
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
		}
		require.NoError(t, res.Mutate(current))

		assert.Len(t, current.Rules, 2)
	})

	t.Run("flavors run in registration order", func(t *testing.T) {
		var order []string
		flavor1 := func(_, _, _ *rbacv1.ClusterRole) error {
			order = append(order, "flavor1")
			return nil
		}
		flavor2 := func(_, _, _ *rbacv1.ClusterRole) error {
			order = append(order, "flavor2")
			return nil
		}

		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(flavor1).
			WithFieldApplicationFlavor(flavor2).
			Build()
		require.NoError(t, err)

		require.NoError(t, res.Mutate(&rbacv1.ClusterRole{}))
		assert.Equal(t, []string{"flavor1", "flavor2"}, order)
	})

	t.Run("flavor error is returned", func(t *testing.T) {
		flavorErr := errors.New("flavor boom")
		res, err := NewBuilder(desired).
			WithFieldApplicationFlavor(func(_, _, _ *rbacv1.ClusterRole) error {
				return flavorErr
			}).
			Build()
		require.NoError(t, err)

		err = res.Mutate(&rbacv1.ClusterRole{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, flavorErr))
	})
}
