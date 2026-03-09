// Package feature provides mechanisms for version-gated feature mutations.
package feature

import (
	"fmt"
)

// VersionConstraint defines a condition based on a semantic version.
// Implementations should report whether a feature is enabled for the given version string.
type VersionConstraint interface {
	// Enabled reports whether the feature is enabled for the given version string.
	Enabled(version string) (bool, error)
}

// ResourceFeature represents the conditions under which a feature mutation applies.
//
// A ResourceFeature is enabled only when all registered semver constraints match
// the current version and all additional truth conditions added via When are true.
type ResourceFeature struct {
	current            string
	versionConstraints []VersionConstraint

	// requiredTruths contains additional boolean conditions that must all be true
	// for the feature to apply.
	requiredTruths []bool
}

// NewResourceFeature creates a new ResourceFeature for the given current version
// and semver constraints.
//
// Nil constraints are ignored.
func NewResourceFeature(currentVersion string, versionConstraints []VersionConstraint) *ResourceFeature {
	var features []VersionConstraint

	for _, constraint := range versionConstraints {
		if constraint != nil {
			features = append(features, constraint)
		}
	}

	return &ResourceFeature{
		current:            currentVersion,
		versionConstraints: features,
	}
}

// When adds a boolean condition that must be true for the feature
// to be enabled.
//
// Calls are additive: all values passed through When must be true for Enabled()
// to return true.
func (f *ResourceFeature) When(truth bool) *ResourceFeature {
	f.requiredTruths = append(f.requiredTruths, truth)
	return f
}

// Enabled reports whether the feature should apply.
//
// A feature is enabled only if:
// - all When conditions are true
// - all version constraints match the current version.
func (f *ResourceFeature) Enabled() (bool, error) {
	for _, truth := range f.requiredTruths {
		if !truth {
			return false, nil
		}
	}

	for _, constraint := range f.versionConstraints {
		enabled, err := constraint.Enabled(f.current)
		if err != nil {
			return false, err
		}
		if !enabled {
			return false, nil
		}
	}

	return true, nil
}

// Mutation defines a mutation that is applied to an object of type T
// only if its associated ResourceFeature is enabled.
type Mutation[T any] struct {
	// Name is a human-readable identifier for the mutation, used for error reporting.
	Name string
	// Feature determines whether the mutation should be applied.
	Feature *ResourceFeature
	// Mutate is the function that applies the changes to the object.
	Mutate func(T) error
}

// ApplyIntent applies the mutation intent on the provided object if the feature is enabled.
// If the feature is nil or disabled, it returns nil without performing any action.
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
