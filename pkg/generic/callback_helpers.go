package generic

import "github.com/sourcehawk/operator-component-framework/pkg/component/concepts"

// WrapGuard converts a value-receiver guard callback into a pointer-receiver
// guard callback suitable for the generic builder layer.
// If the input function is nil, nil is returned.
func WrapGuard[E any](guard func(E) (concepts.GuardStatusWithReason, error)) func(*E) (concepts.GuardStatusWithReason, error) {
	if guard == nil {
		return nil
	}
	return func(ptr *E) (concepts.GuardStatusWithReason, error) {
		return guard(*ptr)
	}
}
