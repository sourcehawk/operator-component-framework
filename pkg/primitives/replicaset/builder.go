// Package replicaset provides a builder and resource for managing Kubernetes ReplicaSets.
package replicaset

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	appsv1 "k8s.io/api/apps/v1"
)

// Builder is a configuration helper for creating and customizing a ReplicaSet Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.WorkloadBuilder[*appsv1.ReplicaSet, *Mutator]
}

// NewBuilder initializes a new Builder with the provided ReplicaSet object.
//
// The ReplicaSet object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided replicaset must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(replicaset *appsv1.ReplicaSet) *Builder {
	identityFunc := func(rs *appsv1.ReplicaSet) string {
		return fmt.Sprintf("apps/v1/ReplicaSet/%s/%s", rs.Namespace, rs.Name)
	}

	base := generic.NewWorkloadBuilder[*appsv1.ReplicaSet, *Mutator](
		replicaset,
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

// WithMutation registers one or more feature-based mutations for the ReplicaSet.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to inject environment variables,
// arguments, or other configuration into the ReplicaSet's containers.
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

// WithCustomConvergeStatus overrides the default logic for determining if the
// ReplicaSet has reached its desired state.
//
// The default behavior uses DefaultConvergingStatusHandler, which considers a
// ReplicaSet ready when its ReadyReplicas count matches the desired replica count.
// Use this method if your ReplicaSet requires more complex health checks.
//
// If you want to augment the default behavior, you can call DefaultConvergingStatusHandler
// within your custom handler.
func (b *Builder) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, *appsv1.ReplicaSet) (concepts.AliveStatusWithReason, error),
) *Builder {
	b.base.WithCustomConvergeStatus(handler)
	return b
}

// WithCustomGraceStatus overrides how the ReplicaSet reports its health while
// it is still converging.
//
// The default behavior uses DefaultGraceStatusHandler.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*appsv1.ReplicaSet) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which reports the
// progress of scaling down to zero replicas.
//
// If you want to augment the default behavior, you can call DefaultSuspensionStatusHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*appsv1.ReplicaSet) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the ReplicaSet should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which scales the
// ReplicaSet to zero replicas.
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
// the ReplicaSet when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which does not
// delete ReplicaSets during suspension (only scaled down). Return true from
// this handler if you want the ReplicaSet to be completely removed from the
// cluster when suspended.
//
// If you want to augment the default behavior, you can call DefaultDeleteOnSuspendHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*appsv1.ReplicaSet) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the ReplicaSet
// is applied during reconciliation. If the guard returns Blocked, the ReplicaSet and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(
	guard func(appsv1.ReplicaSet) (concepts.GuardStatusWithReason, error),
) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataExtractor registers a function to harvest information from the
// ReplicaSet after it has been successfully reconciled.
//
// This is useful for capturing auto-generated fields (like names or assigned
// IPs) and making them available to other components or resources via the
// framework's data extraction mechanism.
func (b *Builder) WithDataExtractor(
	extractor func(appsv1.ReplicaSet) error,
) *Builder {
	b.base.WithDataExtractor(generic.WrapExtractor(extractor))
	return b
}

// Build validates the configuration and returns the initialized Resource.
//
// It ensures that:
//   - A base ReplicaSet object was provided.
//   - The ReplicaSet has both a name and a namespace set.
//
// If validation fails, an error is returned and the Resource should not be used.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}
