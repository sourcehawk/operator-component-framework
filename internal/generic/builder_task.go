//nolint:dupl
package generic

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TaskBuilder configures a generic internal task resource for Kubernetes primitives
// that run to completion, such as Jobs.
//
// It captures the common framework concepts while leaving kind-specific defaults and wrappers
// to the concrete task packages.
type TaskBuilder[T client.Object, M MutatorApplier] struct {
	BaseBuilder[T, M]
	res *TaskResource[T, M]
}

// NewTaskBuilder creates a new generic task builder.
//
// The provided object is treated as the desired base state. The mutator factory is used to
// construct the typed mutator during Mutate.
func NewTaskBuilder[T client.Object, M MutatorApplier](
	obj T,
	identityFunc func(T) string,
	defaultApplicator FieldApplicator[T],
	newMutator func(T) M,
) *TaskBuilder[T, M] {
	res := &TaskResource[T, M]{}
	b := &TaskBuilder[T, M]{
		res: res,
	}
	b.InitBase(obj, identityFunc, defaultApplicator, newMutator)
	b.res.BaseResource = *b.BaseRes
	return b
}

// WithMutation registers a typed feature mutation for the task.
func (b *TaskBuilder[T, M]) WithMutation(
	m feature.Mutation[M],
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithMutation(m)
	return b
}

// WithCustomFieldApplicator overrides the default baseline field applicator.
func (b *TaskBuilder[T, M]) WithCustomFieldApplicator(
	applicator FieldApplicator[T],
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithCustomFieldApplicator(applicator)
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
func (b *TaskBuilder[T, M]) WithFieldApplicationFlavor(
	flavor FieldApplicationFlavor[T],
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithFieldApplicationFlavor(flavor)
	return b
}

// WithDataExtractor registers a typed data extractor to run after successful reconciliation.
func (b *TaskBuilder[T, M]) WithDataExtractor(
	extractor func(T) error,
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithDataExtractor(extractor)
	return b
}

// WithCustomConvergeStatus overrides the task convergence status handler.
func (b *TaskBuilder[T, M]) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, T) (concepts.CompletionStatusWithReason, error),
) *TaskBuilder[T, M] {
	b.res.ConvergingStatusHandler = handler
	return b
}

// WithCustomSuspendStatus overrides the task suspension status handler.
func (b *TaskBuilder[T, M]) WithCustomSuspendStatus(
	handler func(T) (concepts.SuspensionStatusWithReason, error),
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation overrides the task suspension mutation handler.
func (b *TaskBuilder[T, M]) WithCustomSuspendMutation(
	handler func(M) error,
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the task delete-on-suspend decision handler.
func (b *TaskBuilder[T, M]) WithCustomSuspendDeletionDecision(
	handler func(T) bool,
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendDeletionDecision(handler)
	return b
}

// Build validates the task builder configuration and returns the initialized resource.
func (b *TaskBuilder[T, M]) Build() (*TaskResource[T, M], error) {
	b.res.BaseResource = *b.BaseRes
	if err := b.ValidateBase(); err != nil {
		return nil, err
	}
	return b.res, nil
}
