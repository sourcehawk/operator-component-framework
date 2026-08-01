// Package pod provides a builder and resource for managing Kubernetes Pods.
package pod

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a Pod Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// declared data extractions. This builder ensures that the resulting Resource is
// properly initialized and validated before use in a reconciliation loop.
type Builder struct {
	base *generic.WorkloadBuilder[*corev1.Pod, *Mutator]
}

// NewBuilder initializes a new Builder with the provided Pod object.
//
// The Pod object passed here serves as the "desired base state". During
// reconciliation, the Resource will attempt to make the cluster's state match
// this base state, modified by any registered mutations.
//
// The provided pod must have at least a Name and Namespace set, which
// is validated during the Build() call.
func NewBuilder(pod *corev1.Pod) *Builder {
	identityFunc := func(p *corev1.Pod) string {
		return fmt.Sprintf("v1/Pod/%s/%s", p.Namespace, p.Name)
	}

	base := generic.NewWorkloadBuilder[*corev1.Pod, *Mutator](
		pod,
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

// WithMutation registers one or more feature-based mutations for the Pod.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to inject environment variables,
// arguments, or other configuration into the Pod's containers.
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
// Pod has reached its desired state.
//
// The default behavior uses DefaultConvergingStatusHandler, which checks the Pod's
// phase and container statuses. Use this method if your Pod requires more complex
// health checks.
//
// If you want to augment the default behavior, you can call DefaultConvergingStatusHandler
// within your custom handler.
func (b *Builder) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, *corev1.Pod) (concepts.AliveStatusWithReason, error),
) *Builder {
	b.base.WithCustomConvergeStatus(handler)
	return b
}

// WithCustomGraceStatus overrides how the Pod reports its health while
// it has not yet reached full readiness.
//
// The default behavior uses DefaultGraceStatusHandler.
//
// If you want to augment the default behavior, you can call DefaultGraceStatusHandler
// within your custom handler.
func (b *Builder) WithCustomGraceStatus(
	handler func(*corev1.Pod) (concepts.GraceStatusWithReason, error),
) *Builder {
	b.base.WithCustomGraceStatus(handler)
	return b
}

// WithCustomSuspendStatus overrides how the progress of suspension is reported.
//
// The default behavior uses DefaultSuspensionStatusHandler, which always reports
// Suspended because pods are deleted on suspend.
//
// If you want to augment the default behavior, you can call DefaultSuspensionStatusHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendStatus(
	handler func(*corev1.Pod) (concepts.SuspensionStatusWithReason, error),
) *Builder {
	b.base.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation defines how the Pod should be modified when
// the component is suspended.
//
// The default behavior uses DefaultSuspendMutationHandler, which is a no-op
// because pods are deleted on suspend rather than mutated.
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
// the Pod when the component is suspended.
//
// The default behavior uses DefaultDeleteOnSuspendHandler, which returns true
// because pods cannot be paused. Return false from this handler if you want
// the Pod to remain in the cluster when suspended.
//
// If you want to augment the default behavior, you can call DefaultDeleteOnSuspendHandler
// within your custom handler.
func (b *Builder) WithCustomSuspendDeletionDecision(
	handler func(*corev1.Pod) bool,
) *Builder {
	b.base.WithCustomSuspendDeletionDecision(handler)
	return b
}

// WithGuard registers a guard precondition that is evaluated before the Pod
// is applied during reconciliation. If the guard returns Blocked, the Pod and
// all resources registered after it are skipped until the guard clears.
// Passing nil clears any previously registered guard.
func (b *Builder) WithGuard(
	guard func(corev1.Pod) (concepts.GuardStatusWithReason, error),
) *Builder {
	b.base.WithGuard(generic.WrapGuard(guard))
	return b
}

// WithDataGuard declares that the Pod reads the given data cells and
// must not be applied until every one of them is set. The framework generates
// the guard and its reason (waiting for data "<name>"), and component Build
// validates that a producer for each cell is registered earlier. Data guards
// are evaluated before any custom guard registered with WithGuard.
func (b *Builder) WithDataGuard(cells ...concepts.DataCell) *Builder {
	b.base.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares that the Pod reads the given data cells
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
//   - A base Pod object was provided.
//   - The Pod has both a name and a namespace set.
//
// If validation fails, an error is returned and the Resource should not be used.
func (b *Builder) Build() (*Resource, error) {
	genericRes, err := b.base.Build()
	if err != nil {
		return nil, err
	}
	return &Resource{base: genericRes}, nil
}

// ExtractInto declares that this Pod produces the value of cell. fn
// computes the value from a copy of the reconciled Pod; the framework
// stores it in the cell and marks it present, immediately after the Pod
// is applied or fetched. Extracting several values means several ExtractInto
// calls, one per cell. This is a package-level function because Go methods
// cannot introduce the extra type parameter V.
func ExtractInto[V any](b *Builder, cell *concepts.Data[V], fn func(corev1.Pod) (V, error)) {
	generic.ExtractInto(&b.base.BaseBuilder, cell, generic.WrapExtraction(fn))
}
