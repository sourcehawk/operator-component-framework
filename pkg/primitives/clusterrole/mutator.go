// Package clusterrole provides a builder and resource for managing Kubernetes ClusterRoles.
package clusterrole

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Mutation defines a mutation that is applied to a ClusterRole Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits       []func(*editors.ObjectMetaEditor) error
	rulesEdits          []func(*editors.PolicyRulesEditor) error
	aggregationRuleSets []*rbacv1.AggregationRule
}

// Mutator is a high-level helper for modifying a Kubernetes ClusterRole.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the ClusterRole in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered. Within each
// feature, edits are applied in category order: metadata, then rules, then
// aggregation rule.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	cr *rbacv1.ClusterRole

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given ClusterRole.
func NewMutator(cr *rbacv1.ClusterRole) *Mutator {
	m := &Mutator{
		cr:    cr,
		plans: []featurePlan{{}},
	}
	m.active = &m.plans[0]
	return m
}

// BeginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) BeginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the ClusterRole's own metadata.
//
// Metadata edits are applied before rules edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditRules records a mutation for the ClusterRole's .rules field via a
// PolicyRulesEditor.
//
// The editor provides structured operations (AddRule, RemoveRuleByIndex, Clear)
// as well as Raw() for free-form access. Rules edits are applied after metadata
// edits within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditRules(edit func(*editors.PolicyRulesEditor) error) {
	if edit == nil {
		return
	}
	m.active.rulesEdits = append(m.active.rulesEdits, edit)
}

// AddRule records that a PolicyRule should be appended to .rules.
//
// Convenience wrapper over EditRules.
func (m *Mutator) AddRule(rule rbacv1.PolicyRule) {
	m.EditRules(func(e *editors.PolicyRulesEditor) error {
		e.AddRule(rule)
		return nil
	})
}

// SetAggregationRule records that the ClusterRole's .aggregationRule should be
// set to the given value.
//
// An aggregation rule causes the API server to combine rules from ClusterRoles
// whose labels match the provided selectors, instead of using .rules directly.
// If called multiple times within the same feature, the last call wins.
//
// A nil value clears the aggregation rule.
func (m *Mutator) SetAggregationRule(rule *rbacv1.AggregationRule) {
	var copied *rbacv1.AggregationRule
	if rule != nil {
		copied = rule.DeepCopy()
	}
	m.active.aggregationRuleSets = append(m.active.aggregationRuleSets, copied)
}

// Apply executes all recorded mutation intents on the underlying ClusterRole.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Rules edits — EditRules, AddRule (in registration order within each feature)
//  3. Aggregation rule — SetAggregationRule (last call wins within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the ClusterRole as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.cr.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Rules edits
		if len(plan.rulesEdits) > 0 {
			editor := editors.NewPolicyRulesEditor(&m.cr.Rules)
			for _, edit := range plan.rulesEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 3. Aggregation rule (last call wins within this feature)
		if len(plan.aggregationRuleSets) > 0 {
			m.cr.AggregationRule = plan.aggregationRuleSets[len(plan.aggregationRuleSets)-1]
		}
	}

	return nil
}
