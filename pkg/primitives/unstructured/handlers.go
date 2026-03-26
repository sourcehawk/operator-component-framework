package unstructured

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DefaultSuspendMutationHandler is a no-op suspension mutation handler.
// It does not modify the object when suspension is triggered.
func DefaultSuspendMutationHandler(_ *Mutator) error {
	return nil
}

// DefaultSuspensionStatusHandler reports the resource as immediately Suspended.
// Use this when the resource has no meaningful suspension state to track.
func DefaultSuspensionStatusHandler(_ *uns.Unstructured) (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "no suspension status handler configured",
	}, nil
}

// DefaultGraceStatusHandler reports the resource as Healthy during grace periods.
// Use this when the resource has no meaningful degradation state to track.
func DefaultGraceStatusHandler(_ *uns.Unstructured) (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusHealthy,
		Reason: "no grace status handler configured",
	}, nil
}
