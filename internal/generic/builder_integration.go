//nolint:dupl
package generic

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntegrationBuilder configures a generic internal integration resource for Kubernetes primitives
// such as Services, Ingresses, and Gateways.
type IntegrationBuilder[T client.Object, M MutatorApplier] struct {
	BaseBuilder[T, M]
	res *IntegrationResource[T, M]
}

// NewIntegrationBuilder creates a new generic integration builder.
//
// The provided object is treated as the desired base state. The mutator factory is used to
// construct the typed mutator during Mutate.
func NewIntegrationBuilder[T client.Object, M MutatorApplier](
	obj T,
	identityFunc func(T) string,
	defaultApplicator FieldApplicator[T],
	newMutator func(T) M,
) *IntegrationBuilder[T, M] {
	res := &IntegrationResource[T, M]{}
	b := &IntegrationBuilder[T, M]{
		res: res,
	}
	b.InitBase(obj, identityFunc, defaultApplicator, newMutator)
	b.res.BaseResource = *b.BaseRes
	return b
}

// WithMutation registers a typed feature mutation for the integration.
func (b *IntegrationBuilder[T, M]) WithMutation(
	m Mutation[M],
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithMutation(m)
	return b
}

// WithCustomFieldApplicator overrides the default baseline field applicator.
func (b *IntegrationBuilder[T, M]) WithCustomFieldApplicator(
	applicator FieldApplicator[T],
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
func (b *IntegrationBuilder[T, M]) WithFieldApplicationFlavor(
	flavor FieldApplicationFlavor[T],
) *IntegrationBuilder[T, M] {
	b.BaseBuilder.WithFieldApplicationFlavor(flavor)
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
func (b *IntegrationBuilder[T, M]) Build() (*IntegrationResource[T, M], error) {
	b.res.BaseResource = *b.BaseRes
	if err := b.ValidateBase(); err != nil {
		return nil, err
	}
	return b.res, nil
}
