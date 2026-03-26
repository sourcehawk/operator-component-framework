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

// WithMutation registers a typed feature mutation for the workload.
func (b *WorkloadBuilder[T, M]) WithMutation(
	m Mutation[M],
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithMutation(m)
	return b
}

// WithDataExtractor registers a typed data extractor to run after successful reconciliation.
func (b *WorkloadBuilder[T, M]) WithDataExtractor(
	extractor func(T) error,
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithDataExtractor(extractor)
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
func (b *WorkloadBuilder[T, M]) WithCustomSuspendStatus(
	handler func(T) (concepts.SuspensionStatusWithReason, error),
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendStatus(handler)
	return b
}

// WithCustomSuspendMutation overrides the workload suspension mutation handler.
func (b *WorkloadBuilder[T, M]) WithCustomSuspendMutation(
	handler func(M) error,
) *WorkloadBuilder[T, M] {
	b.BaseBuilder.WithCustomSuspendMutation(handler)
	return b
}

// WithCustomSuspendDeletionDecision overrides the workload delete-on-suspend decision handler.
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
