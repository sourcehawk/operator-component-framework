package generic

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntegrationBuilder configures a generic internal integration resource for Kubernetes primitives
// such as Services, Ingresses, and Gateways.
//
// It captures the common framework concepts while leaving kind-specific defaults and wrappers
// to the concrete integration packages.
type IntegrationBuilder[T client.Object, M MutatorApplier] struct {
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
	return &IntegrationBuilder[T, M]{
		res: &IntegrationResource[T, M]{
			DesiredObject:          obj,
			IdentityFunc:           identityFunc,
			DefaultFieldApplicator: defaultApplicator,
			NewMutator:             newMutator,
		},
	}
}

// WithMutation registers a typed feature mutation for the integration.
func (b *IntegrationBuilder[T, M]) WithMutation(
	m feature.Mutation[M],
) *IntegrationBuilder[T, M] {
	b.res.Mutations = append(b.res.Mutations, m)
	return b
}

// WithCustomFieldApplicator overrides the default baseline field applicator.
func (b *IntegrationBuilder[T, M]) WithCustomFieldApplicator(
	applicator FieldApplicator[T],
) *IntegrationBuilder[T, M] {
	b.res.CustomFieldApplicator = applicator
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
func (b *IntegrationBuilder[T, M]) WithFieldApplicationFlavor(
	flavor FieldApplicationFlavor[T],
) *IntegrationBuilder[T, M] {
	if flavor != nil {
		b.res.FieldFlavors = append(b.res.FieldFlavors, flavor)
	}
	return b
}

// WithDataExtractor registers a typed data extractor to run after successful reconciliation.
func (b *IntegrationBuilder[T, M]) WithDataExtractor(
	extractor func(T) error,
) *IntegrationBuilder[T, M] {
	if extractor != nil {
		b.res.DataExtractors = append(b.res.DataExtractors, extractor)
	}
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
	b.res.SuspendStatusHandler = handler
	return b
}

// WithCustomSuspendMutation overrides the integration suspension mutation handler.
func (b *IntegrationBuilder[T, M]) WithCustomSuspendMutation(
	handler func(M) error,
) *IntegrationBuilder[T, M] {
	b.res.SuspendMutationHandler = handler
	return b
}

// WithCustomSuspendDeletionDecision overrides the integration delete-on-suspend decision handler.
func (b *IntegrationBuilder[T, M]) WithCustomSuspendDeletionDecision(
	handler func(T) bool,
) *IntegrationBuilder[T, M] {
	b.res.DeleteOnSuspendHandler = handler
	return b
}

// Build validates the integration builder configuration and returns the initialized resource.
func (b *IntegrationBuilder[T, M]) Build() (*IntegrationResource[T, M], error) {
	if isNil(b.res.DesiredObject) {
		return nil, errors.New("object cannot be nil")
	}

	if b.res.DesiredObject.GetName() == "" {
		return nil, errors.New("object name cannot be empty")
	}

	if b.res.DesiredObject.GetNamespace() == "" {
		return nil, errors.New("object namespace cannot be empty")
	}

	if b.res.IdentityFunc == nil {
		return nil, errors.New("identity function cannot be nil")
	}

	if b.res.DefaultFieldApplicator == nil {
		return nil, errors.New("default field applicator cannot be nil")
	}

	if b.res.NewMutator == nil {
		return nil, errors.New("mutator factory cannot be nil")
	}

	return b.res, nil
}
