// Package generic provides generic builders and resources for operator components.
package generic

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StaticBuilder configures a generic internal static resource for Kubernetes objects
// such as ConfigMaps and Secrets.
//
// It captures the common framework concepts for static desired-state resources while
// leaving concrete identity behavior to the caller.
type StaticBuilder[T client.Object, M FeatureMutator] struct {
	BaseBuilder[T, M]
	res *StaticResource[T, M]
}

// NewStaticBuilder creates a new generic static builder.
//
// The provided object is treated as the desired base state. The identity function
// must return a stable framework identity for the object. The mutator factory
// constructs a typed mutator during each Mutate call.
func NewStaticBuilder[T client.Object, M FeatureMutator](
	obj T,
	identityFunc func(T) string,
	newMutator func(T) M,
) *StaticBuilder[T, M] {
	res := &StaticResource[T, M]{}
	b := &StaticBuilder[T, M]{
		res: res,
	}
	b.InitBase(obj, identityFunc, newMutator)
	b.res.BaseResource = *b.BaseRes
	return b
}

// WithMutation registers one or more typed feature mutations for the static
// resource, in the order provided.
func (b *StaticBuilder[T, M]) WithMutation(ms ...Mutation[M]) *StaticBuilder[T, M] {
	b.BaseBuilder.WithMutation(ms...)
	return b
}

// WithGuard registers a guard precondition for the static resource.
func (b *StaticBuilder[T, M]) WithGuard(
	handler func(T) (concepts.GuardStatusWithReason, error),
) *StaticBuilder[T, M] {
	b.BaseBuilder.WithGuard(handler)
	return b
}

// WithDataGuard declares blocking data reads for the static resource. See
// BaseBuilder.WithDataGuard.
func (b *StaticBuilder[T, M]) WithDataGuard(cells ...concepts.DataCell) *StaticBuilder[T, M] {
	b.BaseBuilder.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares non-blocking data reads for the static resource.
// See BaseBuilder.WithOptionalData.
func (b *StaticBuilder[T, M]) WithOptionalData(cells ...concepts.DataCell) *StaticBuilder[T, M] {
	b.BaseBuilder.WithOptionalData(cells...)
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
