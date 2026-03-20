package generic

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TaskResource is a generic internal resource implementation for Kubernetes
// objects that run to completion, such as Jobs.
//
// It provides shared behavior for:
//   - baseline field application
//   - field application flavors
//   - feature mutations
//   - suspension mutations
//   - data extraction
//
// Concrete task packages are expected to wrap this type and provide kind-specific
// identity and status logic.
type TaskResource[T client.Object, M MutatorApplier] struct {
	BaseResource[T, M]

	ConvergingStatusHandler func(concepts.ConvergingOperation, T) (concepts.CompletionStatusWithReason, error)
}

// ConvergingStatus reports the task's completion status using the configured handler.
func (r *TaskResource[T, M]) ConvergingStatus(
	op concepts.ConvergingOperation,
) (concepts.CompletionStatusWithReason, error) {
	if r.ConvergingStatusHandler == nil {
		return concepts.CompletionStatusWithReason{}, fmt.Errorf("converging status handler is not configured")
	}
	return r.ConvergingStatusHandler(op, r.DesiredObject)
}
