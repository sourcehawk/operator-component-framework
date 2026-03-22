package pvc

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
)

// DefaultOperationalStatusHandler is the default logic for determining if a PVC
// has reached its desired operational state.
//
// It considers a PVC operational when its Status.Phase is Bound:
//   - Bound → Operational
//   - Pending → Pending
//   - Lost → Failing
//
// This function is used as the default handler by the Resource if no custom handler
// is registered via Builder.WithCustomOperationalStatus. It can be reused within
// custom handlers to augment the default behavior.
func DefaultOperationalStatusHandler(
	_ concepts.ConvergingOperation, pvc *corev1.PersistentVolumeClaim,
) (concepts.OperationalStatusWithReason, error) {
	switch pvc.Status.Phase {
	case corev1.ClaimBound:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusOperational,
			Reason: fmt.Sprintf("PVC is bound to volume %s", pvc.Spec.VolumeName),
		}, nil
	case corev1.ClaimLost:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusFailing,
			Reason: "PVC has lost its bound volume",
		}, nil
	default:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusPending,
			Reason: "Waiting for PVC to be bound",
		}, nil
	}
}

// DefaultDeleteOnSuspendHandler provides the default decision of whether to delete
// the PVC when the parent component is suspended.
//
// It always returns false, meaning the PVC is kept in the cluster to preserve its
// underlying storage. Deleting a PVC can lead to permanent data loss if the reclaim
// policy is Delete.
//
// This function is used as the default handler by the Resource if no custom handler
// is registered via Builder.WithCustomSuspendDeletionDecision.
func DefaultDeleteOnSuspendHandler(_ *corev1.PersistentVolumeClaim) bool {
	return false
}

// DefaultSuspendMutationHandler provides the default mutation applied to a PVC when
// the component is suspended.
//
// It is a no-op: PVCs do not have runtime state (like replicas) that needs to be
// wound down. The consuming workload (e.g. a Deployment) is responsible for stopping
// its use of the PVC.
//
// This function is used as the default handler by the Resource if no custom handler
// is registered via Builder.WithCustomSuspendMutation.
func DefaultSuspendMutationHandler(_ *Mutator) error {
	return nil
}

// DefaultSuspensionStatusHandler monitors the progress of the suspension process.
//
// It always reports Suspended because PVCs themselves have no runtime state to wind
// down. Once Suspend() is called, the PVC is considered immediately suspended.
//
// This function is used as the default handler by the Resource if no custom handler
// is registered via Builder.WithCustomSuspendStatus.
func DefaultSuspensionStatusHandler(_ *corev1.PersistentVolumeClaim) (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "PVC does not require suspension actions",
	}, nil
}
