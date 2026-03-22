package pvc

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	corev1 "k8s.io/api/core/v1"
)

// FieldApplicationFlavor defines a function signature for applying flavors to a
// PVC resource. A flavor is called after the baseline field applicator has run
// and can be used to preserve or merge fields from the live cluster object.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*corev1.PersistentVolumeClaim]

// PreserveCurrentLabels ensures that any labels present on the current live
// PVC but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *corev1.PersistentVolumeClaim) error {
	return flavors.PreserveCurrentLabels[*corev1.PersistentVolumeClaim]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live PVC but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *corev1.PersistentVolumeClaim) error {
	return flavors.PreserveCurrentAnnotations[*corev1.PersistentVolumeClaim]()(applied, current, desired)
}
