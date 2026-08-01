package workload

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Builder is a configuration helper for creating and customizing an unstructured
// workload Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// declared data extractions. The converging status handler is required; all other
// handlers default to safe no-ops when omitted.
type Builder struct {
	base         *generic.WorkloadBuilder[*uns.Unstructured, *unstruct.Mutator]
	clusterScope bool
}

// NewBuilder initializes a new Builder with the provided unstructured object.
//
// The object serves as the desired base state. The converging status handler
// must be set via WithCustomConvergeStatus before Build(). All other handlers
// are optional.
func NewBuilder(obj *uns.Unstructured) *Builder {
	placeholder := func(_ *uns.Unstructured) string { return "" }

	return &Builder{
		base: generic.NewWorkloadBuilder[*uns.Unstructured, *unstruct.Mutator](
			obj,
			placeholder,
			unstruct.NewMutator,
		),
	}
}

// MarkClusterScoped marks the resource as cluster-scoped.
func (b *Builder) MarkClusterScoped() *Builder {
	b.clusterScope = true
	b.base.MarkClusterScoped()
	return b
}

// WithMutation registers one or more mutations for the unstructured object.
func (b *Builder) WithMutation(ms ...unstruct.Mutation) *Builder {
	for _, m := range ms {
		b.base.WithMutation(feature.Mutation[*unstruct.Mutator](m))
	}
	return b
}

// WithCustomConvergeStatus sets the handler that evaluates whether the resource
// has reached its desired state. This handler is required.
func (b *Builder) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, *uns.Unstructured) (concepts.AliveStatusWithReason, error),
) *Builder {
	b.base.WithCustomConvergeStatus(handler)
	return b
}

// WithCustomGraceStatus overrides the default grace status handler that assesses
// health during rollouts. The default reports Healthy.
func (b *Builder) WithCustomGraceStatus(
	handler func(*uns.Unstructured) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides the default suspension status handler.
// The default reports Suspended immediately.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*uns.Unstructured) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation overrides the default suspension mutation handler.
// The default is a no-op.
func (b *Builder) WithCustomSuspendMutation(
	handler func(*unstruct.Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the default delete-on-suspend
// decision. The default returns false (keep the resource).
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*uns.Unstructured) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
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

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if the converging status handler has not been set.
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
