package service

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a Service Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
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
		DefaultFieldApplicator,
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

// WithMutation registers a feature-based mutation for the Service.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to modify ports, selectors, or other
// service configuration.
//
// Since mutations are often version-gated, the provided feature.Mutation
// should contain the logic to determine if and how the mutation is applied
// based on the component's current version or configuration.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithCustomFieldApplicator sets a custom strategy for applying the desired
// state to the existing Service in the cluster.
//
// The default applicator (DefaultFieldApplicator) replaces the current object
// with a deep copy of the desired object while preserving Kubernetes-assigned
// immutable fields (spec.clusterIP and spec.clusterIPs).
//
// The applicator receives the current object from the API server and the desired
// object from the Resource, and is responsible for merging the desired changes
// into the current object.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current, desired *corev1.Service) error,
) *Builder {
	b.base.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
//
// Flavors run after the baseline applicator (default or custom) in registration
// order. They are typically used to preserve fields from the live cluster object
// that should not be overwritten by the desired state.
//
// A nil flavor is ignored.
func (b *Builder) WithFieldApplicationFlavor(flavor FieldApplicationFlavor) *Builder {
	b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*corev1.Service](flavor))
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

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which always reports
// Suspended because Services are deleted on suspend.
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
// because Services are deleted on suspend rather than mutated.
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
// The default behavior uses DefaultDeleteOnSuspendHandler, which returns true
// because Services have no meaningful "scaled down" state.
//
// If you want to augment the default behavior, you can call DefaultDeleteOnSuspendHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*corev1.Service) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithDataExtractor registers a function to harvest information from the
// Service after it has been successfully reconciled.
//
// This is useful for capturing auto-generated fields (like assigned ClusterIP
// or LoadBalancer ingress) and making them available to other components or
// resources via the framework's data extraction mechanism.
func (b *Builder) WithDataExtractor(
	extractor func(corev1.Service) error,
) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(s *corev1.Service) error {
			return extractor(*s)
		})
	}
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
