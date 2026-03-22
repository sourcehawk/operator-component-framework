package editors

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PVCSpecEditor provides a typed API for mutating a Kubernetes PersistentVolumeClaimSpec.
type PVCSpecEditor struct {
	spec *corev1.PersistentVolumeClaimSpec
}

// NewPVCSpecEditor creates a new PVCSpecEditor for the given PersistentVolumeClaimSpec.
func NewPVCSpecEditor(spec *corev1.PersistentVolumeClaimSpec) *PVCSpecEditor {
	return &PVCSpecEditor{spec: spec}
}

// Raw returns the underlying *corev1.PersistentVolumeClaimSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *PVCSpecEditor) Raw() *corev1.PersistentVolumeClaimSpec {
	return e.spec
}

// SetStorageRequest sets the storage resource request for the PVC.
//
// On existing PVCs, Kubernetes only allows storage expansion (not shrinking),
// provided the StorageClass supports volume expansion.
func (e *PVCSpecEditor) SetStorageRequest(quantity resource.Quantity) {
	if e.spec.Resources.Requests == nil {
		e.spec.Resources.Requests = corev1.ResourceList{}
	}
	e.spec.Resources.Requests[corev1.ResourceStorage] = quantity
}

// SetAccessModes sets the access modes for the PVC.
//
// Access modes are immutable on existing PVCs. This method is intended for
// initial construction; the DefaultFieldApplicator preserves existing values.
func (e *PVCSpecEditor) SetAccessModes(modes []corev1.PersistentVolumeAccessMode) {
	e.spec.AccessModes = modes
}

// SetStorageClassName sets the storage class name for the PVC.
//
// The storage class name is immutable on existing PVCs. This method is intended
// for initial construction; the DefaultFieldApplicator preserves existing values.
func (e *PVCSpecEditor) SetStorageClassName(name string) {
	e.spec.StorageClassName = &name
}

// SetVolumeMode sets the volume mode (Filesystem or Block) for the PVC.
//
// The volume mode is immutable on existing PVCs. This method is intended for
// initial construction; the DefaultFieldApplicator preserves existing values.
func (e *PVCSpecEditor) SetVolumeMode(mode corev1.PersistentVolumeMode) {
	e.spec.VolumeMode = &mode
}

// SetVolumeName binds the PVC to a specific PersistentVolume by name.
//
// The volume name is immutable on existing PVCs. This method is intended for
// initial construction; the DefaultFieldApplicator preserves existing values.
func (e *PVCSpecEditor) SetVolumeName(name string) {
	e.spec.VolumeName = name
}
