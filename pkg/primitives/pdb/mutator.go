package pdb

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	policyv1 "k8s.io/api/policy/v1"
)

// Mutation defines a mutation that is applied to a pdb Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	specEdits     []func(*editors.PodDisruptionBudgetSpecEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes PodDisruptionBudget.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the PodDisruptionBudget in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	pdb *policyv1.PodDisruptionBudget

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given PodDisruptionBudget.
func NewMutator(pdb *policyv1.PodDisruptionBudget) *Mutator {
	m := &Mutator{
		pdb:   pdb,
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

// EditObjectMetadata records a mutation for the PodDisruptionBudget's own metadata.
//
// Metadata edits are applied before spec edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditSpec records a mutation for the PodDisruptionBudget's spec via a
// PodDisruptionBudgetSpecEditor.
//
// The editor provides structured operations (SetMinAvailable, SetMaxUnavailable,
// SetSelector, etc.) as well as Raw() for free-form access. Spec edits are applied
// after metadata edits within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditSpec(edit func(*editors.PodDisruptionBudgetSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.specEdits = append(m.active.specEdits, edit)
}

// Apply executes all recorded mutation intents on the underlying PodDisruptionBudget.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Spec edits — EditSpec (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the PodDisruptionBudget as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.pdb.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Spec edits
		if len(plan.specEdits) > 0 {
			editor := editors.NewPodDisruptionBudgetSpecEditor(&m.pdb.Spec)
			for _, edit := range plan.specEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
