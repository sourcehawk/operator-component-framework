package generic

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StaticResource is a generic internal resource implementation for Kubernetes objects
// that are treated as static desired-state resources, such as ConfigMaps and Secrets.
//
// It supports:
//   - default or custom baseline field application
//   - post-baseline field application flavors
//   - data extraction after reconciliation
//
// Unlike workload resources, it does not support feature mutations, convergence status,
// or suspension behavior.
type StaticResource[T client.Object] struct {
	DesiredObject T

	IdentityFunc func(T) string

	DefaultFieldApplicator FieldApplicator[T]
	CustomFieldApplicator  FieldApplicator[T]
	FieldFlavors           []FieldApplicationFlavor[T]

	DataExtractors []func(T) error
}

// Identity returns the stable framework identity for the resource.
func (r *StaticResource[T]) Identity() string {
	return r.IdentityFunc(r.DesiredObject)
}

// Object returns a deep copy of the desired resource object.
func (r *StaticResource[T]) Object() (client.Object, error) {
	obj, ok := r.DesiredObject.DeepCopyObject().(client.Object)
	if !ok {
		return nil, fmt.Errorf("failed to deep copy object of type %T", r.DesiredObject)
	}

	return obj, nil
}

// Mutate applies the baseline field applicator and all registered field application
// flavors to the provided current object.
func (r *StaticResource[T]) Mutate(current client.Object) error {
	currentTyped, ok := current.(T)
	if !ok {
		return fmt.Errorf("expected %T, got %T", r.DesiredObject, current)
	}

	applied, err := applyBaselineAndFlavors(
		currentTyped,
		r.DesiredObject,
		r.DefaultFieldApplicator,
		r.CustomFieldApplicator,
		r.FieldFlavors,
	)
	if err != nil {
		return err
	}

	r.DesiredObject = applied
	return nil
}

// ExtractData executes all registered data extractors against a deep copy of the
// reconciled object.
func (r *StaticResource[T]) ExtractData() error {
	copyObj, ok := r.DesiredObject.DeepCopyObject().(T)
	if !ok {
		return fmt.Errorf("failed to deep copy object of type %T", r.DesiredObject)
	}

	for _, extractor := range r.DataExtractors {
		if extractor == nil {
			continue
		}

		if err := extractor(copyObj); err != nil {
			return err
		}
	}

	return nil
}
