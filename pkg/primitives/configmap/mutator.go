// Package configmap provides a builder and resource for managing Kubernetes ConfigMaps.
package configmap

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	corev1 "k8s.io/api/core/v1"
)

// Mutation defines a mutation that is applied to a configmap Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	dataEdits     []func(*editors.ConfigMapDataEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes ConfigMap.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the ConfigMap in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	cm *corev1.ConfigMap

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given ConfigMap.
func NewMutator(cm *corev1.ConfigMap) *Mutator {
	m := &Mutator{
		cm:    cm,
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

// EditObjectMetadata records a mutation for the ConfigMap's own metadata.
//
// Metadata edits are applied before data edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditData records a mutation for the ConfigMap's .data field via a
// ConfigMapDataEditor.
//
// The editor provides structured operations (Set, Remove, MergeYAML) as well as
// Raw() for free-form access. Data edits are applied after metadata edits within
// the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditData(edit func(*editors.ConfigMapDataEditor) error) {
	if edit == nil {
		return
	}
	m.active.dataEdits = append(m.active.dataEdits, edit)
}

// SetEntry records that key in .data should be set to value.
//
// Convenience wrapper over EditData.
func (m *Mutator) SetEntry(key, value string) {
	m.EditData(func(e *editors.ConfigMapDataEditor) error {
		e.Set(key, value)
		return nil
	})
}

// RemoveEntry records that key should be deleted from .data.
//
// Convenience wrapper over EditData.
func (m *Mutator) RemoveEntry(key string) {
	m.EditData(func(e *editors.ConfigMapDataEditor) error {
		e.Remove(key)
		return nil
	})
}

// MergeYAML records a deep-merge of yamlPatch into the existing value at key
// in .data.
//
// Convenience wrapper over EditData. See ConfigMapDataEditor.MergeYAML for merge
// semantics and error conditions.
func (m *Mutator) MergeYAML(key, yamlPatch string) {
	m.EditData(func(e *editors.ConfigMapDataEditor) error {
		return e.MergeYAML(key, yamlPatch)
	})
}

// Apply executes all recorded mutation intents on the underlying ConfigMap.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Data edits — EditData, SetEntry, RemoveEntry, MergeYAML (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the ConfigMap as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.cm.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Data edits
		if len(plan.dataEdits) > 0 {
			editor := editors.NewConfigMapDataEditor(&m.cm.Data, &m.cm.BinaryData)
			for _, edit := range plan.dataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
