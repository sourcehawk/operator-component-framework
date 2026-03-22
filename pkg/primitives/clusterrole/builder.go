package clusterrole

import (
	"errors"
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Builder is a configuration helper for creating and customizing a ClusterRole Resource.
//
// It provides a fluent API for registering mutations, field application flavors,
// and data extractors. Build() validates the configuration and returns an
// initialized Resource ready for use in a reconciliation loop.
type Builder struct {
	obj        *rbacv1.ClusterRole
	mutations  []feature.Mutation[*Mutator]
	applicator func(current, desired *rbacv1.ClusterRole) error
	flavors    []generic.FieldApplicationFlavor[*rbacv1.ClusterRole]
	extractors []func(*rbacv1.ClusterRole) error
}

// NewBuilder initializes a new Builder with the provided ClusterRole object.
//
// The ClusterRole object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations and flavors.
//
// The provided ClusterRole must have Name set (ClusterRole is cluster-scoped and
// does not use a namespace), which is validated during the Build() call.
func NewBuilder(cr *rbacv1.ClusterRole) *Builder {
	return &Builder{obj: cr}
}

// WithMutation registers a mutation for the ClusterRole.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation,
// after the baseline field applicator and any registered flavors have run.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.mutations = append(b.mutations, feature.Mutation[*Mutator](m))
	return b
}

// WithCustomFieldApplicator sets a custom strategy for applying the desired
// state to the existing ClusterRole in the cluster.
//
// The default applicator (DefaultFieldApplicator) replaces the current object
// with a deep copy of the desired object. Use a custom applicator when other
// controllers manage fields you need to preserve.
//
// The applicator receives the current object from the API server and the desired
// object from the Resource, and is responsible for merging the desired changes
// into the current object.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current, desired *rbacv1.ClusterRole) error,
) *Builder {
	b.applicator = applicator
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
	if flavor != nil {
		b.flavors = append(b.flavors, generic.FieldApplicationFlavor[*rbacv1.ClusterRole](flavor))
	}
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
	if extractor != nil {
		b.extractors = append(b.extractors, func(cr *rbacv1.ClusterRole) error {
			return extractor(*cr)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No ClusterRole object was provided.
//   - The ClusterRole is missing a Name.
func (b *Builder) Build() (*Resource, error) {
	if b.obj == nil {
		return nil, errors.New("object cannot be nil")
	}
	if b.obj.Name == "" {
		return nil, errors.New("object name cannot be empty")
	}

	identityFunc := func(cr *rbacv1.ClusterRole) string {
		return fmt.Sprintf("rbac.authorization.k8s.io/v1/ClusterRole/%s", cr.Name)
	}

	sb := generic.NewStaticBuilder[*rbacv1.ClusterRole, *Mutator](
		b.obj,
		identityFunc,
		DefaultFieldApplicator,
		NewMutator,
	)
	sb.MarkClusterScoped()

	for _, m := range b.mutations {
		sb.WithMutation(m)
	}

	if b.applicator != nil {
		sb.WithCustomFieldApplicator(b.applicator)
	}

	for _, f := range b.flavors {
		sb.WithFieldApplicationFlavor(f)
	}

	for _, e := range b.extractors {
		sb.WithDataExtractor(e)
	}

	res, err := sb.Build()
	if err != nil {
		return nil, err
	}

	return &Resource{base: res}, nil
}
