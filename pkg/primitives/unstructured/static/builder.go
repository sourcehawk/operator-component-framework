package static

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Builder is a configuration helper for creating and customizing a static
// unstructured Resource.
//
// It provides a fluent API for registering mutations and declared data extractions.
// Build() validates the configuration and returns an initialized Resource
// ready for use in a reconciliation loop.
type Builder struct {
	base         *generic.StaticBuilder[*uns.Unstructured, *unstruct.Mutator]
	clusterScope bool
}

// NewBuilder initializes a new Builder with the provided unstructured object.
//
// The object serves as the desired base state. During reconciliation the
// Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// The provided object must have a Name set. Namespaced resources must also
// have a Namespace; cluster-scoped resources must call MarkClusterScoped.
func NewBuilder(obj *uns.Unstructured) *Builder {
	// Identity function is set at Build() time once we know the cluster-scope flag.
	placeholder := func(_ *uns.Unstructured) string { return "" }

	return &Builder{
		base: generic.NewStaticBuilder[*uns.Unstructured, *unstruct.Mutator](
			obj,
			placeholder,
			unstruct.NewMutator,
		),
	}
}

// MarkClusterScoped marks the resource as cluster-scoped. Build() will reject
// a non-empty namespace instead of requiring one.
func (b *Builder) MarkClusterScoped() *Builder {
	b.clusterScope = true
	b.base.MarkClusterScoped()
	return b
}

// WithMutation registers one or more mutations for the unstructured object.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(ms ...unstruct.Mutation) *Builder {
	for _, m := range ms {
		b.base.WithMutation(feature.Mutation[*unstruct.Mutator](m))
	}
	return b
}

// WithGuard registers a guard precondition that is evaluated before the object
// is applied during reconciliation. If the guard returns Blocked, the object and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(uns.Unstructured) (concepts.GuardStatusWithReason, error)) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the unstructured object reads the given data
// cells and must not be applied until every one of them is set. The framework
// generates the guard and its reason (waiting for data "<name>"), and
// component Build validates that a producer for each cell is registered
// earlier. Data guards are evaluated before any custom guard registered with
// WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the unstructured object reads the given data
// cells without gating on them. Component Build still validates that a
// producer is registered earlier, and the dependency stays visible to
// introspection. Consumers in this mode use Get and skip quietly when a cell
// is absent.
func (b *Builder) WithOptionalData(cells ...concepts.DataCell) *Builder {
	b.base.WithOptionalData(cells...)
	return b
}

// WithMetricsIdentifier sets the object's identifier for resource-level
// metrics, used as the value of the `resource` label on ocf_resource_apply_total
// and ocf_resource_apply_errors_total.
//
// It is a Prometheus label value, not a Kubernetes name: it must be
// low-cardinality and stable across reconciles, never derived from a per-owner
// value such as the owning custom resource's name. When unset, the resource is
// labelled with its lowercased kind. Build rejects a blank identifier.
func (b *Builder) WithMetricsIdentifier(identifier string) *Builder {
	b.base.WithMetricsIdentifier(identifier)
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No object was provided.
//   - The object is missing a Name.
//   - A namespaced object is missing a Namespace (and MarkClusterScoped was not called).
//   - A cluster-scoped object has a Namespace set.
func (b *Builder) Build() (*Resource, error) {
	b.base.BaseRes.IdentityFunc = unstruct.MakeIdentityFunc(b.clusterScope)
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}

// ExtractInto declares that this unstructured object produces the value of
// cell. fn computes the value from a copy of the reconciled object; the
// framework stores it in the cell and marks it present, immediately after the
// object is applied or fetched. Extracting several values means several
// ExtractInto calls, one per cell. This is a package-level function because Go
// methods cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(uns.Unstructured) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
