package service

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	corev1 "k8s.io/api/core/v1"
)

// FieldApplicationFlavor defines a function signature for applying flavors to a
// Service resource. A flavor is called after the baseline field applicator has
// run and can be used to preserve or merge fields from the live cluster object.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*corev1.Service]

// PreserveCurrentLabels ensures that any labels present on the current live
// Service but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *corev1.Service) error {
	return flavors.PreserveCurrentLabels[*corev1.Service]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live Service but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *corev1.Service) error {
	return flavors.PreserveCurrentAnnotations[*corev1.Service]()(applied, current, desired)
}
