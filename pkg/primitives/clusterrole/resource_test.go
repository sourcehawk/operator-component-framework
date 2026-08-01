package clusterrole

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidCR() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidCR()).Build()
	require.NoError(t, err)
	assert.Equal(t, "rbac.authorization.k8s.io/v1/ClusterRole/test-cr", res.Identity())
}

func TestResource_Object(t *testing.T) {
	cr := newValidCR()
	res, err := NewBuilder(cr).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*rbacv1.ClusterRole)
	require.True(t, ok)
	assert.Equal(t, cr.Name, got.Name)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-cr", cr.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidCR()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*rbacv1.ClusterRole)
	require.Len(t, got.Rules, 1)
	assert.Equal(t, []string{"get", "list"}, got.Rules[0].Verbs)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidCR()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-rule",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.AddRule(rbacv1.PolicyRule{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"get"},
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*rbacv1.ClusterRole)
	require.Len(t, got.Rules, 2)
	assert.Equal(t, []string{"get", "list"}, got.Rules[0].Verbs)
	assert.Equal(t, []string{"deployments"}, got.Rules[1].Resources)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidCR()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.AddRule(rbacv1.PolicyRule{
					APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"},
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.AddRule(rbacv1.PolicyRule{
					APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"},
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*rbacv1.ClusterRole)
	// Base rule + feature-a + feature-b = 3 rules in order.
	require.Len(t, got.Rules, 3)
	assert.Equal(t, []string{"pods"}, got.Rules[0].Resources)
	assert.Equal(t, []string{"secrets"}, got.Rules[1].Resources)
	assert.Equal(t, []string{"configmaps"}, got.Rules[2].Resources)
}
