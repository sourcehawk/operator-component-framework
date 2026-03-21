// Package feature provides mechanisms for version-gated feature mutations.
package feature

import "github.com/sourcehawk/operator-component-framework/internal/generic"

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
// only if its associated feature gate is enabled.
//
// Mutation is an alias for [generic.Mutation]. The Feature field accepts any
// value implementing [generic.MutationFeature], including *ResourceFeature.
// A nil Feature causes the mutation to be skipped.
type Mutation[T any] = generic.Mutation[T]
