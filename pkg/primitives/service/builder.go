package service

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a Service Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// declared data extractions. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.IntegrationBuilder[*corev1.Service, *Mutator]
}

// NewBuilder initializes a new Builder with the provided Service object.
//
// The Service object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided Service must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(svc *corev1.Service) *Builder {
	identityFunc := func(s *corev1.Service) string {
		return fmt.Sprintf("v1/Service/%s/%s", s.Namespace, s.Name)
	}

	base := generic.NewIntegrationBuilder[*corev1.Service, *Mutator](
		svc,
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

// WithMutation registers one or more feature-based mutations for the Service.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to modify ports, selectors, or other
// service configuration.
//
// Since mutations are often version-gated, the provided feature.Mutation
// should contain the logic to determine if and how the mutation is applied
// based on the component's current version or configuration.
func (b *Builder) WithMutation(ms ...Mutation) *Builder {
	for _, m := range ms {
		b.base.WithMutation(feature.Mutation[*Mutator](m))
	}
	return b
}

// WithCustomOperationalStatus overrides the default logic for determining if the
// Service is operational.
//
// The default behavior uses DefaultOperationalStatusHandler, which considers
// LoadBalancer services pending until an ingress IP or hostname is assigned,
// and all other service types immediately operational.
//
// If you want to augment the default behavior, you can call DefaultOperationalStatusHandler
// within your custom handler.
func (b *Builder) WithCustomOperationalStatus(
	handler func(concepts.ConvergingOperation, *corev1.Service) (concepts.OperationalStatusWithReason, error),
) *Builder {
	b.base.WithCustomOperationalStatus(handler)
	return b
}

// WithCustomGraceStatus overrides the default logic for assessing the health of
// the Service when the component's grace period has expired.
//
// The default behavior uses DefaultGraceStatusHandler, which considers
// LoadBalancer services degraded until an ingress IP or hostname is assigned,
// and all other service types immediately healthy.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*corev1.Service) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which always reports
// Suspended because the default behaviour leaves the Service untouched.
//
// If you want to augment the default behavior, you can call DefaultSuspensionStatusHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*corev1.Service) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the Service should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which is a no-op
// because the default behaviour leaves the Service unaffected by suspension.
//
// If you want to augment the default behavior, you can call DefaultSuspendMutationHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendMutation(
	handler func(*Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the decision of whether to delete
// the Service when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which returns false
// because Services are stateless routing objects that are safe to leave in place.
//
// If you want to augment the default behavior, you can call DefaultDeleteOnSuspendHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*corev1.Service) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the Service
// is applied during reconciliation. If the guard returns Blocked, the Service and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(
	guard func(corev1.Service) (concepts.GuardStatusWithReason, error),
) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the Service reads the given data cells and
// must not be applied until every one of them is set. The framework generates
// the guard and its reason (waiting for data "<name>"), and component Build
// validates that a producer for each cell is registered earlier. Data guards
// are evaluated before any custom guard registered with WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the Service reads the given data cells
// without gating on them. Component Build still validates that a producer is
// registered earlier, and the dependency stays visible to introspection.
// Consumers in this mode use Get and skip quietly when a cell is absent.
func (b *Builder) WithOptionalData(cells ...concepts.DataCell) *Builder {
	b.base.WithOptionalData(cells...)
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It ensures that:
//   - A base Service object was provided.
//   - The Service has both a name and a namespace set.
//
// If validation fails, an error is returned and the Resource should not be used.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}

// ExtractInto declares that this Service produces the value of cell. fn
// computes the value from a copy of the reconciled Service; the framework
// stores it in the cell and marks it present, immediately after the Service
// is applied or fetched. Extracting several values means several ExtractInto
// calls, one per cell. This is a package-level function because Go methods
// cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(corev1.Service) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
