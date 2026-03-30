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

// WithMutation registers a mutation for the NetworkPolicy.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithGuard registers a guard precondition that is evaluated before the NetworkPolicy
// is applied during reconciliation. If the guard returns Blocked, the NetworkPolicy and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(networkingv1.NetworkPolicy) (concepts.GuardStatusWithReason, error)) *Builder {
	if guard == nil {
		b.base.WithGuard(nil)
		return b
	}
	b.base.WithGuard(func(np *networkingv1.NetworkPolicy) (concepts.GuardStatusWithReason, error) {
		return guard(*np)
	})
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
	if extractor != nil {
		b.base.WithDataExtractor(func(np *networkingv1.NetworkPolicy) error {
			return extractor(*np)
		})
	}
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
