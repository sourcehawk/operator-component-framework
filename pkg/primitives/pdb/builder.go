package pdb

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	policyv1 "k8s.io/api/policy/v1"
)

// Builder is a configuration helper for creating and customizing a PodDisruptionBudget Resource.
//
// It provides a fluent API for registering mutations, field application flavors,
// and data extractors. Build() validates the configuration and returns an
// initialized Resource ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*policyv1.PodDisruptionBudget, *Mutator]
}

// NewBuilder initializes a new Builder with the provided PodDisruptionBudget object.
//
// The PodDisruptionBudget object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations and flavors.
//
// The provided PodDisruptionBudget must have both Name and Namespace set, which is
// validated during the Build() call.
func NewBuilder(p *policyv1.PodDisruptionBudget) *Builder {
	identityFunc := func(pdb *policyv1.PodDisruptionBudget) string {
		return fmt.Sprintf("policy/v1/PodDisruptionBudget/%s/%s", pdb.Namespace, pdb.Name)
	}

	return &Builder{
		base: generic.NewStaticBuilder[*policyv1.PodDisruptionBudget, *Mutator](
			p,
			identityFunc,
			DefaultFieldApplicator,
			NewMutator,
		),
	}
}

// WithMutation registers a mutation for the PodDisruptionBudget.
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
// state to the existing PodDisruptionBudget in the cluster.
//
// The default applicator (DefaultFieldApplicator) replaces the current object
// with a deep copy of the desired object. Use a custom applicator when other
// controllers manage fields you need to preserve.
//
// The applicator receives the current object from the API server and the desired
// object from the Resource, and is responsible for merging the desired changes
// into the current object.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current, desired *policyv1.PodDisruptionBudget) error,
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
	b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*policyv1.PodDisruptionBudget](flavor))
	return b
}

// WithDataExtractor registers a function to read values from the PodDisruptionBudget
// after it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled PodDisruptionBudget. This is
// useful for surfacing generated or updated values to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(policyv1.PodDisruptionBudget) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(p *policyv1.PodDisruptionBudget) error {
			return extractor(*p)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No PodDisruptionBudget object was provided.
//   - The PodDisruptionBudget is missing a Name or Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
