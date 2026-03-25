package role

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Builder is a configuration helper for creating and customizing a Role Resource.
//
// It provides a fluent API for registering mutations and data extractors.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*rbacv1.Role, *Mutator]
}

// NewBuilder initializes a new Builder with the provided Role object.
//
// The Role object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// The provided Role must have both Name and Namespace set, which is validated
// during the Build() call.
func NewBuilder(role *rbacv1.Role) *Builder {
	identityFunc := func(r *rbacv1.Role) string {
		return fmt.Sprintf("rbac.authorization.k8s.io/v1/Role/%s/%s", r.Namespace, r.Name)
	}

	return &Builder{
		base: generic.NewStaticBuilder[*rbacv1.Role, *Mutator](
			role,
			identityFunc,
			NewMutator,
		),
	}
}

// WithMutation registers a mutation for the Role.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithDataExtractor registers a function to read values from the Role after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled Role. This is useful
// for surfacing generated or updated entries to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(rbacv1.Role) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(r *rbacv1.Role) error {
			return extractor(*r)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No Role object was provided.
//   - The Role is missing a Name or Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
