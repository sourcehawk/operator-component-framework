package static

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Builder is a configuration helper for creating and customizing a static
// unstructured Resource.
//
// It provides a fluent API for registering mutations and data extractors.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base         *generic.StaticBuilder[*uns.Unstructured, *unstruct.Mutator]
	clusterScope bool
}

// NewBuilder initializes a new Builder with the provided unstructured object.
//
// The object serves as the desired base state. During reconciliation the
// Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// The provided object must have a Name set. Namespaced resources must also
// have a Namespace; cluster-scoped resources must call MarkClusterScoped.
func NewBuilder(obj *uns.Unstructured) *Builder {
	// Identity function is set at Build() time once we know the cluster-scope flag.
	placeholder := func(_ *uns.Unstructured) string { return "" }

	return &Builder{
		base: generic.NewStaticBuilder[*uns.Unstructured, *unstruct.Mutator](
			obj,
			placeholder,
			unstruct.NewMutator,
		),
	}
}

// MarkClusterScoped marks the resource as cluster-scoped. Build() will reject
// a non-empty namespace instead of requiring one.
func (b *Builder) MarkClusterScoped() *Builder {
	b.clusterScope = true
	b.base.MarkClusterScoped()
	return b
}

// WithMutation registers a mutation for the unstructured object.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m unstruct.Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*unstruct.Mutator](m))
	return b
}

// WithGuard registers a guard precondition that is evaluated before the object
// is applied during reconciliation. If the guard returns Blocked, the object and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(uns.Unstructured) (concepts.GuardStatusWithReason, error)) *Builder {
	if guard == nil {
		b.base.WithGuard(nil)
		return b
	}
	b.base.WithGuard(func(obj *uns.Unstructured) (concepts.GuardStatusWithReason, error) {
		return guard(*obj)
	})
	return b
}

// WithDataExtractor registers a function to read values from the object after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled object. A nil extractor
// is ignored.
func (b *Builder) WithDataExtractor(extractor func(uns.Unstructured) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(obj *uns.Unstructured) error {
			return extractor(*obj)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No object was provided.
//   - The object is missing a Name.
//   - A namespaced object is missing a Namespace (and MarkClusterScoped was not called).
//   - A cluster-scoped object has a Namespace set.
func (b *Builder) Build() (*Resource, error) {
	b.base.BaseRes.IdentityFunc = unstruct.MakeIdentityFunc(b.clusterScope)
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
