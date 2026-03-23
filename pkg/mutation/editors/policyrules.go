package editors

import (
	rbacv1 "k8s.io/api/rbac/v1"
)

// PolicyRulesEditor provides a typed API for mutating the .rules field of a
// Kubernetes Role or ClusterRole.
//
// PolicyRule has no unique key, so there is no upsert or remove-by-key
// operation. Use SetRules to replace the entire slice atomically, AddRule to
// append a single rule, or Raw for arbitrary manipulation.
type PolicyRulesEditor struct {
	rules *[]rbacv1.PolicyRule
}

// NewPolicyRulesEditor creates a new PolicyRulesEditor wrapping the given rules
// slice pointer.
//
// NewPolicyRulesEditor panics if rules is nil. The slice it points to may be
// nil; methods that write to the slice initialise it automatically.
func NewPolicyRulesEditor(rules *[]rbacv1.PolicyRule) *PolicyRulesEditor {
	if rules == nil {
		panic("editors.NewPolicyRulesEditor: rules pointer must be non-nil")
	}
	return &PolicyRulesEditor{rules: rules}
}

// SetRules replaces the full rules slice with the provided rules.
func (e *PolicyRulesEditor) SetRules(rules []rbacv1.PolicyRule) {
	*e.rules = rules
}

// AddRule appends a single rule to the rules slice.
func (e *PolicyRulesEditor) AddRule(rule rbacv1.PolicyRule) {
	*e.rules = append(*e.rules, rule)
}

// Raw returns a pointer to the underlying rules slice for direct manipulation.
//
// This is an escape hatch for complex modifications when the structured methods
// are insufficient.
func (e *PolicyRulesEditor) Raw() *[]rbacv1.PolicyRule {
	return e.rules
}
