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

// WrapExtractor converts a value-receiver data extractor callback into a
// pointer-receiver callback suitable for the generic builder layer.
// If the input function is nil, nil is returned.
func WrapExtractor[E any](extractor func(E) error) func(*E) error {
	if extractor == nil {
		return nil
	}
	return func(ptr *E) error {
		return extractor(*ptr)
	}
}
