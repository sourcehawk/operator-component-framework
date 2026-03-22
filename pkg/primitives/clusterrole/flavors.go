package clusterrole

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	rbacv1 "k8s.io/api/rbac/v1"
)

// FieldApplicationFlavor defines a function signature for applying flavors to a
// ClusterRole resource. A flavor is called after the baseline field applicator has
// run and can be used to preserve or merge fields from the live cluster object.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*rbacv1.ClusterRole]

// PreserveCurrentLabels ensures that any labels present on the current live
// ClusterRole but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *rbacv1.ClusterRole) error {
	return flavors.PreserveCurrentLabels[*rbacv1.ClusterRole]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live ClusterRole but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *rbacv1.ClusterRole) error {
	return flavors.PreserveCurrentAnnotations[*rbacv1.ClusterRole]()(applied, current, desired)
}

// PreserveExternalRules ensures that any .rules entries present on the current
// live ClusterRole but absent from the applied (desired) object are preserved
// by appending them to the applied object's rules.
//
// This is useful when other controllers or admission webhooks inject rules into
// the ClusterRole that your operator does not own. Rules present in both are
// determined by equality of the entire PolicyRule struct.
func PreserveExternalRules(applied, current, _ *rbacv1.ClusterRole) error {
	for _, currentRule := range current.Rules {
		if !containsRule(applied.Rules, currentRule) {
			applied.Rules = append(applied.Rules, currentRule)
		}
	}
	return nil
}

// containsRule reports whether rules contains a PolicyRule equal to target.
func containsRule(rules []rbacv1.PolicyRule, target rbacv1.PolicyRule) bool {
	for _, r := range rules {
		if policyRuleEqual(r, target) {
			return true
		}
	}
	return false
}

// policyRuleEqual reports whether two PolicyRules are equal by comparing all fields.
func policyRuleEqual(a, b rbacv1.PolicyRule) bool {
	return stringSliceEqual(a.Verbs, b.Verbs) &&
		stringSliceEqual(a.APIGroups, b.APIGroups) &&
		stringSliceEqual(a.Resources, b.Resources) &&
		stringSliceEqual(a.ResourceNames, b.ResourceNames) &&
		stringSliceEqual(a.NonResourceURLs, b.NonResourceURLs)
}

// stringSliceEqual reports whether two string slices are equal.
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
