// Package pvc provides a builder and resource for managing Kubernetes PersistentVolumeClaims.
package pvc

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Mutation defines a mutation that is applied to a pvc Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	specEdits     []func(*editors.PVCSpecEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes PersistentVolumeClaim.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the PVC in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	pvc *corev1.PersistentVolumeClaim

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given PersistentVolumeClaim.
func NewMutator(pvc *corev1.PersistentVolumeClaim) *Mutator {
	m := &Mutator{pvc: pvc}
	m.beginFeature()
	return m
}

// beginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) beginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the PVC's own metadata.
//
// Metadata edits are applied before spec edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditPVCSpec records a mutation for the PVC's spec via a PVCSpecEditor.
//
// The editor provides structured operations (SetStorageRequest, SetAccessModes, etc.)
// as well as Raw() for free-form access. Spec edits are applied after metadata edits
// within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditPVCSpec(edit func(*editors.PVCSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.specEdits = append(m.active.specEdits, edit)
}

// SetStorageRequest records that the PVC's storage request should be set to quantity.
//
// Convenience wrapper over EditPVCSpec.
func (m *Mutator) SetStorageRequest(quantity resource.Quantity) {
	m.EditPVCSpec(func(e *editors.PVCSpecEditor) error {
		e.SetStorageRequest(quantity)
		return nil
	})
}

// Apply executes all recorded mutation intents on the underlying PVC.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Spec edits — EditPVCSpec, SetStorageRequest (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the PVC as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.pvc.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Spec edits
		if len(plan.specEdits) > 0 {
			editor := editors.NewPVCSpecEditor(&m.pvc.Spec)
			for _, edit := range plan.specEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
