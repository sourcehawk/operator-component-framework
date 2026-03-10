package generic

import (
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// FieldApplicator applies desired state onto the current object to create
// the baseline applied state before field flavors and mutations run.
type FieldApplicator[T client.Object] func(current, desired T) error

// applyBaselineAndFlavors runs the standard field application pipeline:
//
// 1. snapshot the original current object
// 2. run the custom applicator if present, otherwise the default applicator
// 3. run all field application flavors in registration order
//
// The current object is mutated in place and returned.
func applyBaselineAndFlavors[T client.Object](
	current T,
	desired T,
	defaultApplicator FieldApplicator[T],
	customApplicator FieldApplicator[T],
	flavors []FieldApplicationFlavor[T],
) (T, error) {

	originalCurrent, ok := current.DeepCopyObject().(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("failed to deep copy current object of type %T", current)
	}

	applicator := defaultApplicator
	if customApplicator != nil {
		applicator = customApplicator
	}

	if applicator == nil {
		var zero T
		return zero, fmt.Errorf("no field applicator configured")
	}

	if err := applicator(current, desired); err != nil {
		var zero T
		return zero, fmt.Errorf("failed to apply baseline fields: %w", err)
	}

	for _, flavor := range flavors {
		if flavor == nil {
			continue
		}

		if err := flavor(current, originalCurrent, desired); err != nil {
			var zero T
			return zero, fmt.Errorf("failed to apply field application flavor: %w", err)
		}
	}

	return current, nil
}
