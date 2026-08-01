package networkpolicy

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	networkingv1 "k8s.io/api/networking/v1"
)

// Builder is a configuration helper for creating and customizing a NetworkPolicy
// Resource.
//
// It provides a fluent API for registering mutations and data extractors.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*networkingv1.NetworkPolicy, *Mutator]
}

// NewBuilder initializes a new Builder with the provided NetworkPolicy object.
//
// The NetworkPolicy object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// The provided NetworkPolicy must have both Name and Namespace set, which is
// validated during the Build() call.
func NewBuilder(np *networkingv1.NetworkPolicy) *Builder {
	identityFunc := func(n *networkingv1.NetworkPolicy) string {
		return fmt.Sprintf("networking.k8s.io/v1/NetworkPolicy/%s/%s", n.Namespace, n.Name)
	}

	return &Builder{
		base: generic.NewStaticBuilder[*networkingv1.NetworkPolicy, *Mutator](
			np,
			identityFunc,
			NewMutator,
		),
	}
}

// WithMutation registers one or more mutations for the NetworkPolicy.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(ms ...Mutation) *Builder {
	for _, m := range ms {
		b.base.WithMutation(feature.Mutation[*Mutator](m))
	}
	return b
}

// WithGuard registers a guard precondition that is evaluated before the NetworkPolicy
// is applied during reconciliation. If the guard returns Blocked, the NetworkPolicy and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(networkingv1.NetworkPolicy) (concepts.GuardStatusWithReason, error)) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the NetworkPolicy reads the given data cells and
// must not be applied until every one of them is set. The framework generates
// the guard and its reason (waiting for data "<name>"), and component Build
// validates that a producer for each cell is registered earlier. Data guards
// are evaluated before any custom guard registered with WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the NetworkPolicy reads the given data cells
// without gating on them. Component Build still validates that a producer is
// registered earlier, and the dependency stays visible to introspection.
// Consumers in this mode use Get and skip quietly when a cell is absent.
func (b *Builder) WithOptionalData(cells ...concepts.DataCell) *Builder {
	b.base.WithOptionalData(cells...)
	return b
}

// WithDataExtractor registers a function to read values from the NetworkPolicy
// after it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled NetworkPolicy. This is
// useful for surfacing the applied policy rules to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(networkingv1.NetworkPolicy) error) *Builder {
	b.base.WithDataExtractor(generic.WrapExtractor(extractor))
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No NetworkPolicy object was provided.
//   - The NetworkPolicy is missing a Name or Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}

// ExtractInto declares that this NetworkPolicy produces the value of cell. fn
// computes the value from a copy of the reconciled NetworkPolicy; the framework
// stores it in the cell and marks it present, immediately after the NetworkPolicy
// is applied or fetched. Extracting several values means several ExtractInto
// calls, one per cell. This is a package-level function because Go methods
// cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(networkingv1.NetworkPolicy) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
