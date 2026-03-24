package clusterrole

import (
	"errors"
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

	current := &rbacv1.ClusterRole{}
	require.NoError(t, res.Mutate(current))

	require.Len(t, current.Rules, 1)
	assert.Equal(t, []string{"get", "list"}, current.Rules[0].Verbs)
}

func TestResource_Mutate_WithMutation(t *testing.T) {
	desired := newValidCR()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "add-rule",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
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

	current := &rbacv1.ClusterRole{}
	require.NoError(t, res.Mutate(current))

	require.Len(t, current.Rules, 2)
	assert.Equal(t, []string{"get", "list"}, current.Rules[0].Verbs)
	assert.Equal(t, []string{"deployments"}, current.Rules[1].Resources)
}

func TestResource_Mutate_FeatureOrdering(t *testing.T) {
	desired := newValidCR()
	res, err := NewBuilder(desired).
		WithMutation(Mutation{
			Name:    "feature-a",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.AddRule(rbacv1.PolicyRule{
					APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"},
				})
				return nil
			},
		}).
		WithMutation(Mutation{
			Name:    "feature-b",
			Feature: feature.NewResourceFeature("v1", nil).When(true),
			Mutate: func(m *Mutator) error {
				m.AddRule(rbacv1.PolicyRule{
					APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"},
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)

	current := &rbacv1.ClusterRole{}
	require.NoError(t, res.Mutate(current))

	// Base rule + feature-a + feature-b = 3 rules in order.
	require.Len(t, current.Rules, 3)
	assert.Equal(t, []string{"pods"}, current.Rules[0].Resources)
	assert.Equal(t, []string{"secrets"}, current.Rules[1].Resources)
	assert.Equal(t, []string{"configmaps"}, current.Rules[2].Resources)
}

func TestResource_Mutate_CustomFieldApplicator(t *testing.T) {
	desired := newValidCR()

	applicatorCalled := false
	res, err := NewBuilder(desired).
		WithCustomFieldApplicator(func(current, d *rbacv1.ClusterRole) error {
			applicatorCalled = true
			current.Rules = d.Rules
			return nil
		}).
		Build()
	require.NoError(t, err)

	current := &rbacv1.ClusterRole{}
	require.NoError(t, res.Mutate(current))

	assert.True(t, applicatorCalled)
	require.Len(t, current.Rules, 1)
}

func TestResource_Mutate_CustomFieldApplicator_Error(t *testing.T) {
	res, err := NewBuilder(newValidCR()).
		WithCustomFieldApplicator(func(_, _ *rbacv1.ClusterRole) error {
			return errors.New("applicator error")
		}).
		Build()
	require.NoError(t, err)

	err = res.Mutate(&rbacv1.ClusterRole{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applicator error")
}

func TestResource_ExtractData(t *testing.T) {
	cr := newValidCR()

	var extracted string
	res, err := NewBuilder(cr).
		WithDataExtractor(func(c rbacv1.ClusterRole) error {
			extracted = c.Name
			return nil
		}).
		Build()
	require.NoError(t, err)

	require.NoError(t, res.ExtractData())
	assert.Equal(t, "test-cr", extracted)
}

func TestResource_ExtractData_Error(t *testing.T) {
	res, err := NewBuilder(newValidCR()).
		WithDataExtractor(func(_ rbacv1.ClusterRole) error {
			return errors.New("extract error")
		}).
		Build()
	require.NoError(t, err)

	err = res.ExtractData()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract error")
}

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	current := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			ResourceVersion: "12345",
			UID:             "abc-def",
			Generation:      3,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Pod", Name: "other-owner", UID: "other-uid"},
			},
			Finalizers: []string{"finalizer.example.com"},
		},
	}
	desired := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test",
			Labels: map[string]string{"app": "test"},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, "test", current.Labels["app"])
	require.Len(t, current.Rules, 1)
	assert.Equal(t, []string{"get", "list"}, current.Rules[0].Verbs)

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, "abc-def", string(current.UID))
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)
}
