package service

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	corev1 "k8s.io/api/core/v1"
)

// Mutation defines a mutation that is applied to a service Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits    []func(*editors.ObjectMetaEditor) error
	serviceSpecEdits []func(*editors.ServiceSpecEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes Service.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the Service in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	svc *corev1.Service

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given Service.
// The constructor creates the initial feature scope automatically.
func NewMutator(svc *corev1.Service) *Mutator {
	m := &Mutator{
		svc: svc,
	}
	m.NextFeature()
	return m
}

// NextFeature advances to a new feature planning scope. All subsequent mutation
// registrations will be grouped into this scope until NextFeature is called again.
//
// The first scope is created automatically by NewMutator. This method is called
// by the framework between mutations to maintain per-feature ordering semantics.
func (m *Mutator) NextFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the Service's own metadata.
//
// Metadata edits are applied before service spec edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditServiceSpec records a mutation for the Service's spec.
//
// The editor provides structured operations (SetType, EnsurePort, SetSelector,
// etc.) as well as Raw() for free-form access. Service spec edits are applied
// after metadata edits within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditServiceSpec(edit func(*editors.ServiceSpecEditor) error) {
	if edit == nil {
		return
	}
	m.active.serviceSpecEdits = append(m.active.serviceSpecEdits, edit)
}

// Apply executes all recorded mutation intents on the underlying Service.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Service spec edits (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the Service as modified by all previous features.
//
// After all edits are applied, auto-allocated NodePort values that were present
// before mutations are restored for NodePort and LoadBalancer Services when the
// resulting port's NodePort is 0 (unset). This prevents mutations that replace
// ports via EnsurePort from unintentionally clearing server-assigned NodePorts.
func (m *Mutator) Apply() error {
	// Snapshot ports before mutations so we can restore auto-allocated
	// NodePorts that mutations may inadvertently clear.
	snapshot := m.svc.DeepCopy()

	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.svc.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Service spec edits
		if len(plan.serviceSpecEdits) > 0 {
			editor := editors.NewServiceSpecEditor(&m.svc.Spec)
			for _, edit := range plan.serviceSpecEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	// Restore auto-allocated NodePorts for types that support them.
	effectiveType := m.svc.Spec.Type
	if effectiveType == "" {
		effectiveType = corev1.ServiceTypeClusterIP
	}
	if effectiveType == corev1.ServiceTypeNodePort || effectiveType == corev1.ServiceTypeLoadBalancer {
		preserveNodePorts(m.svc, snapshot)
	}

	return nil
}
