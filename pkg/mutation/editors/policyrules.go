package editors

import rbacv1 "k8s.io/api/rbac/v1"

// PolicyRulesEditor provides a typed API for mutating the .rules field of
// a Kubernetes Role or ClusterRole.
//
// It exposes structured operations (AddRule, RemoveRuleByIndex, Clear) as well
// as Raw() for free-form access when none of the structured methods are sufficient.
type PolicyRulesEditor struct {
	rules *[]rbacv1.PolicyRule
}

// NewPolicyRulesEditor creates a new PolicyRulesEditor wrapping the given rules
// slice pointer.
//
// The pointer may refer to a nil slice; methods that append rules initialise it
// automatically. Pass a non-nil pointer (e.g. &cr.Rules) so that writes are
// reflected on the object.
func NewPolicyRulesEditor(rules *[]rbacv1.PolicyRule) *PolicyRulesEditor {
	if rules == nil {
		var r []rbacv1.PolicyRule
		rules = &r
	}
	return &PolicyRulesEditor{rules: rules}
}

// Raw returns a pointer to the underlying []rbacv1.PolicyRule, initialising the
// slice if necessary.
//
// This is an escape hatch for free-form editing when none of the structured
// methods are sufficient.
func (e *PolicyRulesEditor) Raw() *[]rbacv1.PolicyRule {
	if *e.rules == nil {
		*e.rules = []rbacv1.PolicyRule{}
	}
	return e.rules
}

// AddRule appends a PolicyRule to the rules slice.
func (e *PolicyRulesEditor) AddRule(rule rbacv1.PolicyRule) {
	*e.rules = append(*e.rules, rule)
}

// RemoveRuleByIndex removes the rule at the given index.
//
// It is a no-op if the index is out of bounds.
func (e *PolicyRulesEditor) RemoveRuleByIndex(index int) {
	if index < 0 || index >= len(*e.rules) {
		return
	}
	*e.rules = append((*e.rules)[:index], (*e.rules)[index+1:]...)
}

// Clear removes all rules.
func (e *PolicyRulesEditor) Clear() {
	*e.rules = nil
}
