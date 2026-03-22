package role

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Mutation defines a mutation that is applied to a role Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	rulesEdits    []func(*editors.PolicyRulesEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes Role.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the Role in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	role *rbacv1.Role

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given Role.
func NewMutator(role *rbacv1.Role) *Mutator {
	m := &Mutator{role: role}
	m.beginFeature()
	return m
}

// beginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) beginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the Role's own metadata.
//
// Metadata edits are applied before rules edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditRules records a mutation for the Role's .rules field via a
// PolicyRulesEditor.
//
// The editor provides SetRules to replace atomically, AddRule to append, and
// Raw() for free-form access. Rules edits are applied after metadata edits
// within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditRules(edit func(*editors.PolicyRulesEditor) error) {
	if edit == nil {
		return
	}
	m.active.rulesEdits = append(m.active.rulesEdits, edit)
}

// Apply executes all recorded mutation intents on the underlying Role.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Rules edits (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the Role as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.role.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Rules edits
		if len(plan.rulesEdits) > 0 {
			editor := editors.NewPolicyRulesEditor(&m.role.Rules)
			for _, edit := range plan.rulesEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
