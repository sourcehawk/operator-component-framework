package generic

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkloadBuilder configures a generic internal workload resource for Kubernetes primitives
// such as Deployments, StatefulSets, and DaemonSets.
//
// It captures the common framework concepts while leaving kind-specific defaults and wrappers
// to the concrete workload packages.
type WorkloadBuilder[T client.Object, M MutatorApplier] struct {
	res *WorkloadResource[T, M]
}

// NewWorkloadBuilder creates a new generic workload builder.
//
// The provided object is treated as the desired base state. The mutator factory is used to
// construct the typed mutator during Mutate.
func NewWorkloadBuilder[T client.Object, M MutatorApplier](
	obj T,
	identityFunc func(T) string,
	defaultApplicator FieldApplicator[T],
	newMutator func(T) M,
) *WorkloadBuilder[T, M] {
	return &WorkloadBuilder[T, M]{
		res: &WorkloadResource[T, M]{
			Object:                 obj,
			IdentityFunc:           identityFunc,
			DefaultFieldApplicator: defaultApplicator,
			NewMutator:             newMutator,
		},
	}
}

// WithMutation registers a typed feature mutation for the workload.
func (b *WorkloadBuilder[T, M]) WithMutation(
	m feature.Mutation[M],
) *WorkloadBuilder[T, M] {
	b.res.Mutations = append(b.res.Mutations, m)
	return b
}

// WithCustomFieldApplicator overrides the default baseline field applicator.
func (b *WorkloadBuilder[T, M]) WithCustomFieldApplicator(
	applicator FieldApplicator[T],
) *WorkloadBuilder[T, M] {
	b.res.CustomFieldApplicator = applicator
	return b
}

// WithFieldApplicationFlavor registers a post-baseline field application flavor.
func (b *WorkloadBuilder[T, M]) WithFieldApplicationFlavor(
	flavor FieldApplicationFlavor[T],
) *WorkloadBuilder[T, M] {
	if flavor != nil {
		b.res.FieldFlavors = append(b.res.FieldFlavors, flavor)
	}
	return b
}

// WithDataExtractor registers a typed data extractor to run after successful reconciliation.
func (b *WorkloadBuilder[T, M]) WithDataExtractor(
	extractor func(T) error,
) *WorkloadBuilder[T, M] {
	if extractor != nil {
		b.res.DataExtractors = append(b.res.DataExtractors, extractor)
	}
	return b
}

// WithCustomConvergeStatus overrides the workload convergence status handler.
func (b *WorkloadBuilder[T, M]) WithCustomConvergeStatus(
	handler func(component.ConvergingOperation, T) (component.ConvergingStatusWithReason, error),
) *WorkloadBuilder[T, M] {
	b.res.ConvergingStatusHandler = handler
	return b
}

// WithCustomGraceStatus overrides the workload grace status handler.
func (b *WorkloadBuilder[T, M]) WithCustomGraceStatus(
	handler func(T) (component.GraceStatusWithReason, error),
) *WorkloadBuilder[T, M] {
	b.res.GraceStatusHandler = handler
	return b
}

// WithCustomSuspendStatus overrides the workload suspension status handler.
func (b *WorkloadBuilder[T, M]) WithCustomSuspendStatus(
	handler func(T) (component.SuspensionStatusWithReason, error),
) *WorkloadBuilder[T, M] {
	b.res.SuspendStatusHandler = handler
	return b
}

// WithCustomSuspendMutation overrides the workload suspension mutation handler.
func (b *WorkloadBuilder[T, M]) WithCustomSuspendMutation(
	handler func(M) error,
) *WorkloadBuilder[T, M] {
	b.res.SuspendMutationHandler = handler
	return b
}

// WithCustomSuspendDeletionDecision overrides the workload delete-on-suspend decision handler.
func (b *WorkloadBuilder[T, M]) WithCustomSuspendDeletionDecision(
	handler func(T) bool,
) *WorkloadBuilder[T, M] {
	b.res.DeleteOnSuspendHandler = handler
	return b
}

// Build validates the workload builder configuration and returns the initialized resource.
func (b *WorkloadBuilder[T, M]) Build() (*WorkloadResource[T, M], error) {
	if isNil(b.res.Object) {
		return nil, errors.New("object cannot be nil")
	}

	if b.res.Object.GetName() == "" {
		return nil, errors.New("object name cannot be empty")
	}

	if b.res.Object.GetNamespace() == "" {
		return nil, errors.New("object namespace cannot be empty")
	}

	if b.res.IdentityFunc == nil {
		return nil, errors.New("identity function cannot be nil")
	}

	if b.res.DefaultFieldApplicator == nil {
		return nil, errors.New("default field applicator cannot be nil")
	}

	if b.res.NewMutator == nil {
		return nil, errors.New("mutator factory cannot be nil")
	}

	return b.res, nil
}
