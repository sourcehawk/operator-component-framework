package pdb

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	policyv1 "k8s.io/api/policy/v1"
)

// FieldApplicationFlavor defines a function signature for applying flavors to a
// PodDisruptionBudget resource. A flavor is called after the baseline field applicator
// has run and can be used to preserve or merge fields from the live cluster object.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*policyv1.PodDisruptionBudget]

// PreserveCurrentLabels ensures that any labels present on the current live
// PodDisruptionBudget but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *policyv1.PodDisruptionBudget) error {
	return flavors.PreserveCurrentLabels[*policyv1.PodDisruptionBudget]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live PodDisruptionBudget but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *policyv1.PodDisruptionBudget) error {
	return flavors.PreserveCurrentAnnotations[*policyv1.PodDisruptionBudget]()(applied, current, desired)
}
