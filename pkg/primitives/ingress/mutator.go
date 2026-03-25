package ingress

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	networkingv1 "k8s.io/api/networking/v1"
)

// Mutation defines a mutation that is applied to an ingress Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits    []func(*editors.ObjectMetaEditor) error
	ingressSpecEdits []func(*editors.IngressSpecEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes Ingress.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the Ingress in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Apply order within each feature:
//  1. Object metadata edits
//  2. Ingress spec edits
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	ing *networkingv1.Ingress

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given Ingress.
// BeginFeature should be called before registering mutations to establish
// feature boundaries. If omitted, EditObjectMetadata and EditIngressSpec
// will call it implicitly.
func NewMutator(ing *networkingv1.Ingress) *Mutator {
	return &Mutator{
		ing: ing,
	}
}

// BeginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
func (m *Mutator) BeginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the Ingress's own metadata.
//
// Metadata edits are applied before ingress spec edits within the same feature.
// A nil edit function is ignored.
// If BeginFeature has not been called, it is called implicitly.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	if m.active == nil {
		m.BeginFeature()
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditIngressSpec records a mutation for the Ingress's spec via an IngressSpecEditor.
//
// The editor provides structured operations (SetIngressClassName, SetDefaultBackend,
// EnsureRule, RemoveRule, EnsureTLS, RemoveTLS) as well as Raw() for free-form access.
// Ingress spec edits are applied after metadata edits within the same feature, in
// registration order.
//
// A nil edit function is ignored.
// If BeginFeature has not been called, it is called implicitly.
func (m *Mutator) EditIngressSpec(edit func(*editors.IngressSpecEditor) error) {
	if edit == nil {
		return
	}
	if m.active == nil {
		m.BeginFeature()
	}
	m.active.ingressSpecEdits = append(m.active.ingressSpecEdits, edit)
}

// Apply executes all recorded mutation intents on the underlying Ingress.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Ingress spec edits (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the Ingress as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.ing.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Ingress spec edits
		if len(plan.ingressSpecEdits) > 0 {
			editor := editors.NewIngressSpecEditor(&m.ing.Spec)
			for _, edit := range plan.ingressSpecEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
