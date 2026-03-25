package secret

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a Secret Resource.
//
// It provides a fluent API for registering mutations and data extractors.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*corev1.Secret, *Mutator]
}

// NewBuilder initializes a new Builder with the provided Secret object.
//
// The Secret object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// The provided Secret must have both Name and Namespace set, which is validated
// during the Build() call.
func NewBuilder(s *corev1.Secret) *Builder {
	identityFunc := func(secret *corev1.Secret) string {
		return fmt.Sprintf("v1/Secret/%s/%s", secret.Namespace, secret.Name)
	}

	return &Builder{
		base: generic.NewStaticBuilder[*corev1.Secret, *Mutator](
			s,
			identityFunc,
			NewMutator,
		),
	}
}

// WithMutation registers a mutation for the Secret.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithDataExtractor registers a function to read values from the Secret after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled Secret. This is useful
// for surfacing generated or updated entries to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(corev1.Secret) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(s *corev1.Secret) error {
			return extractor(*s)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No Secret object was provided.
//   - The Secret is missing a Name or Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
