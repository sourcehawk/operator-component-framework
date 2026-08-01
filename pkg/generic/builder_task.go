package generic

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TaskBuilder configures a generic internal task resource for Kubernetes primitives
// that run to completion, such as Jobs.
//
// It captures the common framework concepts while leaving kind-specific defaults and wrappers
// to the concrete task packages.
type TaskBuilder[T client.Object, M FeatureMutator] struct {
	BaseBuilder[T, M]
	res *TaskResource[T, M]
}

// NewTaskBuilder creates a new generic task builder.
//
// The provided object is treated as the desired base state. The mutator factory is used to
// construct the typed mutator during Mutate.
func NewTaskBuilder[T client.Object, M FeatureMutator](
	obj T,
	identityFunc func(T) string,
	newMutator func(T) M,
) *TaskBuilder[T, M] {
	res := &TaskResource[T, M]{}
	b := &TaskBuilder[T, M]{
		res: res,
	}
	b.InitBase(obj, identityFunc, newMutator)
	b.res.BaseResource = *b.BaseRes
	return b
}

// WithMutation registers one or more typed feature mutations for the task,
// in the order provided.
func (b *TaskBuilder[T, M]) WithMutation(
	ms ...Mutation[M],
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithMutation(ms...)
	return b
}

// WithGuard registers a guard precondition for the task resource.
func (b *TaskBuilder[T, M]) WithGuard(
	handler func(T) (concepts.GuardStatusWithReason, error),
) *TaskBuilder[T, M] {
	b.BaseBuilder.WithGuard(handler)
	return b
}

// WithDataGuard declares blocking data reads for the task resource. See
// BaseBuilder.WithDataGuard.
func (b *TaskBuilder[T, M]) WithDataGuard(cells ...concepts.DataCell) *TaskBuilder[T, M] {
	b.BaseBuilder.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares non-blocking data reads for the task resource.
// See BaseBuilder.WithOptionalData.
func (b *TaskBuilder[T, M]) WithOptionalData(cells ...concepts.DataCell) *TaskBuilder[T, M] {
	b.BaseBuilder.WithOptionalData(cells...)
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
//
// It returns an error if the converging status handler has not been set.
func (b *TaskBuilder[T, M]) Build() (*TaskResource[T, M], error) {
	b.res.BaseResource = *b.BaseRes
	if err := b.ValidateBase(); err != nil {
		return nil, err
	}
	if b.res.ConvergingStatusHandler == nil {
		return nil, errors.New("converging status handler is required")
	}
	return b.res, nil
}
