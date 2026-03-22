// Package clusterrole provides a builder and resource for managing Kubernetes ClusterRoles.
package clusterrole

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Mutation defines a mutation that is applied to a clusterrole Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

// Mutator is a high-level helper for modifying a Kubernetes ClusterRole.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the ClusterRole in a single controlled pass when Apply() is called.
//
// All mutation intents are collected into flat lists and applied in a fixed
// category order: metadata edits, then rules edits, then aggregation rule.
// Within each category, edits are applied in registration order.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	cr *rbacv1.ClusterRole

	metadataEdits       []func(*editors.ObjectMetaEditor) error
	rulesEdits          []func(*editors.PolicyRulesEditor) error
	aggregationRuleSets []*rbacv1.AggregationRule
}

// NewMutator creates a new Mutator for the given ClusterRole.
func NewMutator(cr *rbacv1.ClusterRole) *Mutator {
	return &Mutator{cr: cr}
}

// EditObjectMetadata records a mutation for the ClusterRole's own metadata.
//
// Metadata edits are applied before rules edits.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.metadataEdits = append(m.metadataEdits, edit)
}

// EditRules records a mutation for the ClusterRole's .rules field via a
// PolicyRulesEditor.
//
// The editor provides structured operations (AddRule, RemoveRuleByIndex, Clear)
// as well as Raw() for free-form access. Rules edits are applied after metadata
// edits, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditRules(edit func(*editors.PolicyRulesEditor) error) {
	if edit == nil {
		return
	}
	m.rulesEdits = append(m.rulesEdits, edit)
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
// If called multiple times, the last call wins.
//
// A nil value clears the aggregation rule.
func (m *Mutator) SetAggregationRule(rule *rbacv1.AggregationRule) {
	m.aggregationRuleSets = append(m.aggregationRuleSets, rule)
}

// Apply executes all recorded mutation intents on the underlying ClusterRole.
//
// Execution order:
//
//  1. Metadata edits (in registration order)
//  2. Rules edits — EditRules, AddRule (in registration order)
//  3. Aggregation rule — SetAggregationRule (last call wins)
func (m *Mutator) Apply() error {
	// 1. Metadata edits
	if len(m.metadataEdits) > 0 {
		editor := editors.NewObjectMetaEditor(&m.cr.ObjectMeta)
		for _, edit := range m.metadataEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 2. Rules edits
	if len(m.rulesEdits) > 0 {
		editor := editors.NewPolicyRulesEditor(&m.cr.Rules)
		for _, edit := range m.rulesEdits {
			if err := edit(editor); err != nil {
				return err
			}
		}
	}

	// 3. Aggregation rule (last call wins)
	if len(m.aggregationRuleSets) > 0 {
		m.cr.AggregationRule = m.aggregationRuleSets[len(m.aggregationRuleSets)-1]
	}

	return nil
}
