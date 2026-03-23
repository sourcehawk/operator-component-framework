package pv

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	corev1 "k8s.io/api/core/v1"
)

// Mutation defines a mutation that is applied to a pv Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	specEdits     []func(*editors.PVSpecEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes PersistentVolume.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the PersistentVolume in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	pv *corev1.PersistentVolume

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given PersistentVolume.
func NewMutator(pv *corev1.PersistentVolume) *Mutator {
	m := &Mutator{
		pv:    pv,
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

// EditObjectMetadata records a mutation for the PersistentVolume's own metadata.
//
// Metadata edits are applied before spec edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditPVSpec records a mutation for the PersistentVolume's spec via a PVSpecEditor.
//
// The editor provides structured operations (SetCapacity, SetAccessModes,
// SetPersistentVolumeReclaimPolicy, etc.) as well as Raw() for free-form access.
// Spec edits are applied after metadata edits within the same feature, in
// registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditPVSpec(edit func(*editors.PVSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.specEdits = append(m.active.specEdits, edit)
}

// SetStorageClassName records that the PV's storage class should be set to the given name.
//
// Convenience wrapper over EditPVSpec.
func (m *Mutator) SetStorageClassName(name string) {
	m.EditPVSpec(func(e *editors.PVSpecEditor) error {
		e.SetStorageClassName(name)
		return nil
	})
}

// SetReclaimPolicy records that the PV's reclaim policy should be set to the given value.
//
// Convenience wrapper over EditPVSpec.
func (m *Mutator) SetReclaimPolicy(policy corev1.PersistentVolumeReclaimPolicy) {
	m.EditPVSpec(func(e *editors.PVSpecEditor) error {
		e.SetPersistentVolumeReclaimPolicy(policy)
		return nil
	})
}

// SetMountOptions records that the PV's mount options should be set to the given values.
//
// Convenience wrapper over EditPVSpec.
func (m *Mutator) SetMountOptions(options []string) {
	m.EditPVSpec(func(e *editors.PVSpecEditor) error {
		e.SetMountOptions(options)
		return nil
	})
}

// Apply executes all recorded mutation intents on the underlying PersistentVolume.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Spec edits — EditPVSpec, SetStorageClassName, SetReclaimPolicy, SetMountOptions
//     (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the PersistentVolume as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.pv.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Spec edits
		if len(plan.specEdits) > 0 {
			editor := editors.NewPVSpecEditor(&m.pv.Spec)
			for _, edit := range plan.specEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
