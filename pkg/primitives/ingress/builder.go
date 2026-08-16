package ingress

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	networkingv1 "k8s.io/api/networking/v1"
)

// Builder is a configuration helper for creating and customizing an Ingress Resource.
//
// It provides a fluent API for registering mutations, status handlers, and data
// extractors. Build() validates the configuration and returns an initialized
// Resource ready for use in a reconciliation loop.
type Builder struct {
	base *generic.IntegrationBuilder[*networkingv1.Ingress, *Mutator]
}

// NewBuilder initializes a new Builder with the provided Ingress object.
//
// The Ingress object serves as the desired base state. During reconciliation
// the Resource will make the cluster's state match this base, modified by any
// registered mutations.
//
// The provided Ingress must have both Name and Namespace set, which is validated
// during the Build() call.
func NewBuilder(ing *networkingv1.Ingress) *Builder {
	identityFunc := func(i *networkingv1.Ingress) string {
		return fmt.Sprintf("networking.k8s.io/v1/Ingress/%s/%s", i.Namespace, i.Name)
	}

	base := generic.NewIntegrationBuilder[*networkingv1.Ingress, *Mutator](
		ing,
		identityFunc,
		NewMutator,
	)

	base.
		WithCustomOperationalStatus(DefaultOperationalStatusHandler).
		WithCustomGraceStatus(DefaultGraceStatusHandler).
		WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
		WithCustomSuspendMutation(DefaultSuspendMutationHandler).
		WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

	return &Builder{
		base: base,
	}
}

// WithMutation registers one or more mutations for the Ingress.
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

// WithCustomOperationalStatus overrides the default logic for determining if the
// Ingress has reached its operational state.
//
// The default behavior uses DefaultOperationalStatusHandler, which considers an
// Ingress operational when at least one IP or hostname is assigned in
// Status.LoadBalancer.Ingress. Use this method if your Ingress requires more
// complex health checks.
func (b *Builder) WithCustomOperationalStatus(
	handler func(concepts.ConvergingOperation, *networkingv1.Ingress) (concepts.OperationalStatusWithReason, error),
) *Builder {
	b.base.WithCustomOperationalStatus(handler)
	return b
}

// WithCustomGraceStatus overrides how the Ingress reports its health after the
// component's grace period has expired.
//
// The default behavior uses DefaultGraceStatusHandler.
//
// This is used to provide more granular feedback in the component's status
// about the severity of a load balancer assignment delay.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*networkingv1.Ingress) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which immediately
// reports Suspended since the default suspension is a no-op.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*networkingv1.Ingress) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the Ingress should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which is a no-op.
// Deleting an Ingress causes ingress controller churn; the recommended approach
// is to let the backend service return 502/503.
func (b *Builder) WithCustomSuspendMutation(
	handler func(*Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the decision of whether to delete
// the Ingress when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which returns false.
// Deleting an Ingress causes the ingress controller to reload its configuration,
// affecting the entire cluster's routing. Return true from this handler only if
// explicit deletion is required for your use case.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*networkingv1.Ingress) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the Ingress
// is applied during reconciliation. If the guard returns Blocked, the Ingress and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(guard func(networkingv1.Ingress) (concepts.GuardStatusWithReason, error)) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the Ingress reads the given data cells and
// must not be applied until every one of them is set. The framework generates
// the guard and its reason (waiting for data "<name>"), and component Build
// validates that a producer for each cell is registered earlier. Data guards
// are evaluated before any custom guard registered with WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the Ingress reads the given data cells
// without gating on them. Component Build still validates that a producer is
// registered earlier, and the dependency stays visible to introspection.
// Consumers in this mode use Get and skip quietly when a cell is absent.
func (b *Builder) WithOptionalData(cells ...concepts.DataCell) *Builder {
	b.base.WithOptionalData(cells...)
	return b
}

// WithMetricsIdentifier sets the Ingress's identifier for
// resource-level metrics, used as the value of the `resource` label on
// ocf_resource_apply_total and ocf_resource_apply_errors_total.
//
// It is a Prometheus label value, not a Kubernetes name: it must be
// low-cardinality and stable across reconciles, never derived from a per-owner
// value such as the owning custom resource's name. When unset, the resource is
// labelled `ingress`. Build rejects a blank identifier.
func (b *Builder) WithMetricsIdentifier(identifier string) *Builder {
	b.base.WithMetricsIdentifier(identifier)
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No Ingress object was provided.
//   - The Ingress is missing a Name or Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}

// ExtractInto declares that this Ingress produces the value of cell. fn
// computes the value from a copy of the reconciled Ingress; the framework
// stores it in the cell and marks it present, immediately after the Ingress
// is applied or fetched. Extracting several values means several ExtractInto
// calls, one per cell. This is a package-level function because Go methods
// cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(networkingv1.Ingress) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
