// Package serviceaccount provides a builder and resource for managing Kubernetes ServiceAccounts.
package serviceaccount

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	corev1 "k8s.io/api/core/v1"
)

// Mutation defines a mutation that is applied to a serviceaccount Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits        []func(*editors.ObjectMetaEditor) error
	imagePullSecretEdits []func(*corev1.ServiceAccount)
	automountEdits       []func(*corev1.ServiceAccount)
}

// Mutator is a high-level helper for modifying a Kubernetes ServiceAccount.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the ServiceAccount in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	sa *corev1.ServiceAccount

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given ServiceAccount.
func NewMutator(sa *corev1.ServiceAccount) *Mutator {
	m := &Mutator{
		sa:    sa,
		plans: []featurePlan{{}},
	}
	m.active = &m.plans[0]
	return m
}

// BeginFeature starts a new feature planning scope. All subsequent mutation
// registrations will be grouped into this feature's plan.
//
// This method satisfies the generic.FeatureMutator interface, allowing the
// generic builder to maintain per-feature ordering semantics for external mutators.
func (m *Mutator) BeginFeature() {
	m.plans = append(m.plans, featurePlan{})
	m.active = &m.plans[len(m.plans)-1]
}

// EditObjectMetadata records a mutation for the ServiceAccount's own metadata.
//
// Metadata edits are applied before image pull secret and automount edits within
// the same feature. A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EnsureImagePullSecret records that the named image pull secret should be present
// in .imagePullSecrets. If a secret with the same name already exists, it is a no-op.
func (m *Mutator) EnsureImagePullSecret(name string) {
	m.active.imagePullSecretEdits = append(m.active.imagePullSecretEdits, func(sa *corev1.ServiceAccount) {
		for _, ref := range sa.ImagePullSecrets {
			if ref.Name == name {
				return
			}
		}
		sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
	})
}

// RemoveImagePullSecret records that the named image pull secret should be removed
// from .imagePullSecrets. It is a no-op if the secret is not present.
func (m *Mutator) RemoveImagePullSecret(name string) {
	m.active.imagePullSecretEdits = append(m.active.imagePullSecretEdits, func(sa *corev1.ServiceAccount) {
		filtered := sa.ImagePullSecrets[:0]
		for _, ref := range sa.ImagePullSecrets {
			if ref.Name != name {
				filtered = append(filtered, ref)
			}
		}
		sa.ImagePullSecrets = filtered
	})
}

// SetAutomountServiceAccountToken records that .automountServiceAccountToken should
// be set to the provided value. Pass nil to unset the field.
func (m *Mutator) SetAutomountServiceAccountToken(v *bool) {
	m.active.automountEdits = append(m.active.automountEdits, func(sa *corev1.ServiceAccount) {
		sa.AutomountServiceAccountToken = v
	})
}

// Apply executes all recorded mutation intents on the underlying ServiceAccount.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Image pull secret edits — EnsureImagePullSecret, RemoveImagePullSecret (in registration order within each feature)
//  3. Automount edits — SetAutomountServiceAccountToken (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the ServiceAccount as modified by all previous features.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits
		if len(plan.metadataEdits) > 0 {
			editor := editors.NewObjectMetaEditor(&m.sa.ObjectMeta)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}

		// 2. Image pull secret edits
		for _, edit := range plan.imagePullSecretEdits {
			edit(m.sa)
		}

		// 3. Automount edits
		for _, edit := range plan.automountEdits {
			edit(m.sa)
		}
	}

	return nil
}
