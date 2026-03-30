package serviceaccount

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a ServiceAccount Resource.
//
// It provides a fluent API for registering mutations and data extractors.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*corev1.ServiceAccount, *Mutator]
}

// NewBuilder initializes a new Builder with the provided ServiceAccount object.
//
// The ServiceAccount object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// The provided ServiceAccount must have both Name and Namespace set, which is validated
// during the Build() call.
func NewBuilder(sa *corev1.ServiceAccount) *Builder {
	identityFunc := func(s *corev1.ServiceAccount) string {
		return fmt.Sprintf("v1/ServiceAccount/%s/%s", s.Namespace, s.Name)
	}

	return &Builder{
		base: generic.NewStaticBuilder[*corev1.ServiceAccount, *Mutator](
			sa,
			identityFunc,
			NewMutator,
		),
	}
}

// WithMutation registers a mutation for the ServiceAccount.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithGuard registers a guard precondition that is evaluated before the ServiceAccount
// is applied during reconciliation. If the guard returns Blocked, the ServiceAccount and
// all resources registered after it are skipped until the guard clears.
func (b *Builder) WithGuard(guard func(corev1.ServiceAccount) (concepts.GuardStatusWithReason, error)) *Builder {
	if guard == nil {
		b.base.WithGuard(nil)
		return b
	}
	b.base.WithGuard(func(sa *corev1.ServiceAccount) (concepts.GuardStatusWithReason, error) {
		return guard(*sa)
	})
	return b
}

// WithDataExtractor registers a function to read values from the ServiceAccount after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled ServiceAccount. This is useful
// for surfacing generated or updated entries to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(corev1.ServiceAccount) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(sa *corev1.ServiceAccount) error {
			return extractor(*sa)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No ServiceAccount object was provided.
//   - The ServiceAccount is missing a Name or Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
