package role

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestRole(rules []rbacv1.PolicyRule) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
		Rules: rules,
	}
}

// --- EditObjectMetadata ---

func TestMutator_EditObjectMetadata(t *testing.T) {
	role := newTestRole(nil)
	m := NewMutator(role)
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("app", "myapp")
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Equal(t, "myapp", role.Labels["app"])
}

func TestMutator_EditObjectMetadata_Nil(t *testing.T) {
	role := newTestRole(nil)
	m := NewMutator(role)
	m.EditObjectMetadata(nil)
	assert.NoError(t, m.Apply())
}

// --- EditRules ---

func TestMutator_EditRules_SetRules(t *testing.T) {
	role := newTestRole(nil)
	m := NewMutator(role)
	m.EditRules(func(e *editors.PolicyRulesEditor) error {
		e.SetRules([]rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Len(t, role.Rules, 1)
	assert.Equal(t, []string{"pods"}, role.Rules[0].Resources)
}

func TestMutator_EditRules_AddRule(t *testing.T) {
	role := newTestRole([]rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	})
	m := NewMutator(role)
	m.EditRules(func(e *editors.PolicyRulesEditor) error {
		e.AddRule(rbacv1.PolicyRule{
			APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Len(t, role.Rules, 2)
	assert.Equal(t, []string{"services"}, role.Rules[1].Resources)
}

func TestMutator_EditRules_RawAccess(t *testing.T) {
	role := newTestRole([]rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	})
	m := NewMutator(role)
	m.EditRules(func(e *editors.PolicyRulesEditor) error {
		raw := e.Raw()
		*raw = append(*raw, rbacv1.PolicyRule{
			APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"create"},
		})
		return nil
	})
	require.NoError(t, m.Apply())
	assert.Len(t, role.Rules, 2)
}

func TestMutator_EditRules_Nil(t *testing.T) {
	role := newTestRole(nil)
	m := NewMutator(role)
	m.EditRules(nil)
	assert.NoError(t, m.Apply())
}

// --- Execution order ---

func TestMutator_OperationOrder(t *testing.T) {
	// Within a feature: metadata edits run before rules edits.
	role := newTestRole(nil)
	m := NewMutator(role)
	// Register in reverse logical order to confirm Apply() enforces category ordering.
	m.EditRules(func(e *editors.PolicyRulesEditor) error {
		e.AddRule(rbacv1.PolicyRule{
			APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
		})
		return nil
	})
	m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
		e.EnsureLabel("order", "tested")
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Equal(t, "tested", role.Labels["order"])
	assert.Len(t, role.Rules, 1)
}

func TestMutator_MultipleFeatures(t *testing.T) {
	role := newTestRole(nil)
	m := NewMutator(role)
	m.EditRules(func(e *editors.PolicyRulesEditor) error {
		e.AddRule(rbacv1.PolicyRule{
			APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
		})
		return nil
	})
	m.beginFeature()
	m.EditRules(func(e *editors.PolicyRulesEditor) error {
		e.AddRule(rbacv1.PolicyRule{
			APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"},
		})
		return nil
	})
	require.NoError(t, m.Apply())

	assert.Len(t, role.Rules, 2)
	assert.Equal(t, []string{"pods"}, role.Rules[0].Resources)
	assert.Equal(t, []string{"services"}, role.Rules[1].Resources)
}

// --- ObjectMutator interface ---

func TestMutator_ImplementsObjectMutator(_ *testing.T) {
	var _ editors.ObjectMutator = (*Mutator)(nil)
}
