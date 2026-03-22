// Package pod provides a builder and resource for managing Kubernetes Pods.
package pod

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/internal/generic"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	corev1 "k8s.io/api/core/v1"
)

// Builder is a configuration helper for creating and customizing a Pod Resource.
//
// It provides a fluent API for registering mutations, status handlers, and
// data extractors. This builder ensures that the resulting Resource is
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
		DefaultFieldApplicator,
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

// WithMutation registers a feature-based mutation for the Pod.
//
// Mutations are applied sequentially during the Mutate() phase of reconciliation.
// They are typically used by Features to inject environment variables,
// arguments, or other configuration into the Pod's containers.
//
// Since mutations are often version-gated, the provided feature.Mutation
// should contain the logic to determine if and how the mutation is applied
// based on the component's current version or configuration.
func (b *Builder) WithMutation(m Mutation) *Builder {
	b.base.WithMutation(feature.Mutation[*Mutator](m))
	return b
}

// WithCustomFieldApplicator sets a custom strategy for applying the desired
// state to the existing Pod in the cluster.
//
// The default field applicator (DefaultFieldApplicator) preserves the spec
// on existing pods (since pod spec is largely immutable) and only updates
// metadata. Using a custom applicator is necessary when:
//   - Additional metadata fields need to be selectively propagated.
//   - Specific labels or annotations should be excluded from updates.
//
// The applicator function receives both the 'current' object from the API
// server and the 'desired' object from the Resource. It is responsible for
// merging the desired changes into the current object.
//
// If a custom applicator is set, it overrides the default baseline application
// logic. Post-application flavors and mutations are still applied afterward.
func (b *Builder) WithCustomFieldApplicator(
	applicator func(current *corev1.Pod, desired *corev1.Pod) error,
) *Builder {
	b.base.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a reusable post-application "flavor" for
// the Pod.
//
// Flavors are applied in the order they are registered, after the baseline field
// applicator (default or custom) has already run. They are typically used to
// preserve selected live fields from the current object that should not be
// overwritten by the desired state.
//
// If the provided flavor is nil, it is ignored.
func (b *Builder) WithFieldApplicationFlavor(flavor FieldApplicationFlavor) *Builder {
	b.base.WithFieldApplicationFlavor(generic.FieldApplicationFlavor[*corev1.Pod](flavor))
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

// WithDataExtractor registers a function to harvest information from the
// Pod after it has been successfully reconciled.
//
// This is useful for capturing auto-generated fields (like pod IP or node
// assignment) and making them available to other components or resources via
// the framework's data extraction mechanism.
func (b *Builder) WithDataExtractor(
	extractor func(corev1.Pod) error,
) *Builder {
	if extractor != nil {
		b.base.WithDataExtractor(func(p *corev1.Pod) error {
			return extractor(*p)
		})
	}
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
