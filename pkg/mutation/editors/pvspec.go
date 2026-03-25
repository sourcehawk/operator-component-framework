package editors

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PVSpecEditor provides a typed API for mutating a Kubernetes PersistentVolumeSpec.
//
// It exposes structured operations for the most common PV spec fields. Use Raw()
// for free-form access when none of the structured methods are sufficient.
type PVSpecEditor struct {
	spec *corev1.PersistentVolumeSpec
}

// NewPVSpecEditor creates a new PVSpecEditor for the given PersistentVolumeSpec.
func NewPVSpecEditor(spec *corev1.PersistentVolumeSpec) *PVSpecEditor {
	return &PVSpecEditor{spec: spec}
}

// Raw returns the underlying *corev1.PersistentVolumeSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *PVSpecEditor) Raw() *corev1.PersistentVolumeSpec {
	return e.spec
}

// SetCapacity sets the storage capacity of the PersistentVolume.
func (e *PVSpecEditor) SetCapacity(storage resource.Quantity) {
	if e.spec.Capacity == nil {
		e.spec.Capacity = make(corev1.ResourceList)
	}
	e.spec.Capacity[corev1.ResourceStorage] = storage
}

// SetAccessModes replaces the access modes of the PersistentVolume.
func (e *PVSpecEditor) SetAccessModes(modes []corev1.PersistentVolumeAccessMode) {
	e.spec.AccessModes = modes
}

// SetPersistentVolumeReclaimPolicy sets the reclaim policy for the PersistentVolume.
func (e *PVSpecEditor) SetPersistentVolumeReclaimPolicy(policy corev1.PersistentVolumeReclaimPolicy) {
	e.spec.PersistentVolumeReclaimPolicy = policy
}

// SetStorageClassName sets the storage class name for the PersistentVolume.
func (e *PVSpecEditor) SetStorageClassName(name string) {
	e.spec.StorageClassName = name
}

// SetMountOptions replaces the mount options for the PersistentVolume.
func (e *PVSpecEditor) SetMountOptions(options []string) {
	e.spec.MountOptions = options
}

// SetVolumeMode sets the volume mode for the PersistentVolume.
func (e *PVSpecEditor) SetVolumeMode(mode corev1.PersistentVolumeMode) {
	e.spec.VolumeMode = &mode
}

// SetNodeAffinity sets the node affinity for the PersistentVolume.
func (e *PVSpecEditor) SetNodeAffinity(affinity *corev1.VolumeNodeAffinity) {
	e.spec.NodeAffinity = affinity
}
