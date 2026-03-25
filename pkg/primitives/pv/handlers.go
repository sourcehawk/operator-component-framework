package pv

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
)

// DefaultOperationalStatusHandler is the default logic for determining if a PersistentVolume
// is operationally ready.
//
// It considers a PV operational when its Status.Phase is Available or Bound.
// A PV in the Pending phase is reported as concepts.OperationalStatusPending. A PV in the
// Released or Failed phase is reported as concepts.OperationalStatusFailing.
//
// This function is used as the default handler by the Resource if no custom handler
// is registered via Builder.WithCustomOperationalStatus. It can be reused within
// custom handlers to augment the default behavior.
func DefaultOperationalStatusHandler(
	_ concepts.ConvergingOperation, pv *corev1.PersistentVolume,
) (concepts.OperationalStatusWithReason, error) {
	switch pv.Status.Phase {
	case corev1.VolumeAvailable:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusOperational,
			Reason: "PersistentVolume is available",
		}, nil

	case corev1.VolumeBound:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusOperational,
			Reason: "PersistentVolume is bound to a claim",
		}, nil

	case corev1.VolumePending:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusPending,
			Reason: "PersistentVolume is pending",
		}, nil

	case corev1.VolumeReleased:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusFailing,
			Reason: "PersistentVolume has been released and is not yet reclaimed",
		}, nil

	case corev1.VolumeFailed:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusFailing,
			Reason: "PersistentVolume reclamation has failed",
		}, nil

	default:
		return concepts.OperationalStatusWithReason{
			Status: concepts.OperationalStatusPending,
			Reason: fmt.Sprintf("PersistentVolume phase is %q", pv.Status.Phase),
		}, nil
	}
}
