package networkpolicy

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	networkingv1 "k8s.io/api/networking/v1"
)

// Builder is a configuration helper for creating and customizing a NetworkPolicy
// Resource.
//
// It provides a fluent API for registering mutations, field application flavors,
// and data extractors. Build() validates the configuration and returns an
// initialized Resource ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*networkingv1.NetworkPolicy, *Mutator]
}

// NewBuilder initializes a new Builder with the provided NetworkPolicy object.
//
// The NetworkPolicy object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations and flavors.
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
			DefaultFieldApplicator,
			NewMutator,
		),
	}
}

// WithMutation registers a mutation for the NetworkPolicy.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation,
// after the baseline field applicator and any registered flavors have run.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithCustomFieldApplicator sets a custom strategy for applying the desired
// state to the existing NetworkPolicy in the cluster.
//
// The default applicator (DefaultFieldApplicator) replaces the current object
// with a deep copy of the desired object. Use a custom applicator when other
// controllers manage fields you need to preserve.
//
// The applicator receives the current object from the API server and the desired
// object from the Resource, and is responsible for merging the desired changes
// into the current object.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current, desired *networkingv1.NetworkPolicy) error,
) *Builder {
	b.base.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
//
// Flavors run after the baseline applicator (default or custom) in registration
// order. They are typically used to preserve fields from the live cluster object
// that should not be overwritten by the desired state.
//
// A nil flavor is ignored.
func (b *Builder) WithFieldApplicationFlavor(flavor FieldApplicationFlavor) *Builder {
	b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*networkingv1.NetworkPolicy](flavor))
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
