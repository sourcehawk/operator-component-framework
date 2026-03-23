package generic

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MutatorApplier is implemented by workload mutators that can apply their planned changes
// to the underlying Kubernetes object.
type MutatorApplier interface {
	Apply() error
}

// FeatureMutator is implemented by workload mutators that support defining feature boundaries.
// The interface is exported so that primitive mutators in external packages can satisfy it.
type FeatureMutator interface {
	MutatorApplier
	BeginFeature()
}

// WorkloadResource is a generic internal resource implementation for long-running Kubernetes
// workload objects such as Deployments, StatefulSets, and DaemonSets.
//
// It provides shared behavior for:
//   - baseline field application
//   - field application flavors
//   - feature mutations
//   - suspension mutations
//   - data extraction
//
// Concrete workload packages are expected to wrap this type and provide kind-specific
// identity and status logic.
type WorkloadResource[T client.Object, M MutatorApplier] struct {
	BaseResource[T, M]

	ConvergingStatusHandler func(concepts.ConvergingOperation, T) (concepts.AliveStatusWithReason, error)
	GraceStatusHandler      func(T) (concepts.GraceStatusWithReason, error)
}

// ConvergingStatus reports the workload's convergence status using the configured handler.
func (r *WorkloadResource[T, M]) ConvergingStatus(
	op concepts.ConvergingOperation,
) (concepts.AliveStatusWithReason, error) {
	if r.ConvergingStatusHandler == nil {
		return concepts.AliveStatusWithReason{}, fmt.Errorf("converging status handler is not configured")
	}
	return r.ConvergingStatusHandler(op, r.DesiredObject)
}

// GraceStatus reports the workload's grace status using the configured handler.
func (r *WorkloadResource[T, M]) GraceStatus() (concepts.GraceStatusWithReason, error) {
	if r.GraceStatusHandler == nil {
		return concepts.GraceStatusWithReason{}, fmt.Errorf("grace status handler is not configured")
	}
	return r.GraceStatusHandler(r.DesiredObject)
}
