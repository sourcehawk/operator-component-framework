// Package hpa provides a builder and resource for managing Kubernetes HorizontalPodAutoscalers.
package hpa

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
)

// Mutation defines a mutation that is applied to an HPA Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	hpaSpecEdits  []func(*editors.HPASpecEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes HorizontalPodAutoscaler.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the HPA in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	hpa *autoscalingv2.HorizontalPodAutoscaler

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given HorizontalPodAutoscaler.
// BeginFeature must be called before registering any mutations.
func NewMutator(hpa *autoscalingv2.HorizontalPodAutoscaler) *Mutator {
	return &Mutator{
		hpa: hpa,
	}
}

// BeginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) BeginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the HPA's own metadata.
//
// Metadata edits are applied before HPA spec edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditHPASpec records a mutation for the HPA's spec.
//
// HPA spec edits are applied after metadata edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditHPASpec(edit func(*editors.HPASpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.hpaSpecEdits = append(m.active.hpaSpecEdits, edit)
}

// Apply executes all recorded mutation intents on the underlying HPA.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. HPA spec edits (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the HPA as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.hpa.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. HPA spec edits
		if len(plan.hpaSpecEdits) > 0 {
			editor := editors.NewHPASpecEditor(&m.hpa.Spec)
			for _, edit := range plan.hpaSpecEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
