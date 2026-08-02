package role

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newValidRole() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "test-ns",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
		},
	}
}

func TestResource_Identity(t *testing.T) {
	res, err := NewBuilder(newValidRole()).Build()
	require.NoError(t, err)
	assert.Equal(t, "rbac.authorization.k8s.io/v1/Role/test-ns/test-role", res.Identity())
}

func TestResource_Object(t *testing.T) {
	role := newValidRole()
	res, err := NewBuilder(role).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)

	got, ok := obj.(*rbacv1.Role)
	require.True(t, ok)
	assert.Equal(t, role.Name, got.Name)
	assert.Equal(t, role.Namespace, got.Namespace)

	// Must be a deep copy.
	got.Name = "changed"
	assert.Equal(t, "test-role", role.Name)
}

func TestResource_Mutate(t *testing.T) {
	desired := newValidRole()
	res, err := NewBuilder(desired).Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*rbacv1.Role)
	require.Len(t, got.Rules, 1)
	assert.Equal(t, []string{"pods"}, got.Rules[0].Resources)
	assert.Equal(t, []string{"get", "list"}, got.Rules[0].Verbs)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidRole()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-rule",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditRules(func(e *editors.PolicyRulesEditor) error {
					e.AddRule(rbacv1.PolicyRule{
						APIGroups: []string{"apps"},
						Resources: []string{"deployments"},
						Verbs:     []string{"get"},
					})
					return nil
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*rbacv1.Role)
	require.Len(t, got.Rules, 2)
	assert.Equal(t, []string{"pods"}, got.Rules[0].Resources)
	assert.Equal(t, []string{"deployments"}, got.Rules[1].Resources)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidRole()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditRules(func(e *editors.PolicyRulesEditor) error {
					e.SetRules([]rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
					})
					return nil
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewVersionGate("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.EditRules(func(e *editors.PolicyRulesEditor) error {
					e.AddRule(rbacv1.PolicyRule{
						APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"list"},
					})
					return nil
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	obj, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, res.Mutate(obj))

	got := obj.(*rbacv1.Role)
	// feature-b appends to the rules set by feature-a.
	require.Len(t, got.Rules, 2)
	assert.Equal(t, []string{"secrets"}, got.Rules[0].Resources)
	assert.Equal(t, []string{"configmaps"}, got.Rules[1].Resources)
}
