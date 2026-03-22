// Package rolebinding provides a builder and resource for managing Kubernetes RoleBindings.
package rolebinding

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Mutation defines a mutation that is applied to a rolebinding Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	subjectEdits  []func(*editors.BindingSubjectsEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes RoleBinding.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the RoleBinding in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	rb *rbacv1.RoleBinding

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given RoleBinding.
func NewMutator(rb *rbacv1.RoleBinding) *Mutator {
	m := &Mutator{rb: rb}
	m.beginFeature()
	return m
}

// beginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) beginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the RoleBinding's own metadata.
//
// Metadata edits are applied before subject edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditSubjects records a mutation for the RoleBinding's subjects list via a
// BindingSubjectsEditor.
//
// The editor provides structured operations (EnsureSubject, RemoveSubject) as
// well as Raw() for free-form access. Subject edits are applied after metadata
// edits within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditSubjects(edit func(*editors.BindingSubjectsEditor) error) {
	if edit == nil {
		return
	}
	m.active.subjectEdits = append(m.active.subjectEdits, edit)
}

// Apply executes all recorded mutation intents on the underlying RoleBinding.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Subject edits (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the RoleBinding as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.rb.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Subject edits
		if len(plan.subjectEdits) > 0 {
			editor := editors.NewBindingSubjectsEditor(&m.rb.Subjects)
			for _, edit := range plan.subjectEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
