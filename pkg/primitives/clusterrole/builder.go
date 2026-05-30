package clusterrole

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Builder is a configuration helper for creating and customizing a ClusterRole Resource.
//
// It provides a fluent API for registering mutations and data extractors.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*rbacv1.ClusterRole, *Mutator]
}

// NewBuilder initializes a new Builder with the provided ClusterRole object.
//
// The ClusterRole object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// The provided ClusterRole must have Name set (ClusterRole is cluster-scoped and
// does not use a namespace), which is validated during the Build() call.
func NewBuilder(cr *rbacv1.ClusterRole) *Builder {
	identityFunc := func(cr *rbacv1.ClusterRole) string {
		return fmt.Sprintf("rbac.authorization.k8s.io/v1/ClusterRole/%s", cr.Name)
	}

	sb := generic.NewStaticBuilder[*rbacv1.ClusterRole, *Mutator](
		cr,
		identityFunc,
		NewMutator,
	)
	sb.MarkClusterScoped()

	return &Builder{base: sb}
}

// WithMutation registers one or more mutations for the ClusterRole.
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

// WithGuard registers a guard precondition that is evaluated before the ClusterRole
// is applied during reconciliation. If the guard returns Blocked, the ClusterRole and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(rbacv1.ClusterRole) (concepts.GuardStatusWithReason, error)) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataExtractor registers a function to read values from the ClusterRole after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled ClusterRole. This is useful
// for surfacing generated or updated fields to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(rbacv1.ClusterRole) error) *Builder {
	b.base.WithDataExtractor(generic.WrapExtractor(extractor))
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No ClusterRole object was provided.
//   - The ClusterRole is missing a Name.
func (b *Builder) Build() (*Resource, error) {
	res, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: res}, nil
}
