package hpa

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
)

// Builder is a configuration helper for creating and customizing an HPA Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.IntegrationBuilder[*autoscalingv2.HorizontalPodAutoscaler, *Mutator]
}

// NewBuilder initializes a new Builder with the provided HorizontalPodAutoscaler object.
//
// The HPA object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided HPA must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(hpa *autoscalingv2.HorizontalPodAutoscaler) *Builder {
	identityFunc := func(h *autoscalingv2.HorizontalPodAutoscaler) string {
		return fmt.Sprintf("autoscaling/v2/HorizontalPodAutoscaler/%s/%s", h.Namespace, h.Name)
	}

	base := generic.NewIntegrationBuilder[*autoscalingv2.HorizontalPodAutoscaler, *Mutator](
		hpa,
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

// WithMutation registers one or more feature-based mutations for the HPA.
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

// WithCustomOperationalStatus overrides the default logic for determining the
// HPA's operational status.
//
// The default behavior uses DefaultOperationalStatusHandler, which inspects
// HPA conditions (ScalingActive, AbleToScale) to determine status.
func (b *Builder) WithCustomOperationalStatus(
	handler func(concepts.ConvergingOperation, *autoscalingv2.HorizontalPodAutoscaler) (concepts.OperationalStatusWithReason, error),
) *Builder {
	b.base.WithCustomOperationalStatus(handler)
	return b
}

// WithCustomGraceStatus overrides the default logic for determining the
// HPA's grace status after the allowed grace period has expired.
//
// The default behavior uses DefaultGraceStatusHandler, which inspects HPA
// conditions (ScalingActive, AbleToScale) to determine health.
func (b *Builder) WithCustomGraceStatus(
	handler func(*autoscalingv2.HorizontalPodAutoscaler) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which reports
// Suspended immediately because deletion is handled by the framework.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*autoscalingv2.HorizontalPodAutoscaler) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the HPA should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which is a no-op
// since the HPA is deleted on suspend (no spec mutations are needed).
func (b *Builder) WithCustomSuspendMutation(
	handler func(*Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the decision of whether to delete
// the HPA when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which returns true.
// The HPA is deleted to prevent the Kubernetes HPA controller from scaling the
// target back up during suspension. On resume the framework recreates the HPA.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*autoscalingv2.HorizontalPodAutoscaler) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the HPA
// is applied during reconciliation. If the guard returns Blocked, the HPA and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(
	guard func(autoscalingv2.HorizontalPodAutoscaler) (concepts.GuardStatusWithReason, error),
) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataExtractor registers a function to read values from the HPA after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled HPA. This is useful
// for surfacing generated or updated fields to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(
	extractor func(autoscalingv2.HorizontalPodAutoscaler) error,
) *Builder {
	b.base.WithDataExtractor(generic.WrapExtractor(extractor))
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It returns an error if:
//   - No HPA object was provided.
//   - The HPA is missing a Name or Namespace.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
