// Package generic provides generic builders and resources for operator components.
package generic

import (
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StaticBuilder configures a generic internal static resource for Kubernetes objects
// such as ConfigMaps and Secrets.
//
// It captures the common framework concepts for static desired-state resources while
// leaving concrete identity and default field application behavior to the caller.
type StaticBuilder[T client.Object] struct {
	res *StaticResource[T]
}

// NewStaticBuilder creates a new generic static builder.
//
// The provided object is treated as the desired base state. The identity function
// must return a stable framework identity for the object. The default applicator
// defines the baseline field application strategy when no custom applicator is set.
func NewStaticBuilder[T client.Object](
	obj T,
	identityFunc func(T) string,
	defaultApplicator FieldApplicator[T],
) *StaticBuilder[T] {
	return &StaticBuilder[T]{
		res: &StaticResource[T]{
			DesiredObject:          obj,
			IdentityFunc:           identityFunc,
			DefaultFieldApplicator: defaultApplicator,
		},
	}
}

// WithCustomFieldApplicator overrides the default baseline field applicator.
func (b *StaticBuilder[T]) WithCustomFieldApplicator(
	applicator FieldApplicator[T],
) *StaticBuilder[T] {
	b.res.CustomFieldApplicator = applicator
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
//
// Flavors are applied after the default or custom field applicator has produced the
// baseline applied object. Flavors run in registration order.
func (b *StaticBuilder[T]) WithFieldApplicationFlavor(
	flavor FieldApplicationFlavor[T],
) *StaticBuilder[T] {
	if flavor != nil {
		b.res.FieldFlavors = append(b.res.FieldFlavors, flavor)
	}

	return b
}

// WithDataExtractor registers a typed data extractor to run after successful
// reconciliation.
func (b *StaticBuilder[T]) WithDataExtractor(
	extractor func(T) error,
) *StaticBuilder[T] {
	if extractor != nil {
		b.res.DataExtractors = append(b.res.DataExtractors, extractor)
	}

	return b
}

// Build validates the static builder configuration and returns the initialized resource.
func (b *StaticBuilder[T]) Build() (*StaticResource[T], error) {
	if isNil(b.res.DesiredObject) {
		return nil, errors.New("object cannot be nil")
	}

	if b.res.DesiredObject.GetName() == "" {
		return nil, errors.New("object name cannot be empty")
	}

	if b.res.DesiredObject.GetNamespace() == "" {
		return nil, errors.New("object namespace cannot be empty")
	}

	if b.res.IdentityFunc == nil {
		return nil, errors.New("identity function cannot be nil")
	}

	if b.res.DefaultFieldApplicator == nil {
		return nil, errors.New("default field applicator cannot be nil")
	}

	return b.res, nil
}
