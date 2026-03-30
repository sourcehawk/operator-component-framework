// Package statefulset provides a builder and resource for managing Kubernetes StatefulSets.
package statefulset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	appsv1 "k8s.io/api/apps/v1"
)

// Builder is a configuration helper for creating and customizing a StatefulSet Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.WorkloadBuilder[*appsv1.StatefulSet, *Mutator]
}

// NewBuilder initializes a new Builder with the provided StatefulSet object.
//
// The StatefulSet object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided StatefulSet must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(statefulset *appsv1.StatefulSet) *Builder {
	identityFunc := func(s *appsv1.StatefulSet) string {
		return fmt.Sprintf("apps/v1/StatefulSet/%s/%s", s.Namespace, s.Name)
	}

	base := generic.NewWorkloadBuilder[*appsv1.StatefulSet, *Mutator](
		statefulset,
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

// WithMutation registers a feature-based mutation for the StatefulSet.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to inject environment variables,
// arguments, or other configuration into the StatefulSet's containers.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithCustomConvergeStatus overrides the default logic for determining if the
// StatefulSet has reached its desired state.
//
// The default behavior uses DefaultConvergingStatusHandler, which considers a
// StatefulSet ready when its ReadyReplicas count matches the desired replica count.
func (b *Builder) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, *appsv1.StatefulSet) (concepts.AliveStatusWithReason, error),
) *Builder {
	b.base.WithCustomConvergeStatus(handler)
	return b
}

// WithCustomGraceStatus overrides how the StatefulSet reports its health while
// it is still converging.
//
// The default behavior uses DefaultGraceStatusHandler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*appsv1.StatefulSet) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which reports the
// progress of scaling down to zero replicas.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*appsv1.StatefulSet) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the StatefulSet should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which scales the
// StatefulSet to zero replicas.
func (b *Builder) WithCustomSuspendMutation(
	handler func(*Mutator) error,
) *Builder {
	b.base.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the decision of whether to delete
// the StatefulSet when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which does not
// delete StatefulSets during suspension (only scaled down).
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*appsv1.StatefulSet) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the StatefulSet
// is applied during reconciliation. If the guard returns Blocked, the StatefulSet and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(
	guard func(appsv1.StatefulSet) (concepts.GuardStatusWithReason, error),
) *Builder {
	if guard == nil {
		b.base.WithGuard(nil)
		return b
	}
	b.base.WithGuard(func(s *appsv1.StatefulSet) (concepts.GuardStatusWithReason, error) {
		return guard(*s)
	})
	return b
}

// WithDataExtractor registers a function to harvest information from the
// StatefulSet after it has been successfully reconciled.
//
// This is useful for capturing auto-generated fields and making them available
// to other components or resources via the framework's data extraction mechanism.
func (b *Builder) WithDataExtractor(
	extractor func(appsv1.StatefulSet) error,
) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(s *appsv1.StatefulSet) error {
			return extractor(*s)
		})
	}
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It ensures that:
//   - A base StatefulSet object was provided.
//   - The StatefulSet has both a name and a namespace set.
//
// If validation fails, an error is returned and the Resource should not be used.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
