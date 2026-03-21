package generic

import "fmt"

// MutationFeature is an optional feature gate for a Mutation.
// If the gate's Enabled() returns false, the mutation is not applied.
type MutationFeature interface {
	Enabled() (bool, error)
}

// Mutation defines a conditional mutation applied to an object of type T
// only if its associated MutationFeature is enabled.
//
// If Feature is nil, the mutation is skipped entirely.
type Mutation[T any] struct {
	// Name is a human-readable identifier for the mutation, used for error reporting.
	Name string
	// Feature determines whether the mutation should be applied.
	// A nil Feature causes the mutation to be skipped.
	Feature MutationFeature
	// Mutate is the function that applies the changes to the object.
	Mutate func(T) error
}

// ApplyIntent applies the mutation intent on the provided object if the feature is enabled.
// If Feature is nil or disabled, it returns nil without performing any action.
// If the mutation is enabled but the Mutate function is nil, it returns an error.
func (m *Mutation[T]) ApplyIntent(obj T) error {
	if m.Feature == nil {
		return nil
	}

	enabled, err := m.Feature.Enabled()
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	if m.Mutate == nil {
		return fmt.Errorf("mutation handler of %s is nil", m.Name)
	}

	return m.Mutate(obj)
}
