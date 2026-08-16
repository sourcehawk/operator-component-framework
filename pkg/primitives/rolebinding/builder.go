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
// It provides a fluent API for registering mutations and declared data extractions.
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

// WithMutation registers one or more mutations for the RoleBinding.
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

// WithGuard registers a guard precondition that is evaluated before the RoleBinding
// is applied during reconciliation. If the guard returns Blocked, the RoleBinding and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(rbacv1.RoleBinding) (concepts.GuardStatusWithReason, error)) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the RoleBinding reads the given data cells and
// must not be applied until every one of them is set. The framework generates
// the guard and its reason (waiting for data "<name>"), and component Build
// validates that a producer for each cell is registered earlier. Data guards
// are evaluated before any custom guard registered with WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the RoleBinding reads the given data cells
// without gating on them. Component Build still validates that a producer is
// registered earlier, and the dependency stays visible to introspection.
// Consumers in this mode use Get and skip quietly when a cell is absent.
func (b *Builder) WithOptionalData(cells ...concepts.DataCell) *Builder {
	b.base.WithOptionalData(cells...)
	return b
}

// WithMetricsIdentifier sets the RoleBinding's identifier for
// resource-level metrics, used as the value of the `resource` label on
// ocf_resource_apply_total and ocf_resource_apply_errors_total.
//
// It is a Prometheus label value, not a Kubernetes name: it must be
// low-cardinality and stable across reconciles, never derived from a per-owner
// value such as the owning custom resource's name. When unset, the resource is
// labelled `rolebinding`. Build rejects a blank identifier.
func (b *Builder) WithMetricsIdentifier(identifier string) *Builder {
	b.base.WithMetricsIdentifier(identifier)
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

// ExtractInto declares that this RoleBinding produces the value of cell. fn
// computes the value from a copy of the reconciled RoleBinding; the framework
// stores it in the cell and marks it present, immediately after the RoleBinding
// is applied or fetched. Extracting several values means several ExtractInto
// calls, one per cell. This is a package-level function because Go methods
// cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(rbacv1.RoleBinding) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
