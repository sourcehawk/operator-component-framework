package pdb

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	policyv1 "k8s.io/api/policy/v1"
)

// Builder is a configuration helper for creating and customizing a PodDisruptionBudget Resource.
//
// It provides a fluent API for registering mutations and data extractors.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*policyv1.PodDisruptionBudget, *Mutator]
}

// NewBuilder initializes a new Builder with the provided PodDisruptionBudget object.
//
// The PodDisruptionBudget object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
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
			NewMutator,
		),
	}
}

// WithMutation registers a mutation for the PodDisruptionBudget.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithGuard registers a guard precondition that is evaluated before the PodDisruptionBudget
// is applied during reconciliation. If the guard returns Blocked, the PodDisruptionBudget and
// all resources registered after it are skipped until the guard clears.
func (b *Builder) WithGuard(guard func(policyv1.PodDisruptionBudget) (concepts.GuardStatusWithReason, error)) *Builder {
	if guard == nil {
		b.base.WithGuard(nil)
		return b
	}
	b.base.WithGuard(func(p *policyv1.PodDisruptionBudget) (concepts.GuardStatusWithReason, error) {
		return guard(*p)
	})
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
