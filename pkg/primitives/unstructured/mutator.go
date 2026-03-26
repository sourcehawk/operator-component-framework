// Package unstructured provides shared types for building unstructured Kubernetes
// resource primitives.
//
// It defines the Mutator and Mutation types used by all four concept-specific
// sub-packages (workload, integration, static, task). The Mutator follows the
// same plan-and-apply pattern used by typed primitives: mutations are recorded
// first, then applied in a single controlled pass when Apply() is called.
package unstructured

import (
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Mutation defines a mutation that is applied to an unstructured Mutator
// only if its associated feature gate is enabled.
type Mutation feature.Mutation[*Mutator]

type featurePlan struct {
	metadataEdits []func(*editors.ObjectMetaEditor) error
	contentEdits  []func(*editors.UnstructuredContentEditor) error
}

// Mutator is a high-level helper for modifying a Kubernetes unstructured object.
//
// It uses a "plan-and-apply" pattern: mutations are recorded first, then
// applied to the object in a single controlled pass when Apply() is called.
//
// The Mutator maintains feature boundaries: each feature's mutations are planned
// together and applied in the order the features were registered.
//
// Mutator implements editors.ObjectMutator.
type Mutator struct {
	obj *uns.Unstructured

	plans  []featurePlan
	active *featurePlan
}

// NewMutator creates a new Mutator for the given unstructured object.
// The constructor creates the initial feature scope, so mutations can be
// registered immediately without an explicit call to NextFeature.
func NewMutator(obj *uns.Unstructured) *Mutator {
	m := &Mutator{
		obj: obj,
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

// EditObjectMetadata records a mutation for the unstructured object's metadata.
//
// Metadata edits are applied before content edits within the same feature.
// A nil edit function is ignored.
func (m *Mutator) EditObjectMetadata(edit func(*editors.ObjectMetaEditor) error) {
	if edit == nil {
		return
	}
	m.active.metadataEdits = append(m.active.metadataEdits, edit)
}

// EditContent records a mutation for the unstructured object's content via an
// UnstructuredContentEditor.
//
// The editor provides structured operations (SetNestedField, RemoveNestedField,
// typed setters) as well as Raw() for free-form access. Content edits are applied
// after metadata edits within the same feature, in registration order.
//
// A nil edit function is ignored.
func (m *Mutator) EditContent(edit func(*editors.UnstructuredContentEditor) error) {
	if edit == nil {
		return
	}
	m.active.contentEdits = append(m.active.contentEdits, edit)
}

// Apply executes all recorded mutation intents on the underlying unstructured object.
//
// Execution order across all registered features:
//
//  1. Metadata edits (in registration order within each feature)
//  2. Content edits (in registration order within each feature)
//
// Features are applied in the order they were registered. Later features observe
// the object as modified by all previous features.
//
// Since *unstructured.Unstructured does not embed metav1.ObjectMeta, metadata
// edits are bridged: a temporary ObjectMeta is populated from the object's
// labels and annotations, the ObjectMetaEditor mutates it, and the results
// are written back via SetLabels/SetAnnotations.
func (m *Mutator) Apply() error {
	for _, plan := range m.plans {
		// 1. Metadata edits: bridge through a temporary ObjectMeta.
		if len(plan.metadataEdits) > 0 {
			tmp := &metav1.ObjectMeta{
				Labels:      m.obj.GetLabels(),
				Annotations: m.obj.GetAnnotations(),
			}
			editor := editors.NewObjectMetaEditor(tmp)
			for _, edit := range plan.metadataEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
			m.obj.SetLabels(tmp.Labels)
			m.obj.SetAnnotations(tmp.Annotations)
		}

		// 2. Content edits: directly on the object's content map.
		if len(plan.contentEdits) > 0 {
			editor := editors.NewUnstructuredContentEditor(m.obj.Object)
			for _, edit := range plan.contentEdits {
				if err := edit(editor); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
