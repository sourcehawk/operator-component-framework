// Package generic provides generic builders and resources for operator components.
package generic

import "sigs.k8s.io/controller-runtime/pkg/client"

// StaticBuilder configures a generic internal static resource for Kubernetes objects
// such as ConfigMaps and Secrets.
//
// It captures the common framework concepts for static desired-state resources while
// leaving concrete identity and default field application behavior to the caller.
type StaticBuilder[T client.Object, M MutatorApplier] struct {
	BaseBuilder[T, M]
	res *StaticResource[T, M]
}

// NewStaticBuilder creates a new generic static builder.
//
// The provided object is treated as the desired base state. The identity function
// must return a stable framework identity for the object. The default applicator
// defines the baseline field application strategy when no custom applicator is set.
// The mutator factory constructs a typed mutator during each Mutate call.
func NewStaticBuilder[T client.Object, M MutatorApplier](
	obj T,
	identityFunc func(T) string,
	defaultApplicator FieldApplicator[T],
	newMutator func(T) M,
) *StaticBuilder[T, M] {
	res := &StaticResource[T, M]{}
	b := &StaticBuilder[T, M]{
		res: res,
	}
	b.InitBase(obj, identityFunc, defaultApplicator, newMutator)
	b.res.BaseResource = *b.BaseRes
	return b
}

// WithMutation registers a typed feature mutation for the static resource.
func (b *StaticBuilder[T, M]) WithMutation(m Mutation[M]) *StaticBuilder[T, M] {
	b.BaseBuilder.WithMutation(m)
	return b
}

// WithCustomFieldApplicator overrides the default baseline field applicator.
func (b *StaticBuilder[T, M]) WithCustomFieldApplicator(
	applicator FieldApplicator[T],
) *StaticBuilder[T, M] {
	b.BaseBuilder.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
//
// Flavors are applied after the default or custom field applicator has produced the
// baseline applied object. Flavors run in registration order.
func (b *StaticBuilder[T, M]) WithFieldApplicationFlavor(
	flavor FieldApplicationFlavor[T],
) *StaticBuilder[T, M] {
	b.BaseBuilder.WithFieldApplicationFlavor(flavor)
	return b
}

// WithDataExtractor registers a typed data extractor to run after successful
// reconciliation.
func (b *StaticBuilder[T, M]) WithDataExtractor(
	extractor func(T) error,
) *StaticBuilder[T, M] {
	b.BaseBuilder.WithDataExtractor(extractor)
	return b
}

// Build validates the static builder configuration and returns the initialized resource.
func (b *StaticBuilder[T, M]) Build() (*StaticResource[T, M], error) {
	b.res.BaseResource = *b.BaseRes
	if err := b.ValidateBase(); err != nil {
		return nil, err
	}
	return b.res, nil
}
