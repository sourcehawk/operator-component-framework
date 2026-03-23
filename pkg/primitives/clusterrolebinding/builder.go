package clusterrolebinding

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Builder is a configuration helper for creating and customizing a ClusterRoleBinding Resource.
//
// It provides a fluent API for registering mutations, field application flavors,
// and data extractors. Build() validates the configuration and returns an
// initialized Resource ready for use in a reconciliation loop.
type Builder struct {
	base *generic.StaticBuilder[*rbacv1.ClusterRoleBinding, *Mutator]
}

// NewBuilder initializes a new Builder with the provided ClusterRoleBinding object.
//
// The ClusterRoleBinding object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations and flavors.
//
// The provided ClusterRoleBinding must have a Name set, which is validated
// during the Build() call. Namespace must be empty as ClusterRoleBinding is
// cluster-scoped.
func NewBuilder(crb *rbacv1.ClusterRoleBinding) *Builder {
	identityFunc := func(c *rbacv1.ClusterRoleBinding) string {
		return fmt.Sprintf("rbac.authorization.k8s.io/v1/ClusterRoleBinding/%s", c.Name)
	}

	base := generic.NewStaticBuilder[*rbacv1.ClusterRoleBinding, *Mutator](
		crb,
		identityFunc,
		DefaultFieldApplicator,
		NewMutator,
	)
	base.MarkClusterScoped()

	return &Builder{
		base: base,
	}
}

// WithMutation registers a mutation for the ClusterRoleBinding.
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
// state to the existing ClusterRoleBinding in the cluster.
//
// The default applicator (DefaultFieldApplicator) replaces the current object
// with a deep copy of the desired object, preserving roleRef on updates since
// it is immutable after creation.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current, desired *rbacv1.ClusterRoleBinding) error,
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
	if flavor != nil {
		b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*rbacv1.ClusterRoleBinding](flavor))
	}
	return b
}

// WithDataExtractor registers a function to read values from the ClusterRoleBinding
// after it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled ClusterRoleBinding. This
// is useful for surfacing generated or updated entries to other components or
// resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(rbacv1.ClusterRoleBinding) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(crb *rbacv1.ClusterRoleBinding) error {
			return extractor(*crb)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No ClusterRoleBinding object was provided.
//   - The ClusterRoleBinding is missing a Name.
//   - The ClusterRoleBinding has a non-empty Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
