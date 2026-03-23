package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
)

// --- SetRules ---

func TestPolicyRulesEditor_SetRules(t *testing.T) {
	var rules []rbacv1.PolicyRule
	e := NewPolicyRulesEditor(&rules)
	e.SetRules([]rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	})
	assert.Len(t, rules, 1)
	assert.Equal(t, []string{"pods"}, rules[0].Resources)
}

func TestPolicyRulesEditor_SetRules_ReplacesExisting(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
		{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"}},
	}
	e := NewPolicyRulesEditor(&rules)
	e.SetRules([]rbacv1.PolicyRule{
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"create"}},
	})
	assert.Len(t, rules, 1)
	assert.Equal(t, []string{"deployments"}, rules[0].Resources)
}

func TestPolicyRulesEditor_SetRules_Empty(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	e := NewPolicyRulesEditor(&rules)
	e.SetRules([]rbacv1.PolicyRule{})
	assert.Empty(t, rules)
}

// --- AddRule ---

func TestPolicyRulesEditor_AddRule(t *testing.T) {
	var rules []rbacv1.PolicyRule
	e := NewPolicyRulesEditor(&rules)
	e.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"},
	})
	assert.Len(t, rules, 1)
	assert.Equal(t, []string{"pods"}, rules[0].Resources)
}

func TestPolicyRulesEditor_AddRule_Appends(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	e := NewPolicyRulesEditor(&rules)
	e.AddRule(rbacv1.PolicyRule{
		APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"}},
	)
	assert.Len(t, rules, 2)
	assert.Equal(t, []string{"pods"}, rules[0].Resources)
	assert.Equal(t, []string{"services"}, rules[1].Resources)
}

// --- Raw ---

func TestPolicyRulesEditor_Raw(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	e := NewPolicyRulesEditor(&rules)
	raw := e.Raw()
	*raw = append(*raw, rbacv1.PolicyRule{
		APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"create"}},
	)
	assert.Len(t, rules, 2)
}

func TestPolicyRulesEditor_Raw_ReturnsPointer(t *testing.T) {
	var rules []rbacv1.PolicyRule
	e := NewPolicyRulesEditor(&rules)
	raw := e.Raw()
	assert.Same(t, &rules, raw)
}

// --- Nil safety ---

func TestNewPolicyRulesEditor_NilPanics(t *testing.T) {
	assert.Panics(t, func() {
		NewPolicyRulesEditor(nil)
	})
}
