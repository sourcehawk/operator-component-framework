//nolint:dupl
package generic

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntegrationBuilder configures a generic internal integration resource for Kubernetes primitives
// such as Services, Ingresses, and Gateways.
type IntegrationBuilder[T client.Object, M FeatureMutator] struct {
	BaseBuilder[T, M]
	res *IntegrationResource[T, M]
}

// NewIntegrationBuilder creates a new generic integration builder.
//
// The provided object is treated as the desired base state. The mutator factory is used to
// construct the typed mutator during Mutate.
func NewIntegrationBuilder[T client.Object, M FeatureMutator](
	obj T,
	identityFunc func(T) string,
	newMutator func(T) M,
) *IntegrationBuilder[T, M] {
	res := &IntegrationResource[T, M]{}
	b := &IntegrationBuilder[T, M]{
		res: res,
	}
	b.InitBase(obj, identityFunc, newMutator)

	// Default grace status: report Healthy.
	b.res.GraceStatusHandler = func(_ T) (concepts.GraceStatusWithReason, error) {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusHealthy,
			Reason: "default grace status",
		}, nil
	}

	b.res.BaseResource = *b.BaseRes
	return b
}

// WithMutation registers one or more typed feature mutations for the
// integration, in the order provided.
func (b *IntegrationBuilder[T, M]) WithMutation(
	ms ...Mutation[M],
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithMutation(ms...)
	return b
}

// WithGuard registers a guard precondition for the integration resource.
func (b *IntegrationBuilder[T, M]) WithGuard(
	handler func(T) (concepts.GuardStatusWithReason, error),
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithGuard(handler)
	return b
}

// WithDataExtractor registers a typed data extractor to run after successful reconciliation.
func (b *IntegrationBuilder[T, M]) WithDataExtractor(
	extractor func(T) error,
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithDataExtractor(extractor)
	return b
}

// WithCustomOperationalStatus overrides the integration operational status handler.
func (b *IntegrationBuilder[T, M]) WithCustomOperationalStatus(
	handler func(concepts.ConvergingOperation, T) (concepts.OperationalStatusWithReason, error),
) *IntegrationBuilder[T, M] {
	b.res.OperationalStatusHandler = handler
	return b
}

// WithCustomGraceStatus overrides the integration grace status handler.
func (b *IntegrationBuilder[T, M]) WithCustomGraceStatus(
	handler func(T) (concepts.GraceStatusWithReason, error),
) *IntegrationBuilder[T, M] {
	b.res.GraceStatusHandler = handler
	return b
}

// WithCustomSuspendStatus overrides the integration suspension status handler.
func (b *IntegrationBuilder[T, M]) WithCustomSuspendStatus(
	handler func(T) (concepts.SuspensionStatusWithReason, error),
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation overrides the integration suspension mutation handler.
func (b *IntegrationBuilder[T, M]) WithCustomSuspendMutation(
	handler func(M) error,
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the integration delete-on-suspend decision handler.
func (b *IntegrationBuilder[T, M]) WithCustomSuspendDeletionDecision(
	handler func(T) bool,
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendDeletionDecision(handler)
	return b
}

// Build validates the integration builder configuration and returns the initialized resource.
//
// It returns an error if the operational status handler has not been set.
func (b *IntegrationBuilder[T, M]) Build() (*IntegrationResource[T, M], error) {
	b.res.BaseResource = *b.BaseRes
	if err := b.ValidateBase(); err != nil {
		return nil, err
	}
	if b.res.OperationalStatusHandler == nil {
		return nil, errors.New("operational status handler is required")
	}
	return b.res, nil
}
