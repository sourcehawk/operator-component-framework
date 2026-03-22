// Package secret provides a builder and resource for managing Kubernetes Secrets.
package secret

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	corev1 "k8s.io/api/core/v1"
)

// Mutation defines a mutation that is applied to a secret Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	dataEdits     []func(*editors.SecretDataEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes Secret.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the Secret in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	secret *corev1.Secret

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given Secret.
func NewMutator(s *corev1.Secret) *Mutator {
	m := &Mutator{secret: s}
	m.beginFeature()
	return m
}

// beginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) beginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the Secret's own metadata.
//
// Metadata edits are applied before data edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditData records a mutation for the Secret's .data and .stringData fields
// via a SecretDataEditor.
//
// The editor provides structured operations (Set, Remove, SetString,
// RemoveString) as well as Raw() and RawStringData() for free-form access.
// Data edits are applied after metadata edits within the same feature, in
// registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditData(edit func(*editors.SecretDataEditor) error) {
	if edit == nil {
		return
	}
	m.active.dataEdits = append(m.active.dataEdits, edit)
}

// SetData records that key in .data should be set to value.
//
// Convenience wrapper over EditData.
func (m *Mutator) SetData(key string, value []byte) {
	m.EditData(func(e *editors.SecretDataEditor) error {
		e.Set(key, value)
		return nil
	})
}

// RemoveData records that key should be deleted from .data.
//
// Convenience wrapper over EditData.
func (m *Mutator) RemoveData(key string) {
	m.EditData(func(e *editors.SecretDataEditor) error {
		e.Remove(key)
		return nil
	})
}

// SetStringData records that key in .stringData should be set to value.
//
// The API server merges .stringData into .data on write; use this when
// working with plaintext values that should not be pre-encoded.
//
// Convenience wrapper over EditData.
func (m *Mutator) SetStringData(key, value string) {
	m.EditData(func(e *editors.SecretDataEditor) error {
		e.SetString(key, value)
		return nil
	})
}

// RemoveStringData records that key should be deleted from .stringData.
//
// Convenience wrapper over EditData.
func (m *Mutator) RemoveStringData(key string) {
	m.EditData(func(e *editors.SecretDataEditor) error {
		e.RemoveString(key)
		return nil
	})
}

// Apply executes all recorded mutation intents on the underlying Secret.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Data edits — EditData, SetData, RemoveData, SetStringData, RemoveStringData
//     (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the Secret as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.secret.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Data edits
		if len(plan.dataEdits) > 0 {
			editor := editors.NewSecretDataEditor(&m.secret.Data, &m.secret.StringData)
			for _, edit := range plan.dataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
