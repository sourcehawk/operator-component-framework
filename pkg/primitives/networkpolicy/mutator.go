// Package networkpolicy provides a builder and resource for managing Kubernetes NetworkPolicies.
package networkpolicy

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	networkingv1 "k8s.io/api/networking/v1"
)

// Mutation defines a mutation that is applied to a networkpolicy Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	specEdits     []func(*editors.NetworkPolicySpecEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes NetworkPolicy.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the NetworkPolicy in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Within a single feature plan, edits are applied in category order: metadata
// edits first, then spec edits. All edits within a category run in registration
// order.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	np *networkingv1.NetworkPolicy

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given NetworkPolicy.
func NewMutator(np *networkingv1.NetworkPolicy) *Mutator {
	m := &Mutator{
		np:    np,
		plans: []featurePlan{{}},
	}
	m.active = &m.plans[0]
	return m
}

// beginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) beginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the NetworkPolicy's own metadata.
//
// Metadata edits are applied before spec edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditNetworkPolicySpec records a mutation for the NetworkPolicy's spec via a
// NetworkPolicySpecEditor.
//
// The editor provides structured operations (SetPodSelector, EnsureIngressRule,
// RemoveIngressRules, EnsureEgressRule, RemoveEgressRules, SetPolicyTypes) as
// well as Raw() for free-form access. Spec edits are applied after metadata
// edits within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditNetworkPolicySpec(edit func(*editors.NetworkPolicySpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.specEdits = append(m.active.specEdits, edit)
}

// Apply executes all recorded mutation intents on the underlying NetworkPolicy.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Spec edits — EditNetworkPolicySpec (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the NetworkPolicy as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.np.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Spec edits
		if len(plan.specEdits) > 0 {
			editor := editors.NewNetworkPolicySpecEditor(&m.np.Spec)
			for _, edit := range plan.specEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
