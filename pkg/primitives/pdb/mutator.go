package pdb

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	policyv1 "k8s.io/api/policy/v1"
)

// ErrNoActiveFeature is returned when EditObjectMetadata or EditSpec is called
// before BeginFeature has started a feature scope.
var ErrNoActiveFeature = errors.New("pdb mutator: no active feature scope; call BeginFeature first")

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
// Unlike other primitive mutators, EditObjectMetadata and EditSpec return an
// error when a non-nil edit function is provided without an active feature
// scope (i.e. before BeginFeature is called). Nil edit functions are always
// ignored and return nil. This means Mutator does not satisfy editors.ObjectMutator.
type Mutator struct {
	pdb *policyv1.PodDisruptionBudget

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given PodDisruptionBudget.
//
// It is typically used within a Feature's Mutation logic to express desired
// changes to the PodDisruptionBudget. BeginFeature must be called before
// registering any mutations.
func NewMutator(pdb *policyv1.PodDisruptionBudget) *Mutator {
	return &Mutator{
		pdb: pdb,
	}
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
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) error {
	if edit == nil {
		return nil
	}
	if m.active == nil {
		return ErrNoActiveFeature
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
	return nil
}

// EditSpec records a mutation for the PodDisruptionBudget's spec via a
// PodDisruptionBudgetSpecEditor.
//
// The editor provides structured operations (SetMinAvailable, SetMaxUnavailable,
// SetSelector, etc.) as well as Raw() for free-form access. Spec edits are applied
// after metadata edits within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditSpec(edit func(*editors.PodDisruptionBudgetSpecEditor) error) error {
	if edit == nil {
		return nil
	}
	if m.active == nil {
		return ErrNoActiveFeature
	}
	m.active.specEdits = append(m.active.specEdits, edit)
	return nil
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
