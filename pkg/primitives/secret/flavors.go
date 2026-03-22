package secret

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors"
	corev1 "k8s.io/api/core/v1"
)

// FieldApplicationFlavor defines a function signature for applying flavors to a
// Secret resource. A flavor is called after the baseline field applicator has
// run and can be used to preserve or merge fields from the live cluster object.
type FieldApplicationFlavor flavors.FieldApplicationFlavor[*corev1.Secret]

// PreserveCurrentLabels ensures that any labels present on the current live
// Secret but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels(applied, current, desired *corev1.Secret) error {
	return flavors.PreserveCurrentLabels[*corev1.Secret]()(applied, current, desired)
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live Secret but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations(applied, current, desired *corev1.Secret) error {
	return flavors.PreserveCurrentAnnotations[*corev1.Secret]()(applied, current, desired)
}

// PreserveExternalEntries ensures that any .data keys present on the current live
// Secret but absent from the applied (desired) object are preserved on the
// applied object.
//
// This is useful when other controllers or admission webhooks inject entries into
// the Secret that your operator does not own. Keys present in both are left
// as-is on the applied object (the desired value wins).
func PreserveExternalEntries(applied, current, _ *corev1.Secret) error {
	for k, v := range current.Data {
		if _, exists := applied.Data[k]; !exists {
			if applied.Data == nil {
				applied.Data = make(map[string][]byte)
			}
			applied.Data[k] = v
		}
	}
	return nil
}
