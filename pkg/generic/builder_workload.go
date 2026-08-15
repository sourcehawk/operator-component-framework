//nolint:dupl
package generic

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkloadBuilder configures a generic internal workload resource for Kubernetes primitives
// such as Deployments, StatefulSets, and DaemonSets.
//
// It captures the common framework concepts while leaving kind-specific defaults and wrappers
// to the concrete workload packages.
type WorkloadBuilder[T client.Object, M FeatureMutator] struct {
	BaseBuilder[T, M]
	res *WorkloadResource[T, M]
}

// NewWorkloadBuilder creates a new generic workload builder.
//
// The provided object is treated as the desired base state. The mutator factory is used to
// construct the typed mutator during Mutate.
func NewWorkloadBuilder[T client.Object, M FeatureMutator](
	obj T,
	identityFunc func(T) string,
	newMutator func(T) M,
) *WorkloadBuilder[T, M] {
	res := &WorkloadResource[T, M]{}
	b := &WorkloadBuilder[T, M]{
		res: res,
	}
	b.InitBase(obj, identityFunc, newMutator)

	// Default grace status: report Healthy.
	b.res.GraceStatusHandler = func(_ T) (concepts.GraceStatusWithReason, error) {
		return concepts.GraceStatusWithReason{
			Status: concepts.GraceStatusHealthy,
			Reason: "default grace status",
		}, nil
	}

	b.res.BaseResource = *b.BaseRes
	return b
}

// WithMutation registers one or more typed feature mutations for the workload,
// in the order provided.
func (b *WorkloadBuilder[T, M]) WithMutation(
	ms ...Mutation[M],
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithMutation(ms...)
	return b
}

// WithGuard registers a guard precondition for the workload resource.
func (b *WorkloadBuilder[T, M]) WithGuard(
	handler func(T) (concepts.GuardStatusWithReason, error),
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithGuard(handler)
	return b
}

// WithDataGuard declares blocking data reads for the workload resource. See
// BaseBuilder.WithDataGuard.
func (b *WorkloadBuilder[T, M]) WithDataGuard(cells ...concepts.DataCell) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithDataGuard(cells...)
	return b
}

// WithOptionalData declares non-blocking data reads for the workload
// resource. See BaseBuilder.WithOptionalData.
func (b *WorkloadBuilder[T, M]) WithOptionalData(cells ...concepts.DataCell) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithOptionalData(cells...)
	return b
}

// WithCustomConvergeStatus overrides the workload convergence status handler.
func (b *WorkloadBuilder[T, M]) WithCustomConvergeStatus(
	handler func(concepts.ConvergingOperation, T) (concepts.AliveStatusWithReason, error),
) *WorkloadBuilder[T, M] {
	b.res.ConvergingStatusHandler = handler
	return b
}

// WithCustomGraceStatus overrides the workload grace status handler.
func (b *WorkloadBuilder[T, M]) WithCustomGraceStatus(
	handler func(T) (concepts.GraceStatusWithReason, error),
) *WorkloadBuilder[T, M] {
	b.res.GraceStatusHandler = handler
	return b
}

// WithCustomSuspendStatus overrides the workload suspension status handler.
//
// The handler receives the object as it stands after the suspension apply of
// the current reconcile, so it observes server-populated fields such as
// Generation and Status. See BaseBuilder.WithCustomSuspendStatus.
func (b *WorkloadBuilder[T, M]) WithCustomSuspendStatus(
	handler func(T) (concepts.SuspensionStatusWithReason, error),
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation overrides the workload suspension mutation handler.
//
// The handler receives the mutator for the object that is about to be applied,
// which after the first reconcile carries the API server's response from the
// previous apply. See BaseBuilder.WithCustomSuspendMutation.
func (b *WorkloadBuilder[T, M]) WithCustomSuspendMutation(
	handler func(M) error,
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the workload delete-on-suspend decision handler.
//
// The handler is consulted both before the suspension apply and after the
// resource reports Suspended, so the decision must be stable across both.
// See BaseBuilder.WithCustomSuspendDeletionDecision.
func (b *WorkloadBuilder[T, M]) WithCustomSuspendDeletionDecision(
	handler func(T) bool,
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendDeletionDecision(handler)
	return b
}

// Build validates the workload builder configuration and returns the initialized resource.
//
// It returns an error if the converging status handler has not been set.
func (b *WorkloadBuilder[T, M]) Build() (*WorkloadResource[T, M], error) {
	b.res.BaseResource = *b.BaseRes
	if err := b.ValidateBase(); err != nil {
		return nil, err
	}
	if b.res.ConvergingStatusHandler == nil {
		return nil, errors.New("converging status handler is required")
	}
	return b.res, nil
}
