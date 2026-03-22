package pod

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	corev1 "k8s.io/api/core/v1"
)

// FieldApplicationFlavor defines a function signature for applying "flavors" to a resource.
// A flavor typically preserves certain fields from the current (live) object after the
// baseline field application has occurred.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*corev1.Pod]

// PreserveCurrentLabels ensures that any labels present on the current live
// Pod but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *corev1.Pod) error {
	return flavors.PreserveCurrentLabels[*corev1.Pod]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live Pod but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *corev1.Pod) error {
	return flavors.PreserveCurrentAnnotations[*corev1.Pod]()(applied, current, desired)
}
