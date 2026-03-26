// Package daemonset provides a builder and resource for managing Kubernetes DaemonSets.
package daemonset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	appsv1 "k8s.io/api/apps/v1"
)

// Builder is a configuration helper for creating and customizing a DaemonSet Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.WorkloadBuilder[*appsv1.DaemonSet, *Mutator]
}

// NewBuilder initializes a new Builder with the provided DaemonSet object.
//
// The DaemonSet object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided DaemonSet must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(daemonset *appsv1.DaemonSet) *Builder {
	identityFunc := func(d *appsv1.DaemonSet) string {
		return fmt.Sprintf("apps/v1/DaemonSet/%s/%s", d.Namespace, d.Name)
	}

	base := generic.NewWorkloadBuilder[*appsv1.DaemonSet, *Mutator](
		daemonset,
		identityFunc,
		NewMutator,
	)

	base.
		WithCustomConvergeStatus(DefaultConvergingStatusHandler).
		WithCustomGraceStatus(DefaultGraceStatusHandler).
		WithCustomSuspendStatus(DefaultSuspensionStatusHandler).
		WithCustomSuspendMutation(DefaultSuspendMutationHandler).
		WithCustomSuspendDeletionDecision(DefaultDeleteOnSuspendHandler)

	return &Builder{
		base: base,
	}
}

// WithMutation registers a feature-based mutation for the DaemonSet.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to inject environment variables,
// arguments, or other configuration into the DaemonSet's containers.
//
// Since mutations are often version-gated, the provided feature.Mutation
// should contain the logic to determine if and how the mutation is applied
// based on the component's current version or configuration.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithCustomConvergeStatus overrides the default logic for determining if the
// DaemonSet has reached its desired state.
//
// The default behavior uses DefaultConvergingStatusHandler, which:
//   - Treats the DaemonSet as converged when DesiredNumberScheduled == 0 as soon as
//     status.ObservedGeneration is greater than or equal to metadata.Generation.
//   - When DesiredNumberScheduled > 0, treats the DaemonSet as converged once
//     status.NumberReady is greater than or equal to status.DesiredNumberScheduled.
//
// If you want to augment the default behavior, you can call DefaultConvergingStatusHandler
// within your custom handler.
func (b *Builder) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, *appsv1.DaemonSet) (concepts.AliveStatusWithReason, error),
) *Builder {
	b.base.WithCustomConvergeStatus(handler)
	return b
}

// WithCustomGraceStatus overrides how the DaemonSet reports its health while
// it is still converging.
//
// The default behavior uses DefaultGraceStatusHandler.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*appsv1.DaemonSet) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which always reports
// Suspended because DaemonSets are deleted on suspension.
//
// If you want to augment the default behavior, you can call DefaultSuspensionStatusHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*appsv1.DaemonSet) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the DaemonSet should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which is a no-op
// because DaemonSets are deleted on suspension rather than mutated.
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
// the DaemonSet when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which returns true
// because DaemonSets have no replicas field and cannot be scaled down.
//
// If you want to augment the default behavior, you can call DefaultDeleteOnSuspendHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*appsv1.DaemonSet) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithDataExtractor registers a function to harvest information from the
// DaemonSet after it has been successfully reconciled.
//
// This is useful for capturing auto-generated fields (like names or assigned
// IPs) and making them available to other components or resources via the
// framework's data extraction mechanism.
func (b *Builder) WithDataExtractor(
	extractor func(appsv1.DaemonSet) error,
) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(d *appsv1.DaemonSet) error {
			if d == nil {
				return extractor(appsv1.DaemonSet{})
			}
			return extractor(*d.DeepCopy())
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It ensures that:
//   - A base DaemonSet object was provided.
//   - The DaemonSet has both a name and a namespace set.
//
// If validation fails, an error is returned and the Resource should not be used.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
