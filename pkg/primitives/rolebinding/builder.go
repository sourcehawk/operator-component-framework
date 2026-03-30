package rolebinding

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Builder is a configuration helper for creating and customizing a RoleBinding Resource.
//
// It provides a fluent API for registering mutations and data extractors.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*rbacv1.RoleBinding, *Mutator]
}

// NewBuilder initializes a new Builder with the provided RoleBinding object.
//
// The RoleBinding object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// roleRef must be set on the provided RoleBinding object. It is immutable after
// creation and is not modifiable via the mutation API.
//
// The provided RoleBinding must have both Name and Namespace set, which is
// validated during the Build() call.
func NewBuilder(rb *rbacv1.RoleBinding) *Builder {
	identityFunc := func(r *rbacv1.RoleBinding) string {
		return fmt.Sprintf("rbac.authorization.k8s.io/v1/RoleBinding/%s/%s", r.Namespace, r.Name)
	}

	return &Builder{
		base: generic.NewStaticBuilder[*rbacv1.RoleBinding, *Mutator](
			rb,
			identityFunc,
			NewMutator,
		),
	}
}

// WithMutation registers a mutation for the RoleBinding.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithGuard registers a guard precondition that is evaluated before the RoleBinding
// is applied during reconciliation. If the guard returns Blocked, the RoleBinding and
// all resources registered after it are skipped until the guard clears.
func (b *Builder) WithGuard(guard func(rbacv1.RoleBinding) (concepts.GuardStatusWithReason, error)) *Builder {
	if guard == nil {
		b.base.WithGuard(nil)
		return b
	}
	b.base.WithGuard(func(rb *rbacv1.RoleBinding) (concepts.GuardStatusWithReason, error) {
		return guard(*rb)
	})
	return b
}

// WithDataExtractor registers a function to read values from the RoleBinding
// after it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled RoleBinding. This is
// useful for surfacing generated or updated entries to other components or
// resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(rbacv1.RoleBinding) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(rb *rbacv1.RoleBinding) error {
			return extractor(*rb)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No RoleBinding object was provided.
//   - The RoleBinding is missing a Name or Namespace.
//   - The RoleRef is missing APIGroup, Kind, or Name.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}

	ref := genericRes.DesiredObject.RoleRef
	if ref.APIGroup == "" || ref.Kind == "" || ref.Name == "" {
		return nil, fmt.Errorf("roleRef must have non-empty APIGroup, Kind, and Name")
	}

	return &Resource{base: genericRes}, nil
}
