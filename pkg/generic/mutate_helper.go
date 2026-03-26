package generic

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyMutations provides a shared implementation for the Mutate method in generic resources.
// It handles:
//  1. Feature mutations
//  2. Optional suspension mutations
func ApplyMutations[T client.Object, M FeatureMutator](
	current client.Object,
	newMutator func(T) M,
	mutations []Mutation[M],
	suspender func(M) error,
) (T, error) {
	currentTyped, ok := current.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("type assertion failed: expected current to be assignable to %T, got %T", zero, current)
	}

	mutator := newMutator(currentTyped)

	// The constructor creates the initial feature scope. After each mutation,
	// advance to the next scope so the following mutation gets its own boundary.
	for _, mutation := range mutations {
		if err := mutation.ApplyIntent(mutator); err != nil {
			var zero T
			return zero, fmt.Errorf("failed to apply mutation intent for %s: %w", mutation.Name, err)
		}

		mutator.NextFeature()
	}

	if err := mutator.Apply(); err != nil {
		var zero T
		return zero, fmt.Errorf("failed to apply planned mutations: %w", err)
	}

	if suspender != nil {
		// The trailing NextFeature() from the loop above already created a fresh scope.
		if err := suspender(mutator); err != nil {
			var zero T
			return zero, err
		}

		if err := mutator.Apply(); err != nil {
			var zero T
			return zero, fmt.Errorf("failed to apply suspension mutations: %w", err)
		}
	}

	return currentTyped, nil
}
