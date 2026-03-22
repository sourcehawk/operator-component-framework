package networkpolicy

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	networkingv1 "k8s.io/api/networking/v1"
)

// FieldApplicationFlavor defines a function signature for applying flavors to a
// NetworkPolicy resource. A flavor is called after the baseline field applicator
// has run and can be used to preserve or merge fields from the live cluster object.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*networkingv1.NetworkPolicy]

// PreserveCurrentLabels ensures that any labels present on the current live
// NetworkPolicy but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *networkingv1.NetworkPolicy) error {
	return flavors.PreserveCurrentLabels[*networkingv1.NetworkPolicy]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live NetworkPolicy but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *networkingv1.NetworkPolicy) error {
	return flavors.PreserveCurrentAnnotations[*networkingv1.NetworkPolicy]()(applied, current, desired)
}
