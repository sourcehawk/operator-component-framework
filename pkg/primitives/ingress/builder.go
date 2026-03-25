package ingress

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
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
		WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
		WithCustomSuspendMutation(DefaultSuspendMutationHandler).
		WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

	return &Builder{
		base: base,
	}
}

// WithMutation registers a mutation for the Ingress.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// A mutation with a nil Feature is applied unconditionally; one with a non-nil
// Feature is applied only when that feature is enabled.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
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

// WithDataExtractor registers a function to read values from the Ingress after
// it has been successfully reconciled.
//
// The extractor receives a value copy of the reconciled Ingress. This is useful
// for surfacing generated or updated entries (such as assigned load balancer
// addresses) to other components or resources.
//
// A nil extractor is ignored.
func (b *Builder) WithDataExtractor(extractor func(networkingv1.Ingress) error) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(ing *networkingv1.Ingress) error {
			return extractor(*ing)
		})
	}
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
