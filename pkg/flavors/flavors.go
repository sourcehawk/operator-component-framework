package flavors

import (
	"github.com/sourcehawk/operator-component-framework/pkg/flavors/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FieldApplicationFlavor defines a function signature for applying "flavors" to a resource.
// A flavor typically preserves certain fields from the current (live) object after the
// baseline field application has occurred.
type FieldApplicationFlavor[T client.Object] func(applied, current, desired T) error

// PreserveCurrentLabels ensures that any labels present on the current live
// resource but missing from the applied (desired) object are preserved.
// If a label exists in both, the applied value wins.
func PreserveCurrentLabels[T client.Object]() FieldApplicationFlavor[T] {
	return func(applied, current, _ T) error {
		applied.SetLabels(utils.PreserveMap(applied.GetLabels(), current.GetLabels()))
		return nil
	}
}

// PreserveCurrentAnnotations ensures that any annotations present on the current
// live resource but missing from the applied (desired) object are preserved.
// If an annotation exists in both, the applied value wins.
func PreserveCurrentAnnotations[T client.Object]() FieldApplicationFlavor[T] {
	return func(applied, current, _ T) error {
		applied.SetAnnotations(utils.PreserveMap(applied.GetAnnotations(), current.GetAnnotations()))
		return nil
	}
}
