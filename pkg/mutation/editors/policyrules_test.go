package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestPolicyRulesEditor_AddRule(t *testing.T) {
	var rules []rbacv1.PolicyRule
	e := NewPolicyRulesEditor(&rules)

	rule := rbacv1.PolicyRule{
		APIGroups: []string{""},
		Resources: []string{"pods"},
		Verbs:     []string{"get", "list"},
	}
	e.AddRule(rule)

	require.Len(t, rules, 1)
	assert.Equal(t, rule, rules[0])
}

func TestPolicyRulesEditor_AddRule_Appends(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	e := NewPolicyRulesEditor(&rules)

	e.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"list"},
	})

	assert.Len(t, rules, 2)
	assert.Equal(t, "secrets", rules[1].Resources[0])
}

func TestPolicyRulesEditor_RemoveRuleByIndex(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
		{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"list"}},
		{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"watch"}},
	}
	e := NewPolicyRulesEditor(&rules)

	e.RemoveRuleByIndex(1)

	require.Len(t, rules, 2)
	assert.Equal(t, "pods", rules[0].Resources[0])
	assert.Equal(t, "services", rules[1].Resources[0])
}

func TestPolicyRulesEditor_RemoveRuleByIndex_OutOfBounds(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	e := NewPolicyRulesEditor(&rules)

	e.RemoveRuleByIndex(-1)
	e.RemoveRuleByIndex(5)

	assert.Len(t, rules, 1)
}

func TestPolicyRulesEditor_Clear(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	e := NewPolicyRulesEditor(&rules)

	e.Clear()

	assert.Nil(t, rules)
}

func TestPolicyRulesEditor_Raw(t *testing.T) {
	var rules []rbacv1.PolicyRule
	e := NewPolicyRulesEditor(&rules)

	raw := e.Raw()
	require.NotNil(t, raw)
	assert.Empty(t, *raw)

	*raw = append(*raw, rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"},
	})

	assert.Len(t, rules, 1)
	assert.Equal(t, "configmaps", rules[0].Resources[0])
}

func TestNewPolicyRulesEditor_PanicsOnNilPointer(t *testing.T) {
	assert.PanicsWithValue(t, "NewPolicyRulesEditor: rules must be a non-nil pointer", func() {
		NewPolicyRulesEditor(nil)
	})
}

func TestPolicyRulesEditor_Raw_NilInitialized(t *testing.T) {
	var rules []rbacv1.PolicyRule
	e := NewPolicyRulesEditor(&rules)

	raw := e.Raw()
	assert.NotNil(t, *raw)
}
